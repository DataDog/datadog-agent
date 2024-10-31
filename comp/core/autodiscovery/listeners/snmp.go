// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package listeners

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	"github.com/DataDog/datadog-agent/pkg/snmp"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultWorkers           = 2
	defaultAllowedFailures   = 3
	defaultDiscoveryInterval = 3600
	tagSeparator             = ","
)

// SNMPListener implements SNMP discovery
type SNMPListener struct {
	sync.RWMutex
	newService chan<- Service
	delService chan<- Service
	stop       chan bool
	config     snmp.ListenerConfig
	services   map[string]Service
}

// SNMPService implements and store results from the Service interface for the SNMP listener
type SNMPService struct {
	adIdentifier string
	entityID     string
	deviceIP     string
	config       snmp.Config
}

// Make sure SNMPService implements the Service interface
var _ Service = &SNMPService{}

type snmpSubnet struct {
	adIdentifier   string
	config         snmp.Config
	startingIP     net.IP
	network        net.IPNet
	cacheKey       string
	devices        map[string]string
	deviceFailures map[string]int
}

type snmpJob struct {
	subnet    *snmpSubnet
	currentIP net.IP
}

// NewSNMPListener creates a SNMPListener
func NewSNMPListener(ServiceListernerDeps) (ServiceListener, error) {
	snmpConfig, err := snmp.NewListenerConfig()
	if err != nil {
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

func (l *SNMPListener) loadCache(subnet *snmpSubnet) {
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

func (l *SNMPListener) writeCache(subnet *snmpSubnet) {
	// We don't lock the subnet for now, because the listener ought to be already locked
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
	deviceIP := job.currentIP.String()
	params, err := job.subnet.config.BuildSNMPParams(deviceIP)
	if err != nil {
		log.Errorf("Error building params for device %s: %v", deviceIP, err)
		return
	}
	entityID := job.subnet.config.Digest(deviceIP)
	if err := params.Connect(); err != nil {
		log.Debugf("SNMP connect to %s error: %v", deviceIP, err)
		l.deleteService(entityID, job.subnet)
	} else {
		defer params.Conn.Close()

		// Since `params<GoSNMP>.ContextEngineID` is empty
		// `params.GetNext` might lead to multiple SNMP GET calls when using SNMP v3
		value, err := params.GetNext([]string{snmp.DeviceReachableGetNextOid})
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
			startingIP:     startingIP,
			network:        *ipNet,
			cacheKey:       cacheKey,
			devices:        map[string]string{},
			deviceFailures: map[string]int{},
		}
		subnets = append(subnets, subnet)

		l.loadCache(&subnet)
	}

	if l.config.Workers == 0 {
		l.config.Workers = defaultWorkers
	}

	if l.config.AllowedFailures == 0 {
		l.config.AllowedFailures = defaultAllowedFailures
	}

	if l.config.DiscoveryInterval == 0 {
		l.config.DiscoveryInterval = defaultDiscoveryInterval
	}

	jobs := make(chan snmpJob)
	for w := 0; w < l.config.Workers; w++ {
		go worker(l, jobs)
	}

	discoveryTicker := time.NewTicker(time.Duration(l.config.DiscoveryInterval) * time.Second)
	defer discoveryTicker.Stop()
	for {
		var subnet *snmpSubnet
		for i := range subnets {
			// Use `&subnets[i]` to pass the correct pointer address to snmpJob{}
			subnet = &subnets[i]
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
		}

		select {
		case <-l.stop:
			return
		case <-discoveryTicker.C:
		}
	}
}

func (l *SNMPListener) createService(entityID string, subnet *snmpSubnet, deviceIP string, writeCache bool) {
	l.Lock()
	defer l.Unlock()
	if _, present := l.services[entityID]; present {
		return
	}
	svc := &SNMPService{
		adIdentifier: subnet.adIdentifier,
		entityID:     entityID,
		deviceIP:     deviceIP,
		config:       subnet.config,
	}
	l.services[entityID] = svc
	subnet.devices[entityID] = deviceIP
	subnet.deviceFailures[entityID] = 0
	if writeCache {
		l.writeCache(subnet)
	}
	l.newService <- svc
}

func (l *SNMPListener) deleteService(entityID string, subnet *snmpSubnet) {
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
			l.writeCache(subnet)
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

// Equal returns whether the two SNMPService are equal
func (s *SNMPService) Equal(o Service) bool {
	s2, ok := o.(*SNMPService)
	if !ok {
		return false
	}

	return s.entityID == s2.entityID &&
		s.deviceIP == s2.deviceIP &&
		s.config.Port == s2.config.Port &&
		s.adIdentifier == s2.adIdentifier
}

// GetServiceID returns the unique entity ID linked to that service
func (s *SNMPService) GetServiceID() string {
	return s.entityID
}

// GetADIdentifiers returns a set of AD identifiers
func (s *SNMPService) GetADIdentifiers(context.Context) ([]string, error) {
	return []string{s.adIdentifier}, nil
}

// GetHosts returns the device IP
func (s *SNMPService) GetHosts(context.Context) (map[string]string, error) {
	ips := map[string]string{
		"": s.deviceIP,
	}
	return ips, nil
}

// GetPorts returns the device port
func (s *SNMPService) GetPorts(context.Context) ([]ContainerPort, error) {
	port := int(s.config.Port)
	return []ContainerPort{{port, fmt.Sprintf("p%d", port)}}, nil
}

// GetTags returns the list of container tags - currently always empty
func (s *SNMPService) GetTags() ([]string, error) {
	return []string{}, nil
}

// GetTagsWithCardinality returns the tags with given cardinality.
func (s *SNMPService) GetTagsWithCardinality(_ string) ([]string, error) {
	return s.GetTags()
}

// GetPid returns nil and an error because pids are currently not supported
func (s *SNMPService) GetPid(context.Context) (int, error) {
	return -1, ErrNotSupported
}

// GetHostname returns nothing - not supported
func (s *SNMPService) GetHostname(context.Context) (string, error) {
	return "", ErrNotSupported
}

// IsReady returns true
func (s *SNMPService) IsReady(context.Context) bool {
	return true
}

// HasFilter returns false on SNMP
//
//nolint:revive // TODO(NDM) Fix revive linter
func (s *SNMPService) HasFilter(_ containers.FilterType) bool {
	return false
}

// GetExtraConfig returns data from configuration
func (s *SNMPService) GetExtraConfig(key string) (string, error) {
	switch key {
	case "version":
		return s.config.Version, nil
	case "timeout":
		return fmt.Sprintf("%d", s.config.Timeout), nil
	case "retries":
		return fmt.Sprintf("%d", s.config.Retries), nil
	case "oid_batch_size":
		return fmt.Sprintf("%d", s.config.OidBatchSize), nil
	case "community":
		return s.config.Community, nil
	case "user":
		return s.config.User, nil
	case "auth_key":
		return s.config.AuthKey, nil
	case "auth_protocol":
		return s.config.AuthProtocol, nil
	case "priv_key":
		return s.config.PrivKey, nil
	case "priv_protocol":
		return s.config.PrivProtocol, nil
	case "context_engine_id":
		return s.config.ContextEngineID, nil
	case "context_name":
		return s.config.ContextName, nil
	case "autodiscovery_subnet":
		return s.config.Network, nil
	case "loader":
		return s.config.Loader, nil
	case "namespace":
		return s.config.Namespace, nil
	case "collect_device_metadata":
		return strconv.FormatBool(s.config.CollectDeviceMetadata), nil
	case "collect_topology":
		return strconv.FormatBool(s.config.CollectTopology), nil
	case "use_device_id_as_hostname":
		return strconv.FormatBool(s.config.UseDeviceIDAsHostname), nil
	case "tags":
		return convertToCommaSepTags(s.config.Tags), nil
	case "min_collection_interval":
		return fmt.Sprintf("%d", s.config.MinCollectionInterval), nil
	case "interface_configs":
		ifConfigs := s.config.InterfaceConfigs[s.deviceIP]
		if len(ifConfigs) == 0 {
			return "", nil
		}
		//nolint:revive // TODO(NDM) Fix revive linter
		ifConfigsJson, err := json.Marshal(ifConfigs)
		if err != nil {
			return "", fmt.Errorf("error marshalling interface_configs: %s", err)
		}
		return string(ifConfigsJson), nil
	case "ping":
		pingConfig := s.config.PingConfig

		pingCfgJSON, err := json.Marshal(pingConfig)
		if err != nil {
			return "", fmt.Errorf("error marshalling ping config: %s", err)
		}

		return string(pingCfgJSON), nil
	}
	return "", ErrNotSupported
}

// FilterTemplates does nothing.
//
//nolint:revive // TODO(NDM) Fix revive linter
func (s *SNMPService) FilterTemplates(_ map[string]integration.Config) {
}

func convertToCommaSepTags(tags []string) string {
	normalizedTags := make([]string, 0, len(tags))
	for _, tag := range tags {
		// Convert comma `,` to `_` since comma is used as separator.
		// `,` is not an allowed character for tags and will be converted to `_` by backend anyway,
		// so, converting `,` to `_` shouldn't have any impact.
		normalizedTags = append(normalizedTags, strings.ReplaceAll(tag, tagSeparator, "_"))
	}
	return strings.Join(normalizedTags, tagSeparator)
}
