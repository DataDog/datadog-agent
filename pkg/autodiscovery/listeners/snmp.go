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
	"github.com/DataDog/datadog-agent/pkg/util/containers"
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
}

// SNMPService implements and store results from the Service interface for the SNMP listener
type SNMPService struct {
	adIdentifier string
	entityID     string
	deviceIP     string
	creationTime integration.CreationTime
	config       util.SNMPConfig
}

// Make sure SNMPService implements the Service interface
var _ Service = &SNMPService{}

type snmpSubnet struct {
	adIdentifier   string
	config         util.SNMPConfig
	defaultParams  *gosnmp.GoSNMP
	startingIP     net.IP
	network        net.IPNet
	cacheKey       string
	devices        map[string]string
	deviceFailures map[string]int
}

type snmpJob struct {
	subnet    snmpSubnet
	currentIP net.IP
}

// NewSNMPListener creates a SNMPListener
func NewSNMPListener() (ServiceListener, error) {
	var snmpConfig util.SNMPListenerConfig
	if err := config.Datadog.UnmarshalKey("snmp_listener", &snmpConfig); err != nil {
		return nil, err
	}
	return &SNMPListener{
		services: map[string]Service{},
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

func (l *SNMPListener) loadCache(subnet snmpSubnet) {
	cacheValue, err := persistentcache.Read(subnet.cacheKey)
	if err != nil {
		log.Errorf("Couldn't read cache for %s: %s", subnet.cacheKey, err)
		return
	}
	if cacheValue == "" {
		return
	}
	var devices []net.IP
	if err = json.Unmarshal([]byte(cacheValue), &devices); err != nil {
		log.Errorf("Couldn't unmarshal cache for %s: %s", subnet.cacheKey, err)
		return
	}
	for _, deviceIP := range devices {
		entityID := subnet.config.Digest(deviceIP.String())
		l.createService(entityID, subnet, deviceIP.String(), false)
	}
}

func (l *SNMPListener) writeCache(subnet snmpSubnet, lock bool) {
	if lock {
		l.Lock()
		defer l.Unlock()
	}

	devices := make([]string, 0, len(subnet.devices))
	for _, v := range subnet.devices {
		devices = append(devices, v)
	}

	cacheValue, err := json.Marshal(devices)
	if err != nil {
		log.Errorf("Couldn't marshal cache: %s", err)
		return
	}

	if err = persistentcache.Write(subnet.cacheKey, string(cacheValue)); err != nil {
		log.Errorf("Couldn't write cache: %s", err)
	}
}

// Don't make it a method, to be overridden in tests
var worker = func(l *SNMPListener, jobs <-chan snmpJob) {
	for {
		select {
		case <-l.stop:
			log.Debug("Stopping SNMP worker")
			return
		case job := <-jobs:
			log.Debugf("Handling IP %s", job.currentIP.String())
			l.checkDevice(job)
		}
	}
}

func (l *SNMPListener) checkDevice(job snmpJob) {
	params := *job.subnet.defaultParams
	deviceIP := job.currentIP.String()
	params.Target = deviceIP
	entityID := job.subnet.config.Digest(deviceIP)
	if err := params.Connect(); err != nil {
		log.Debugf("SNMP connect to %s error: %v", deviceIP, err)
		l.deleteService(entityID, job.subnet)
	} else {
		defer params.Conn.Close()

		oids := []string{"1.3.6.1.2.1.1.2.0"}
		value, err := params.Get(oids)
		if err != nil {
			log.Debugf("SNMP get to %s error: %v", deviceIP, err)
			l.deleteService(entityID, job.subnet)
		} else if len(value.Variables) < 1 || value.Variables[0].Value == nil {
			log.Debugf("SNMP get to %s no data", deviceIP)
			l.deleteService(entityID, job.subnet)
		} else {
			log.Debugf("SNMP get to %s success: %v", deviceIP, value.Variables[0].Value)
			l.createService(entityID, job.subnet, deviceIP, true)
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
		adIdentifier := config.ADIdentifier
		if adIdentifier == "" {
			adIdentifier = "snmp"
		}

		subnet := snmpSubnet{
			adIdentifier:   adIdentifier,
			config:         config,
			defaultParams:  defaultParams,
			startingIP:     startingIP,
			network:        *ipNet,
			cacheKey:       cacheKey,
			devices:        map[string]string{},
			deviceFailures: map[string]int{},
		}
		subnets = append(subnets, subnet)

		l.loadCache(subnet)
	}

	if l.config.Workers == 0 {
		l.config.Workers = 2
	}

	if l.config.AllowedFailures == 0 {
		l.config.AllowedFailures = 3
	}

	if l.config.DiscoveryInterval == 0 {
		l.config.DiscoveryInterval = 3600
	}

	jobs := make(chan snmpJob)
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

				jobIP := make(net.IP, len(currentIP))
				copy(jobIP, currentIP)
				job := snmpJob{
					subnet:    subnet,
					currentIP: jobIP,
				}
				jobs <- job

				select {
				case <-l.stop:
					return
				default:
				}
			}
			l.writeCache(subnet, true)
		}

		select {
		case <-l.stop:
			return
		case <-discoveryTicker.C:
		}
	}
}

