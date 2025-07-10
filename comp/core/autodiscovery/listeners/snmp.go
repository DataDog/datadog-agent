// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package listeners

import (
	"encoding/json"
	"expvar"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	filter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	"github.com/DataDog/datadog-agent/pkg/snmp"
	"github.com/DataDog/datadog-agent/pkg/snmp/devicededuper"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const cacheKeyPrefix = "snmp"

var (
	autodiscoveryStatusBySubnetVar = expvar.NewMap("snmpAutodiscovery")
)

// AutodiscoveryStatus represents the status of the autodiscovery of a subnet we want to expose in the snmp status
type AutodiscoveryStatus struct {
	DevicesFoundList    []string
	CurrentDevice       string
	DevicesScannedCount int
}

func (s *AutodiscoveryStatus) String() string {
	jsonData, err := json.Marshal(s)
	if err != nil {
		return ""
	}
	return string(jsonData)
}

const (
	defaultWorkers           = 2
	defaultAllowedFailures   = 3
	defaultDiscoveryInterval = 3600
	tagSeparator             = ","
)

// SNMPListener implements SNMP discovery
type SNMPListener struct {
	sync.RWMutex
	newService    chan<- Service
	delService    chan<- Service
	stop          chan bool
	config        snmp.ListenerConfig
	services      map[string]*SNMPService
	deviceDeduper devicededuper.DeviceDeduper
}

// SNMPService implements and store results from the Service interface for the SNMP listener
type SNMPService struct {
	adIdentifier string
	entityID     string
	deviceIP     string
	config       snmp.Config
	subnet       *snmpSubnet
	pending      bool
}

// Make sure SNMPService implements the Service interface
var _ Service = &SNMPService{}

type snmpSubnet struct {
	adIdentifier          string
	config                snmp.Config
	startingIP            net.IP
	network               net.IPNet
	cacheKey              string
	devices               map[string]device
	deviceFailures        map[string]int
	devicesScannedCounter atomic.Uint32
}

type device struct {
	IP        net.IP `json:"ip"`
	AuthIndex int    `json:"auth_index"`
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
		services:      map[string]*SNMPService{},
		stop:          make(chan bool),
		config:        snmpConfig,
		deviceDeduper: devicededuper.NewDeviceDeduper(snmpConfig),
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

	// Try to unmarshal with the old cache format
	var deviceIPs []net.IP
	err = json.Unmarshal([]byte(cacheValue), &deviceIPs)
	if err == nil {
		for _, deviceIP := range deviceIPs {
			entityID := subnet.config.Digest(deviceIP.String())
			deviceInfo := l.checkDeviceInfo(subnet.config.Authentications[0], subnet.config.Port, deviceIP.String())

			l.createService(entityID, subnet, deviceIP.String(), deviceInfo, 0, false)
		}
		return
	}

	var devices []device
	err = json.Unmarshal([]byte(cacheValue), &devices)
	if err != nil {
		log.Errorf("Couldn't unmarshal cache for %s: %s", subnet.cacheKey, err)
		return
	}
	for _, device := range devices {
		entityID := subnet.config.Digest(device.IP.String())
		deviceInfo := l.checkDeviceInfo(subnet.config.Authentications[device.AuthIndex], subnet.config.Port, device.IP.String())

		l.createService(entityID, subnet, device.IP.String(), deviceInfo, device.AuthIndex, false)
	}
}

func (l *SNMPListener) writeCache(subnet *snmpSubnet) {
	// We don't lock the subnet for now, because the listener ought to be already locked
	devices := make([]device, 0, len(subnet.devices))
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
	entityID := job.subnet.config.Digest(deviceIP)

	deviceFound := false
	for authIndex, authentication := range job.subnet.config.Authentications {
		deviceFound = l.checkDeviceReachable(authentication, job.subnet.config.Port, deviceIP)

		l.deviceDeduper.MarkIPAsProcessed(deviceIP)
		l.registerDedupedDevices()

		if !deviceFound {
			continue
		}

		deviceInfo := l.checkDeviceInfo(authentication, job.subnet.config.Port, deviceIP)

		if deviceFound {
			l.createService(entityID, job.subnet, deviceIP, deviceInfo, authIndex, true)
			break
		}
	}
	if !deviceFound {
		l.deleteService(entityID, job.subnet)
	}

	autodiscoveryStatus := AutodiscoveryStatus{DevicesFoundList: l.getDevicesFoundInSubnet(*job.subnet), CurrentDevice: job.currentIP.String(), DevicesScannedCount: int(job.subnet.devicesScannedCounter.Inc())}
	autodiscoveryStatusBySubnetVar.Set(GetSubnetVarKey(job.subnet.config.Network, job.subnet.cacheKey), &autodiscoveryStatus)
}

func (l *SNMPListener) checkDeviceReachable(authentication snmp.Authentication, port uint16, deviceIP string) bool {
	params, err := authentication.BuildSNMPParams(deviceIP, port)
	if err != nil {
		log.Errorf("Error building params for device %s: %v", deviceIP, err)
		return false
	}

	if err := params.Connect(); err != nil {
		log.Debugf("SNMP connect to %s error: %v", deviceIP, err)
		return false
	}

	defer params.Conn.Close()

	// Since `params<GoSNMP>.ContextEngineID` is empty
	// `params.GetNext` might lead to multiple SNMP GET calls when using SNMP v3
	value, err := params.GetNext([]string{snmp.DeviceReachableGetNextOid})
	if err != nil {
		log.Debugf("SNMP get to %s error: %v", deviceIP, err)
		return false
	}
	if len(value.Variables) < 1 || value.Variables[0].Value == nil {
		log.Debugf("SNMP get to %s no data", deviceIP)
		return false
	}

	log.Debugf("SNMP get to %s success: %v", deviceIP, value.Variables[0].Value)

	return true
}

func (l *SNMPListener) checkDeviceInfo(authentication snmp.Authentication, port uint16, deviceIP string) devicededuper.DeviceInfo {
	if !l.config.Deduplicate {
		return devicededuper.DeviceInfo{}
	}

	params, err := authentication.BuildSNMPParams(deviceIP, port)
	if err != nil {
		log.Errorf("Error building params for device %s: %v", deviceIP, err)
		return devicededuper.DeviceInfo{}
	}

	if err := params.Connect(); err != nil {
		log.Debugf("SNMP connect to %s error: %v", deviceIP, err)
		return devicededuper.DeviceInfo{}
	}

	defer params.Conn.Close()
	value, err := params.Get([]string{snmp.DeviceSysNameOid, snmp.DeviceSysDescrOid, snmp.DeviceSysUptimeOid, snmp.DeviceSysObjectIDOid})
	if err != nil {
		return devicededuper.DeviceInfo{}
	}
	if len(value.Variables) < 4 || value.Variables[0].Value == nil || value.Variables[1].Value == nil || value.Variables[2].Value == nil || value.Variables[3].Value == nil {
		return devicededuper.DeviceInfo{}
	}

	sysNameBytes, ok := extractSNMPValue[[]byte](value.Variables[0].Value)
	if !ok {
		return devicededuper.DeviceInfo{}
	}
	sysName := string(sysNameBytes)

	sysDescrBytes, ok := extractSNMPValue[[]byte](value.Variables[1].Value)
	if !ok {
		return devicededuper.DeviceInfo{}
	}
	sysDescr := string(sysDescrBytes)

	sysUptime, ok := extractSNMPValue[uint32](value.Variables[2].Value)
	if !ok {
		return devicededuper.DeviceInfo{}
	}

	sysObjectID, ok := extractSNMPValue[string](value.Variables[3].Value)
	if !ok {
		return devicededuper.DeviceInfo{}
	}

	// sysUptime is in hundredths of a second, convert it to milliseconds
	uptime := time.Duration(sysUptime*10) * time.Millisecond

	bootTimestamp := time.Now().Add(-uptime).UnixMilli()

	return devicededuper.DeviceInfo{Name: sysName, Description: sysDescr, BootTimeMs: bootTimestamp, SysObjectID: sysObjectID}
}

func (l *SNMPListener) getDevicesFoundInSubnet(subnet snmpSubnet) []string {
	l.Lock()
	defer l.Unlock()

	ipsFound := []string{}
	for _, svc := range l.services {
		if svc.subnet.cacheKey == subnet.cacheKey && !svc.pending {
			ipsFound = append(ipsFound, svc.deviceIP)
		}
	}
	return ipsFound
}

func (l *SNMPListener) initializeSubnets() []snmpSubnet {
	subnets := []snmpSubnet{}
	for _, config := range l.config.Configs {
		ipAddr, ipNet, err := net.ParseCIDR(config.Network)
		if err != nil {
			log.Errorf("Couldn't parse SNMP network: %s", err)
			continue
		}

		startingIP := ipAddr.Mask(ipNet.Mask)

		configHash := config.Digest(config.Network)
		cacheKey := fmt.Sprintf("%s:%s", cacheKeyPrefix, configHash)
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
			devices:        map[string]device{},
			deviceFailures: map[string]int{},
		}
		subnets = append(subnets, subnet)

		l.loadCache(&subnet)
	}

	return subnets
}

