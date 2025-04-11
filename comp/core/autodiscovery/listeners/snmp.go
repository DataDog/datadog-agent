// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package listeners

import (
	"context"
	"encoding/json"
	"expvar"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"hash/fnv"

	"github.com/gosnmp/gosnmp"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	"github.com/DataDog/datadog-agent/pkg/snmp"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const cacheKeyPrefix = "snmp"

const uptimeDiffTolerance = 50

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

// IPCounter interface defines operations for tracking IP authentication attempts
type IPCounter interface {
	Inc(ip string)
	Dec(ip string)
	Get(ip string) int
	Set(ip string, count int)
	Len() int
	GetAll() map[string]int
}

type DeviceHashInfo struct {
	Name        string
	Description string
	BootTimeMs  int64
	IP          string
}

// SNMPListener implements SNMP discovery
type SNMPListener struct {
	sync.RWMutex
	newService                      chan<- Service
	delService                      chan<- Service
	stop                            chan bool
	config                          snmp.ListenerConfig
	services                        map[string]*SNMPService
	devicesFoundByFullDeviceHash    map[string]DeviceHashInfo
	fullDeviceHashByBasicDeviceHash map[string][]string
	ipsCounter                      IPCounter
	pendingServicesByFullDeviceHash map[string]*pendingService
}

type ipAuthenticationCounter struct {
	sync.RWMutex
	counter map[string]int
}

func newIPAuthenticationCounter() *ipAuthenticationCounter {
	return &ipAuthenticationCounter{
		counter: make(map[string]int),
	}
}

func (c *ipAuthenticationCounter) Inc(ip string) {
	c.Lock()
	defer c.Unlock()
	c.counter[ip]++
}

func (c *ipAuthenticationCounter) Dec(ip string) {
	c.Lock()
	defer c.Unlock()
	c.counter[ip]--
}

func (c *ipAuthenticationCounter) Get(ip string) int {
	c.RLock()
	defer c.RUnlock()
	return c.counter[ip]
}

func (c *ipAuthenticationCounter) Set(ip string, count int) {
	c.Lock()
	defer c.Unlock()
	c.counter[ip] = count
}

func (c *ipAuthenticationCounter) Len() int {
	c.RLock()
	defer c.RUnlock()
	return len(c.counter)
}

func (c *ipAuthenticationCounter) GetAll() map[string]int {
	c.RLock()
	defer c.RUnlock()
	return c.counter
}

// SNMPService implements and store results from the Service interface for the SNMP listener
type SNMPService struct {
	adIdentifier string
	entityID     string
	deviceIP     string
	config       snmp.Config
	subnet       *snmpSubnet
}

// Make sure SNMPService implements the Service interface
var _ Service = &SNMPService{}

type pendingService struct {
	svc             *SNMPService
	basicDeviceHash string
	fullDeviceHash  string
	deviceHashInfo  DeviceHashInfo
	authIndex       int
	writeCache      bool
}

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
		services:                        map[string]*SNMPService{},
		stop:                            make(chan bool),
		config:                          snmpConfig,
		devicesFoundByFullDeviceHash:    map[string]DeviceHashInfo{},
		fullDeviceHashByBasicDeviceHash: map[string][]string{},
		pendingServicesByFullDeviceHash: map[string]*pendingService{},
		ipsCounter:                      newIPAuthenticationCounter(),
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
			l.createService(entityID, subnet, deviceIP.String(), 0, false)
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
		l.createService(entityID, subnet, device.IP.String(), device.AuthIndex, false)
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
		log.Debugf("Building SNMP params for device %s for authentication at index %d", deviceIP, authIndex)
		params, err := authentication.BuildSNMPParams(deviceIP, job.subnet.config.Port)
		if err != nil {
			log.Errorf("Error building params for device %s: %v", deviceIP, err)
			continue
		}

		deviceFound = l.checkDeviceForParams(params, deviceIP)

		l.ipsCounter.Dec(deviceIP)

		if l.ipsCounter.Get(deviceIP) == 0 {
			l.flushPendingServices()
		}

		if deviceFound {
			l.createService(entityID, job.subnet, deviceIP, authIndex, true)
			break
		}
	}
	if !deviceFound {
		l.deleteService(entityID, job.subnet)
	}

	autodiscoveryStatus := AutodiscoveryStatus{DevicesFoundList: l.getDevicesFoundInSubnet(*job.subnet), CurrentDevice: job.currentIP.String(), DevicesScannedCount: int(job.subnet.devicesScannedCounter.Inc())}
	autodiscoveryStatusBySubnetVar.Set(GetSubnetVarKey(job.subnet.config.Network, job.subnet.cacheKey), &autodiscoveryStatus)
}