func (l *SNMPListener) createService(entityID string, subnet snmpSubnet, deviceIP string, writeCache bool) {
	l.Lock()
	defer l.Unlock()
	if _, present := l.services[entityID]; present {
		return
	}
	svc := &SNMPService{
		adIdentifier: subnet.adIdentifier,
		entityID:     entityID,
		deviceIP:     deviceIP,
		creationTime: integration.Before,
		config:       subnet.config,
	}
	l.services[entityID] = svc
	subnet.devices[entityID] = deviceIP
	subnet.deviceFailures[entityID] = 0
	if writeCache {
		l.writeCache(subnet, false)
	}
	l.newService <- svc
}

func (l *SNMPListener) deleteService(entityID string, subnet snmpSubnet) {
	l.Lock()
	defer l.Unlock()
	if svc, present := l.services[entityID]; present {
		failure, present := subnet.deviceFailures[entityID]
		if !present {
			subnet.deviceFailures[entityID] = 1
			failure = 1
		} else {
			subnet.deviceFailures[entityID]++
			failure++
		}

		if l.config.AllowedFailures != -1 && failure >= l.config.AllowedFailures {
			l.delService <- svc
			delete(l.services, entityID)
			delete(subnet.devices, entityID)
			l.writeCache(subnet, false)
		}
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

// GetPorts returns the device port
func (s *SNMPService) GetPorts() ([]ContainerPort, error) {
	port := int(s.config.Port)
	if port == 0 {
		port = 161
	}
	return []ContainerPort{{port, fmt.Sprintf("p%d", port)}}, nil
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

// HasFilter returns false on SNMP
func (s *SNMPService) HasFilter(filter containers.FilterType) bool {
	return false
}

// GetSNMPInfo returns data from configuration
func (s *SNMPService) GetSNMPInfo(key string) (string, error) {
	switch key {
	case "version":
		return s.config.Version, nil
	case "timeout":
		return fmt.Sprintf("%d", s.config.Timeout), nil
	case "retries":
		return fmt.Sprintf("%d", s.config.Retries), nil
	case "community":
		return s.config.Community, nil
	case "user":
		return s.config.User, nil
	case "auth_key":
		return s.config.AuthKey, nil
	case "auth_protocol":
		if s.config.AuthProtocol == "MD5" {
			return "usmHMACMD5AuthProtocol", nil
		} else if s.config.AuthProtocol == "SHA" {
			return "usmHMACSHAAuthProtocol", nil
		}
		return "", nil
	case "priv_key":
		return s.config.PrivKey, nil
	case "priv_protocol":
		if s.config.PrivProtocol == "DES" {
			return "usmDESPrivProtocol", nil
		} else if s.config.PrivProtocol == "AES" {
			return "usmAesCfb128Protocol", nil
		}
		return "", nil
	case "context_engine_id":
		return s.config.ContextEngineID, nil
	case "context_name":
		return s.config.ContextName, nil
	}
	return "", ErrNotSupported
}
