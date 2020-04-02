// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020 Datadog, Inc.

package listeners

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/soniah/gosnmp"
)

func init() {
	Register("snmp", NewSNMPListener)
}

// SNMPListener implements SNMP discovery
type SNMPListener struct {
	sync.RWMutex
	newService chan<- Service
	delService chan<- Service
	stop       chan bool
	config     util.SNMPListenerConfig
	services   map[string]Service
	devices    map[string][]string
}

// SNMPService implements and store results from the Service interface for the SNMP listener
type SNMPService struct {
	adIdentifier string
	entityID     string
	deviceIP     string
	creationTime integration.CreationTime
}

// Make sure SNMPService implements the Service interface
var _ Service = &SNMPService{}

type snmpSubnet struct {
	adIdentifier  string
	config        util.SNMPConfig
	defaultParams *gosnmp.GoSNMP
	startingIP    net.IP
	currentIP     net.IP
	network       net.IPNet
	cacheKey      string
}

// NewSNMPListener creates a SNMPListener
func NewSNMPListener() (ServiceListener, error) {
	var snmpConfig util.SNMPListenerConfig
	if err := config.Datadog.UnmarshalKey("snmp_listener", &snmpConfig); err != nil {
		return nil, err
	}
	return &SNMPListener{
		services: map[string]Service{},
		devices:  map[string][]string{},
		stop:     make(chan bool),
		config:   snmpConfig,
	}, nil
}

// Listen periodically refreshes devices
func (l *SNMPListener) Listen(newSvc chan<- Service, delSvc chan<- Service) {
	// setup the I/O channels
	l.newService = newSvc
	l.delService = delSvc

	go l.checkDevices()
}

func (l *SNMPListener) loadCache(config util.SNMPConfig, adIdentifier string, cacheKey string) {
	cacheValue, err := persistentcache.Read(cacheKey)
	if err != nil {
		log.Errorf("Couldn't read cache for %s: %s", cacheKey, err)
		return
	}
	if cacheValue == "" {
		return
	}
	var devices []net.IP
	if err = json.Unmarshal([]byte(cacheValue), &devices); err != nil {
		log.Errorf("Couldn't unmarshal cache for %s: %s", cacheKey, err)
		return
	}
	for _, deviceIP := range devices {
		entityID := config.Digest(deviceIP.String())
		l.createService(deviceIP.String(), adIdentifier, entityID)
	}
}

func (l *SNMPListener) writeCache(cacheKey string, adIdentifier string) {
	l.Lock()
	defer l.Unlock()
	devices := l.devices[adIdentifier]
	cacheValue, err := json.Marshal(devices)
	if err != nil {
		log.Errorf("Couldn't marshal cache: %s", err)
		return
	}
	if err = persistentcache.Write(cacheKey, string(cacheValue)); err != nil {
		log.Errorf("Couldn't write cache: %s", err)
	}
}

// Don't make it a method, to be overridden in tests
var worker = func(l *SNMPListener, jobs <-chan snmpSubnet) {
	for {
		select {
		case <-l.stop:
			log.Debug("Stopping SNMP worker")
			return
		case subnet := <-jobs:
			log.Debugf("Handling IP %s", subnet.currentIP.String())
			l.checkDevice(subnet.adIdentifier, subnet.currentIP.String(), subnet.config, subnet.defaultParams)
		}
	}
}

func (l *SNMPListener) checkDevice(adIdentifier string, deviceIP string, config util.SNMPConfig, defaultParams *gosnmp.GoSNMP) {
	params := *defaultParams
	params.Target = deviceIP
	entityID := config.Digest(deviceIP)
	if err := params.Connect(); err != nil {
		log.Debugf("SNMP connect to %s error: %v", deviceIP, err)
		l.deleteService(entityID)
	} else {
		defer params.Conn.Close()

		oids := []string{"1.3.6.1.2.1.1.2.0"}
		value, err := params.Get(oids)
		if err != nil {
			log.Debugf("SNMP get to %s error: %v", deviceIP, err)
			l.deleteService(entityID)
		} else if len(value.Variables) < 1 || value.Variables[0].Value == nil {
			log.Debugf("SNMP get to %s no data", deviceIP)
			l.deleteService(entityID)
		} else {
			log.Debugf("SNMP get to %s success: %v", deviceIP, value.Variables[0].Value)
			l.createService(deviceIP, adIdentifier, entityID)
		}
	}
}