func (l *SNMPListener) checkDeviceForParams(params *gosnmp.GoSNMP, deviceIP string) bool {
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

func (l *SNMPListener) getDeviceHash(authentication snmp.Authentication, subnet *snmpSubnet, deviceIP string) (string, string, DeviceHashInfo, error) {
	params, err := authentication.BuildSNMPParams(deviceIP, subnet.config.Port)
	if err != nil {
		return "", "", DeviceHashInfo{}, fmt.Errorf("error building SNMP params for device %s: %w", deviceIP, err)
	}

	if err := params.Connect(); err != nil {
		return "", "", DeviceHashInfo{}, fmt.Errorf("error connecting to device %s: %w", deviceIP, err)
	}

	defer params.Conn.Close()

	value, err := params.Get([]string{snmp.DeviceSysNameOid, snmp.DeviceSysDescrOid, snmp.DeviceSysUptimeOid, snmp.DeviceSysObjectIDOid})
	if err != nil {
		return "", "", DeviceHashInfo{}, fmt.Errorf("error getting system info from device %s: %w", deviceIP, err)
	}
	if len(value.Variables) < 4 || value.Variables[0].Value == nil || value.Variables[1].Value == nil || value.Variables[2].Value == nil || value.Variables[3].Value == nil {
		return "", "", DeviceHashInfo{}, fmt.Errorf("insufficient data received from device %s", deviceIP)
	}

	sysName := string(value.Variables[0].Value.([]byte))
	sysDescr := string(value.Variables[1].Value.([]byte))
	sysUptime := value.Variables[2].Value.(uint32)
	sysObjectID := value.Variables[3].Value.(string)

	log.Debugf("SNMP get sys infos to %s success: %s, %s, %d, %s", deviceIP, sysName, sysDescr, sysUptime, sysObjectID)

	// sysUptime is in hundredths of a second, convert it to milliseconds
	uptime := time.Duration(sysUptime*10) * time.Millisecond

	bootTimestamp := time.Now().Add(-uptime).UnixMilli()

	h := fnv.New64()
	h.Write([]byte(sysName))     //nolint:errcheck
	h.Write([]byte(sysObjectID)) //nolint:errcheck
	h.Write([]byte(sysDescr))    //nolint:errcheck

	basicDeviceHash := strconv.FormatUint(h.Sum64(), 16)

	h.Write([]byte(strconv.FormatInt(bootTimestamp, 10))) //nolint:errcheck

	fullDeviceHash := strconv.FormatUint(h.Sum64(), 16)

	return basicDeviceHash, fullDeviceHash, DeviceHashInfo{Name: sysName, Description: sysDescr, BootTimeMs: bootTimestamp, IP: deviceIP}, nil
}

func (l *SNMPListener) getDevicesFoundInSubnet(subnet snmpSubnet) []string {
	l.Lock()
	defer l.Unlock()

	ipsFound := []string{}
	for _, svc := range l.services {
		if svc.subnet.cacheKey == subnet.cacheKey {
			ipsFound = append(ipsFound, svc.deviceIP)
		}
	}
	return ipsFound
}

func parseCIDR(network string) (startingIP net.IP, ipNet *net.IPNet, err error) {
	ipAddr, ipNet, err := net.ParseCIDR(network)
	if err != nil {
		return nil, nil, fmt.Errorf("couldn't parse SNMP network: %w", err)
	}

	startingIP = ipAddr.Mask(ipNet.Mask)
	return startingIP, ipNet, nil
}

// forEachIP iterates through all IP addresses in a subnet and calls the provided function for each
// the provided function should return true to continue the loop and false to break it
func forEachIP(startingIP net.IP, ipNet *net.IPNet, f func(currentIP net.IP) bool) {
	currentIP := make(net.IP, len(startingIP))
	copy(currentIP, startingIP)

	for ; ipNet.Contains(currentIP); incrementIP(currentIP) {
		if !f(currentIP) {
			break
		}

		nextIP := make(net.IP, len(currentIP))
		copy(nextIP, currentIP)
		currentIP = nextIP
	}
}

func (l *SNMPListener) initializeIPAuthenticationCounter() {
	l.Lock()
	defer l.Unlock()

	for _, config := range l.config.Configs {
		startingIP, ipNet, err := parseCIDR(config.Network)
		if err != nil {
			log.Error(err)
			continue
		}

		forEachIP(startingIP, ipNet, func(currentIP net.IP) bool {
			if ignored := config.IsIPIgnored(currentIP); ignored {
				return true
			}
			count := l.ipsCounter.Get(currentIP.String())
			l.ipsCounter.Set(currentIP.String(), count+len(config.Authentications))

			return true
		})
	}

	log.Debugf("Initialized authentication counter with %d IP addresses", l.ipsCounter.Len())
}

func (l *SNMPListener) checkPreviousIPs(deviceIP string) bool {
	for ip, count := range l.ipsCounter.GetAll() {
		if count > 0 && minimumIP(ip, deviceIP) == ip {
			return false
		}
	}

	return true
}

func (l *SNMPListener) initializeSubnets() []snmpSubnet {
	subnets := []snmpSubnet{}

	for _, config := range l.config.Configs {
		startingIP, ipNet, err := parseCIDR(config.Network)
		if err != nil {
			log.Error(err)
			continue
		}

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
	l.initializeIPAuthenticationCounter()
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

			forEachIP(startingIP, &subnet.network, func(currentIP net.IP) bool {
				if ignored := subnet.config.IsIPIgnored(currentIP); ignored {
					return true
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
					return false
				default:
					return true
				}
			})

			select {
			case <-l.stop:
				return
			default:
			}
		}

		select {
		case <-l.stop:
			return
		case <-discoveryTicker.C:
		}
	}
}

