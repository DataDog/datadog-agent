// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package listeners

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/listeners/listeners_interfaces"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	"github.com/DataDog/datadog-agent/pkg/snmp"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
// defaultWorkers           = 2
// defaultAllowedFailures   = 3
// defaultDiscoveryInterval = 3600
// tagSeparator             = ","
)

func init() {
	Register("distributed_checks", NewDistributedChecksListener)
}

// DistributedChecksListener implements SNMP discovery
type DistributedChecksListener struct {
	sync.RWMutex
	newService chan<- cprofstruct.Service
	delService chan<- cprofstruct.Service
	stop       chan bool
	config     snmp.ListenerConfig
	services   map[string]cprofstruct.Service
}

// DistributedCheckService implements and store results from the Service interface for the SNMP listener
type DistributedCheckService struct {
	adIdentifier string
	entityID     string
	deviceIP     string
	config       snmp.Config
}

// Make sure DistributedCheckService implements the Service interface
var _ cprofstruct.Service = &DistributedCheckService{}

// NewDistributedChecksListener creates a DistributedChecksListener
func NewDistributedChecksListener(cprofstruct.Config) (cprofstruct.ServiceListener, error) {
	log.Info("[DistributedChecksListener] NewDistributedChecksListener")
	snmpConfig, err := snmp.NewListenerConfig()
	if err != nil {
		return nil, err
	}
	return &DistributedChecksListener{
		services: map[string]cprofstruct.Service{},
		stop:     make(chan bool),
		config:   snmpConfig,
	}, nil
}

// Listen periodically refreshes devices
func (l *DistributedChecksListener) Listen(newSvc chan<- cprofstruct.Service, delSvc chan<- cprofstruct.Service) {
	// setup the I/O channels
	l.newService = newSvc
	l.delService = delSvc

	go l.checkDevices()
}

func (l *DistributedChecksListener) loadCache(subnet *snmpSubnet) {
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

func (l *DistributedChecksListener) writeCache(subnet *snmpSubnet) {
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

func (l *DistributedChecksListener) checkDevices() {

	discoveryTicker := time.NewTicker(time.Duration(l.config.DiscoveryInterval) * time.Second)

	for {
		log.Info("[DistributedChecksListener] run")

		select {
		case <-l.stop:
			return
		case <-discoveryTicker.C:
		}
	}
}

func (l *DistributedChecksListener) createService(entityID string, subnet *snmpSubnet, deviceIP string, writeCache bool) {
	l.Lock()
	defer l.Unlock()
	if _, present := l.services[entityID]; present {
		return
	}
	svc := &DistributedCheckService{
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

func (l *DistributedChecksListener) deleteService(entityID string, subnet *snmpSubnet) {
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

// Stop queues a shutdown of DistributedChecksListener
func (l *DistributedChecksListener) Stop() {
	l.stop <- true
}

// GetServiceID returns the unique entity ID linked to that service
func (s *DistributedCheckService) GetServiceID() string {
	return s.entityID
}

// GetTaggerEntity returns the unique entity ID linked to that service
func (s *DistributedCheckService) GetTaggerEntity() string {
	return s.entityID
}

// GetADIdentifiers returns a set of AD identifiers
func (s *DistributedCheckService) GetADIdentifiers(context.Context) ([]string, error) {
	return []string{s.adIdentifier}, nil
}

// GetHosts returns the device IP
func (s *DistributedCheckService) GetHosts(context.Context) (map[string]string, error) {
	ips := map[string]string{
		"": s.deviceIP,
	}
	return ips, nil
}

// GetPorts returns the device port
func (s *DistributedCheckService) GetPorts(context.Context) ([]cprofstruct.ContainerPort, error) {
	port := int(s.config.Port)
	return []cprofstruct.ContainerPort{{port, fmt.Sprintf("p%d", port)}}, nil
}

// GetTags returns the list of container tags - currently always empty
func (s *DistributedCheckService) GetTags() ([]string, error) {
	return []string{}, nil
}

// GetPid returns nil and an error because pids are currently not supported
func (s *DistributedCheckService) GetPid(context.Context) (int, error) {
	return -1, ErrNotSupported
}

// GetHostname returns nothing - not supported
func (s *DistributedCheckService) GetHostname(context.Context) (string, error) {
	return "", ErrNotSupported
}

// IsReady returns true
func (s *DistributedCheckService) IsReady(context.Context) bool {
	return true
}

// GetCheckNames returns nil
func (s *DistributedCheckService) GetCheckNames(context.Context) []string {
	return nil
}

// HasFilter returns false on SNMP
func (s *DistributedCheckService) HasFilter(filter containers.FilterType) bool {
	return false
}

// GetExtraConfig returns data from configuration
func (s *DistributedCheckService) GetExtraConfig(key string) (string, error) {
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
		ifConfigsJson, err := json.Marshal(ifConfigs)
		if err != nil {
			return "", fmt.Errorf("error marshalling interface_configs: %s", err)
		}
		return string(ifConfigsJson), nil
	}
	return "", ErrNotSupported
}

// FilterTemplates does nothing.
func (s *DistributedCheckService) FilterTemplates(configs map[string]integration.Config) {
}