func (l *SNMPListener) checkDevices() {
	subnets := l.initializeSubnets()

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
		for _, subnet := range subnets {
			autodiscoveryStatusBySubnetVar.Set(GetSubnetVarKey(subnet.config.Network, subnet.cacheKey), &expvar.String{})
		}

		var subnet *snmpSubnet
		for i := range subnets {
			// Use `&subnets[i]` to pass the correct pointer address to snmpJob{}
			subnet = &subnets[i]
			subnet.devicesScannedCounter.Store(uint32(len(subnet.config.IgnoredIPAddresses)))
			startingIP := make(net.IP, len(subnet.startingIP))
			copy(startingIP, subnet.startingIP)
			for currentIP := startingIP; subnet.network.Contains(currentIP); devicededuper.IncrementIP(currentIP) {

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

func (l *SNMPListener) createService(entityID string, subnet *snmpSubnet, deviceIP string, deviceInfo devicededuper.DeviceInfo, authIndex int, writeCache bool) {
	l.Lock()
	defer l.Unlock()
	if _, present := l.services[entityID]; present {
		return
	}

	config := subnet.config
	if authIndex < 0 || authIndex >= len(config.Authentications) {
		log.Errorf("Invalid authentication index %d for device %s (max: %d)", authIndex, deviceIP, len(config.Authentications)-1)
		return
	}
	authentication := config.Authentications[authIndex]
	config.Version = authentication.Version
	config.Timeout = authentication.Timeout
	config.Retries = authentication.Retries
	config.Community = authentication.Community
	config.User = authentication.User
	config.AuthKey = authentication.AuthKey
	config.AuthProtocol = authentication.AuthProtocol
	config.PrivKey = authentication.PrivKey
	config.PrivProtocol = authentication.PrivProtocol
	config.ContextEngineID = authentication.ContextEngineID
	config.ContextName = authentication.ContextName

	svc := SNMPService{
		adIdentifier: subnet.adIdentifier,
		entityID:     entityID,
		deviceIP:     deviceIP,
		config:       config,
		subnet:       subnet,
		pending:      true,
	}
	l.services[entityID] = &svc

	pendingDevice := devicededuper.PendingDevice{
		Config:     config,
		Info:       deviceInfo,
		AuthIndex:  authIndex,
		WriteCache: writeCache,
		IP:         deviceIP,
	}

	if deviceInfo == (devicededuper.DeviceInfo{}) {
		l.registerService(pendingDevice)
		return
	}

	l.deviceDeduper.AddPendingDevice(pendingDevice)
}

func (l *SNMPListener) registerDedupedDevices() {
	if !l.config.Deduplicate {
		return
	}
	for _, pendingSvc := range l.deviceDeduper.GetDedupedDevices() {
		l.registerService(pendingSvc)
	}
}

func (l *SNMPListener) registerService(pendingDevice devicededuper.PendingDevice) {
	entityID := pendingDevice.Config.Digest(pendingDevice.IP)

	svc, ok := l.services[entityID]
	if !ok {
		return
	}
	svc.pending = false

	svc.subnet.devices[svc.entityID] = device{
		IP:        net.ParseIP(svc.deviceIP),
		AuthIndex: pendingDevice.AuthIndex,
	}
	svc.subnet.deviceFailures[svc.entityID] = 0
	if pendingDevice.WriteCache {
		l.writeCache(svc.subnet)
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
func (s *SNMPService) GetADIdentifiers() []string {
	return []string{s.adIdentifier}
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
func (s *SNMPService) GetPid() (int, error) {
	return -1, ErrNotSupported
}

// GetHostname returns nothing - not supported
func (s *SNMPService) GetHostname() (string, error) {
	return "", ErrNotSupported
}

// IsReady returns true
func (s *SNMPService) IsReady() bool {
	return true
}

// HasFilter returns false on SNMP
func (s *SNMPService) HasFilter(_ filter.Scope) bool {
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
	case "collect_vpn":
		return strconv.FormatBool(s.config.CollectVPN), nil
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
		ifConfigsJSON, err := json.Marshal(ifConfigs)
		if err != nil {
			return "", fmt.Errorf("error marshalling interface_configs: %s", err)
		}
		return string(ifConfigsJSON), nil
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

// GetSubnetVarKey returns a key for a subnet in the expvar map
func GetSubnetVarKey(network string, cacheKey string) string {
	return fmt.Sprintf("%s|%s", network, strings.Trim(cacheKey, fmt.Sprintf("%s:", cacheKeyPrefix)))
}

func extractSNMPValue[T any](value interface{}) (T, bool) {
	typedValue, ok := value.(T)
	return typedValue, ok
}