func (l *SNMPListener) createService(entityID string, subnet *snmpSubnet, deviceIP string, authIndex int, writeCache bool) {
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

	basicDeviceHash, fullDeviceHash, deviceHashInfo, err := l.getDeviceHash(authentication, subnet, deviceIP)
	if err != nil {
		log.Errorf("Error getting device hash for device %s: %v", deviceIP, err)
		return
	}

	for _, fullHash := range l.fullDeviceHashByBasicDeviceHash[basicDeviceHash] {
		existing := l.devicesFoundByFullDeviceHash[fullHash]
		diff := math.Abs(float64(existing.BootTimeMs - deviceHashInfo.BootTimeMs))
		if diff <= float64(uptimeDiffTolerance) {
			log.Debugf("Device %s already discovered", deviceIP)
			return
		}
	}

	svc := &SNMPService{
		adIdentifier: subnet.adIdentifier,
		entityID:     entityID,
		deviceIP:     deviceIP,
		config:       config,
		subnet:       subnet,
	}

	previousIPsDiscovered := l.checkPreviousIPs(deviceIP)

	pendingSvc := &pendingService{
		svc:             svc,
		authIndex:       authIndex,
		writeCache:      writeCache,
		basicDeviceHash: basicDeviceHash,
		deviceHashInfo:  deviceHashInfo,
		fullDeviceHash:  fullDeviceHash,
	}

	if !previousIPsDiscovered {
		log.Debugf("Previous IPs not all scanned for device %s, adding to pending", deviceIP)


		log.Debugf("Checking hashes %v", l.fullDeviceHashByBasicDeviceHash)
		// check all devices with the same fuzzy hash (aka same name and description)
		for _, fullHash := range l.fullDeviceHashByBasicDeviceHash[basicDeviceHash] {

			if existingSvc, present := l.pendingServicesByFullDeviceHash[fullHash]; present {
				log.Debugf("Found existing pending service for device %s while checking %s", existingSvc.svc.deviceIP, deviceIP)

				// check time difference between the two devices
				diff := math.Abs(float64(existingSvc.deviceHashInfo.BootTimeMs - deviceHashInfo.BootTimeMs))
				if diff <= float64(uptimeDiffTolerance) {
					// check which device has the lowest IP
					minIP := minimumIP(existingSvc.svc.deviceIP, deviceIP)
					log.Debugf("Minimum IP between %s and %s is %s", existingSvc.svc.deviceIP, deviceIP, minIP)
					if minIP != deviceIP {
						return
					}
					// remove the other device from the pending services
					delete(l.pendingServicesByFullDeviceHash, fullHash)
				}
			}
		}

		l.pendingServicesByFullDeviceHash[fullDeviceHash] = pendingSvc
		l.fullDeviceHashByBasicDeviceHash[basicDeviceHash] = append(l.fullDeviceHashByBasicDeviceHash[basicDeviceHash], fullDeviceHash)

		return
	}

	l.registerService(pendingSvc)
}