func (l *SNMPListener) checkDevices() {
	subnets := []snmpSubnet{}
	for _, config := range l.config.Configs {
		ipAddr, ipNet, err := net.ParseCIDR(config.Network)
		if err != nil {
			log.Errorf("Couldn't parse SNMP network: %s", err)
			continue
		}

		defaultParams, err := config.BuildSNMPParams()
		if err != nil {
			log.Error(err)
			continue
		}

		startingIP := ipAddr.Mask(ipNet.Mask)

		configHash := config.Digest(config.Network)
		cacheKey := fmt.Sprintf("snmp:%s", configHash)
		adIdentifier := cacheKey
		l.loadCache(config, adIdentifier, cacheKey)

		subnet := snmpSubnet{
			adIdentifier:  adIdentifier,
			config:        config,
			defaultParams: defaultParams,
			startingIP:    startingIP,
			network:       *ipNet,
			cacheKey:      cacheKey,
		}
		subnets = append(subnets, subnet)
	}

	if l.config.Workers == 0 {
		l.config.Workers = 2
	}

	if l.config.DiscoveryInterval == 0 {
		l.config.DiscoveryInterval = 3600
	}

	jobs := make(chan snmpSubnet)
	for w := 0; w < l.config.Workers; w++ {
		go worker(l, jobs)
	}

	discoveryTicker := time.NewTicker(time.Duration(l.config.DiscoveryInterval) * time.Second)

	for {
		for _, subnet := range subnets {
			startingIP := make(net.IP, len(subnet.startingIP))
			copy(startingIP, subnet.startingIP)
			for currentIP := startingIP; subnet.network.Contains(currentIP); incrementIP(currentIP) {

				if ignored := subnet.config.IsIPIgnored(currentIP); ignored {
					continue
				}

				jobSubnet := subnet
				jobSubnet.currentIP = make(net.IP, len(currentIP))
				copy(jobSubnet.currentIP, currentIP)
				jobs <- jobSubnet

				select {
				case <-l.stop:
					return
				default:
				}
			}
			// XXX write the cache a bit more often
			l.writeCache(subnet.cacheKey, subnet.adIdentifier)
		}

		select {
		case <-l.stop:
			return
		case <-discoveryTicker.C:
		}
	}
}

func (l *SNMPListener) createService(deviceIP string, adIdentifier string, entityID string) {
	l.Lock()
	defer l.Unlock()
	if _, present := l.services[entityID]; present {
		return
	}
	svc := &SNMPService{
		adIdentifier: adIdentifier,
		entityID:     entityID,
		deviceIP:     deviceIP,
		creationTime: integration.Before,
	}
	l.services[entityID] = svc
	l.devices[adIdentifier] = append(l.devices[adIdentifier], deviceIP)
	l.newService <- svc
}

func (l *SNMPListener) deleteService(entityID string) {
	l.Lock()
	defer l.Unlock()
	// XXX don't delete on first failure
	if svc, present := l.services[entityID]; present {
		l.delService <- svc
		delete(l.services, entityID)
	}
}

func incrementIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] > 0 {
			break
		}
	}
}

// Stop queues a shutdown of SNMPListener
func (l *SNMPListener) Stop() {
	l.stop <- true
}

// GetEntity returns the unique entity ID linked to that service
func (s *SNMPService) GetEntity() string {
	return s.entityID
}

// GetTaggerEntity returns the unique entity ID linked to that service
func (s *SNMPService) GetTaggerEntity() string {
	return s.entityID
}

// GetADIdentifiers returns a set of AD identifiers
func (s *SNMPService) GetADIdentifiers() ([]string, error) {
	return []string{s.adIdentifier}, nil
}

// GetHosts returns the device IP
func (s *SNMPService) GetHosts() (map[string]string, error) {
	ips := map[string]string{
		"": s.deviceIP,
	}
	return ips, nil
}

// GetPorts returns the device ports - currently not supported
func (s *SNMPService) GetPorts() ([]ContainerPort, error) {
	return []ContainerPort{}, ErrNotSupported
}

// GetTags returns the list of container tags - currently always empty
func (s *SNMPService) GetTags() ([]string, error) {
	return []string{}, nil
}

// GetPid returns nil and an error because pids are currently not supported
func (s *SNMPService) GetPid() (int, error) {
	return -1, ErrNotSupported
}

// GetHostname returns nothing - not supported
func (s *SNMPService) GetHostname() (string, error) {
	return "", ErrNotSupported
}

// GetCreationTime returns the creation time of the Service
func (s *SNMPService) GetCreationTime() integration.CreationTime {
	return s.creationTime
}

// IsReady returns true
func (s *SNMPService) IsReady() bool {
	return true
}

// GetCheckNames returns an empty slice
func (s *SNMPService) GetCheckNames() []string {
	return []string{}
}