func (l *SNMPListener) registerService(pendingSvc *pendingService) {
	l.devicesFoundByFullDeviceHash[pendingSvc.fullDeviceHash] = pendingSvc.deviceHashInfo
	l.fullDeviceHashByBasicDeviceHash[pendingSvc.basicDeviceHash] = append(l.fullDeviceHashByBasicDeviceHash[pendingSvc.basicDeviceHash], pendingSvc.fullDeviceHash)
	l.services[pendingSvc.svc.entityID] = pendingSvc.svc
	pendingSvc.svc.subnet.devices[pendingSvc.svc.entityID] = device{
		IP:        net.ParseIP(pendingSvc.svc.deviceIP),
		AuthIndex: pendingSvc.authIndex,
	}
	pendingSvc.svc.subnet.deviceFailures[pendingSvc.svc.entityID] = 0
	if pendingSvc.writeCache {
		l.writeCache(pendingSvc.svc.subnet)
	}
	l.newService <- pendingSvc.svc
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

func minimumIP(ipStr1, ipStr2 string) string {
	ip1 := net.ParseIP(ipStr1)
	ip2 := net.ParseIP(ipStr2)

	if ip1 == nil || ip2 == nil {
		return ""
	}

	for i := range ip1 {
		if ip1[i] < ip2[i] {
			return ip1.String()
		} else if ip1[i] > ip2[i] {
			return ip2.String()
		}
	}
	return ip1.String()
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

// GetSubnetVarKey returns a key for a subnet in the expvar map
func GetSubnetVarKey(network string, cacheKey string) string {
	return fmt.Sprintf("%s|%s", network, strings.Trim(cacheKey, fmt.Sprintf("%s:", cacheKeyPrefix)))
}

func (l *SNMPListener) flushPendingServices() {
	for _, pendingSvc := range l.pendingServicesByFullDeviceHash {
		log.Debugf("Checking pending service for device %s", pendingSvc.svc.deviceIP)
		previousIPsScanned := l.checkPreviousIPs(pendingSvc.svc.deviceIP)

		if previousIPsScanned {
			log.Debugf("All previous IPs scanned for device %s, activating service", pendingSvc.svc.deviceIP)

			l.registerService(pendingSvc)
			delete(l.pendingServicesByFullDeviceHash, pendingSvc.basicDeviceHash)
		}
	}
}
