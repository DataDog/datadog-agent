// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(NDM) Fix revive linter
package discovery

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/devicecheck"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
)

const cacheKeyPrefix = "snmp"
const sysObjectIDOid = "1.3.6.1.2.1.1.2.0"

// Discovery handles snmp discovery states
type Discovery struct {
	config    *checkconfig.CheckConfig
	stop      chan struct{}
	discDevMu sync.RWMutex

	// TODO: use a new type for device deviceDigest
	// discoveredDevices contains devices with device deviceDigest as map key
	// see also CheckConfig.DeviceDigest()
	discoveredDevices map[checkconfig.DeviceDigest]Device

	sessionFactory session.Factory
}

// Device implements and store results from the Service interface for the SNMP listener
type Device struct {
	deviceDigest checkconfig.DeviceDigest
	deviceIP     string
	deviceCheck  *devicecheck.DeviceCheck
}
type snmpSubnet struct {
	config     *checkconfig.CheckConfig
	startingIP net.IP
	network    net.IPNet

	cacheKey string

	// discoveredDevices contains devices ip with device deviceDigest as map key
	// see also CheckConfig.DeviceDigest()
	devices map[checkconfig.DeviceDigest]string

	// discoveredDevices contains device failures count with device deviceDigest as map key
	// see also CheckConfig.DeviceDigest()
	deviceFailures map[checkconfig.DeviceDigest]int
}

type checkDeviceJob struct {
	subnet    *snmpSubnet
	currentIP net.IP
}

// Start discovery
func (d *Discovery) Start() {
	log.Debugf("subnet %s: Start discovery", d.config.Network)
	go d.discoverDevices()
}

// Stop signal discovery to shut down
func (d *Discovery) Stop() {
	log.Debugf("subnet %s: Stop discovery", d.config.Network)
	close(d.stop)
}

// GetDiscoveredDeviceConfigs returns discovered device configs
func (d *Discovery) GetDiscoveredDeviceConfigs() []*devicecheck.DeviceCheck {
	d.discDevMu.RLock()
	defer d.discDevMu.RUnlock()

	discoveredDevices := make([]*devicecheck.DeviceCheck, 0, len(d.discoveredDevices))
	for _, device := range d.discoveredDevices {
		discoveredDevices = append(discoveredDevices, device.deviceCheck)
	}
	return discoveredDevices
}

// Start discovery
func (d *Discovery) runWorker(w int, jobs <-chan checkDeviceJob) {
	log.Debugf("subnet %s: Start SNMP worker %d", d.config.Network, w)
	for {
		select {
		case <-d.stop:
			log.Debugf("subnet %s: Stop SNMP worker %d", d.config.Network, w)
			return
		case job := <-jobs:
			log.Debugf("subnet %s: Handling IP %s", d.config.Network, job.currentIP.String())
			err := d.checkDevice(job)
			if err != nil {
				log.Errorf(err.Error())
			}
		}
	}
}

func (d *Discovery) discoverDevices() {
	ipAddr, ipNet, err := net.ParseCIDR(d.config.Network)
	if err != nil {
		log.Errorf("subnet %s: Couldn't parse SNMP network: %s", d.config.Network, err)
		return
	}

	startingIP := ipAddr.Mask(ipNet.Mask)

	configHash := d.config.DeviceDigest(d.config.Network)
	cacheKey := fmt.Sprintf("%s:%s", cacheKeyPrefix, configHash)

	subnet := snmpSubnet{
		config:     d.config,
		startingIP: startingIP,
		network:    *ipNet,
		cacheKey:   cacheKey,

		// Since subnet devices fields (`devices` and `deviceFailures`) are changed at the same time
		// as Discovery.discoveredDevices, we rely on Discovery.discDevMu mutex to protect against concurrent changes.
		devices:        map[checkconfig.DeviceDigest]string{},
		deviceFailures: map[checkconfig.DeviceDigest]int{},
	}

	d.loadCache(&subnet)

	jobs := make(chan checkDeviceJob)
	for w := 0; w < d.config.DiscoveryWorkers; w++ {
		go d.runWorker(w, jobs)
	}

	discoveryTicker := time.NewTicker(time.Duration(d.config.DiscoveryInterval) * time.Second)

	for {
		log.Debugf("subnet %s: Run discovery", d.config.Network)
		startingIP := make(net.IP, len(subnet.startingIP))
		copy(startingIP, subnet.startingIP)
		for currentIP := startingIP; subnet.network.Contains(currentIP); incrementIP(currentIP) {

			if ignored := subnet.config.IsIPIgnored(currentIP); ignored {
				continue
			}

			jobIP := make(net.IP, len(currentIP))
			copy(jobIP, currentIP)
			job := checkDeviceJob{
				subnet:    &subnet,
				currentIP: jobIP,
			}
			jobs <- job

			select {
			case <-d.stop:
				log.Debugf("subnet %s: Stop scheduling devices", d.config.Network)
				return
			default:
			}
		}

		select {
		case <-d.stop:
			log.Debugf("subnet %s: Stop scheduling devices", d.config.Network)
			return
		case <-discoveryTicker.C:
		}
	}
}

func (d *Discovery) checkDevice(job checkDeviceJob) error {
	deviceIP := job.currentIP.String()
	config := *job.subnet.config // shallow copy
	config.IPAddress = deviceIP
	sess, err := d.sessionFactory(&config)
	if err != nil {
		return fmt.Errorf("error configure session for ip %s: %v", deviceIP, err)
	}
	deviceDigest := job.subnet.config.DeviceDigest(deviceIP)
	if err := sess.Connect(); err != nil {
		log.Debugf("subnet %s: SNMP connect to %s error: %v", d.config.Network, deviceIP, err)
		d.deleteDevice(deviceDigest, job.subnet)
	} else {
		defer sess.Close()

		oids := []string{sysObjectIDOid}
		// Since `params<GoSNMP>.ContextEngineID` is empty
		// `params.Get` might lead to multiple SNMP GET calls when using SNMP v3
		// a first call might be needed to retrieve the engineID and then the call to get the oid values.
		value, err := sess.Get(oids)
		if err != nil {
			log.Debugf("subnet %s: SNMP get to %s error: %v", d.config.Network, deviceIP, err)
			d.deleteDevice(deviceDigest, job.subnet)
		} else if len(value.Variables) < 1 || value.Variables[0].Value == nil {
			log.Debugf("subnet %s: SNMP get to %s no data", d.config.Network, deviceIP)
			d.deleteDevice(deviceDigest, job.subnet)
		} else {
			log.Debugf("subnet %s: SNMP get to %s success: %v", d.config.Network, deviceIP, value.Variables[0].Value)
			d.createDevice(deviceDigest, job.subnet, deviceIP, true)
		}
	}
	return nil
}

func (d *Discovery) createDevice(deviceDigest checkconfig.DeviceDigest, subnet *snmpSubnet, deviceIP string, writeCache bool) {
	deviceCk, err := devicecheck.NewDeviceCheck(subnet.config, deviceIP, d.sessionFactory)
	if err != nil {
		// should not happen since the deviceCheck is expected to be valid at this point
		// and are only changing the device ip
		log.Warnf("subnet %s: failed to create new device check `%s`: %s", d.config.Network, deviceIP, err)
		return
	}

	d.discDevMu.Lock()
	defer d.discDevMu.Unlock()

	if _, present := d.discoveredDevices[deviceDigest]; present {
		return
	}
	device := Device{
		deviceDigest: deviceDigest,
		deviceIP:     deviceIP,
		deviceCheck:  deviceCk,
	}
	d.discoveredDevices[deviceDigest] = device
	subnet.devices[deviceDigest] = deviceIP
	subnet.deviceFailures[deviceDigest] = 0

	if writeCache {
		d.writeCache(subnet)
	}
}

// deleteDevice removes a device from discovered devices list and cache
// if the allowed device failures count is reached
func (d *Discovery) deleteDevice(deviceDigest checkconfig.DeviceDigest, subnet *snmpSubnet) {
	d.discDevMu.Lock()
	defer d.discDevMu.Unlock()
	if _, present := d.discoveredDevices[deviceDigest]; present {
		failure, present := subnet.deviceFailures[deviceDigest]
		if !present {
			subnet.deviceFailures[deviceDigest] = 1
			failure = 1
		} else {
			subnet.deviceFailures[deviceDigest]++
			failure++
		}

		if d.config.DiscoveryAllowedFailures != -1 && failure >= d.config.DiscoveryAllowedFailures {
			delete(d.discoveredDevices, deviceDigest)
			delete(subnet.devices, deviceDigest)
			delete(subnet.deviceFailures, deviceDigest)
			d.writeCache(subnet)
		}
	}
}

func (d *Discovery) readCache(subnet *snmpSubnet) ([]net.IP, error) {
	cacheValue, err := persistentcache.Read(subnet.cacheKey)
	if err != nil {
		return nil, fmt.Errorf("couldn't read cache for %s: %s", subnet.cacheKey, err)
	}
	if cacheValue == "" {
		return []net.IP{}, nil
	}
	var devices []net.IP
	if err = json.Unmarshal([]byte(cacheValue), &devices); err != nil {
		return nil, fmt.Errorf("couldn't unmarshal cache for %s: %s", subnet.cacheKey, err)
	}
	return devices, nil
}

func (d *Discovery) loadCache(subnet *snmpSubnet) {
	devices, err := d.readCache(subnet)
	if err != nil {
		log.Errorf("subnet %s: error reading cache: %s", d.config.Network, err)
		return
	}
	for _, deviceIP := range devices {
		deviceDigest := subnet.config.DeviceDigest(deviceIP.String())
		d.createDevice(deviceDigest, subnet, deviceIP.String(), false)
	}
}

func (d *Discovery) writeCache(subnet *snmpSubnet) {
	// We don't lock the subnet for now, because the discovery ought to be already locked
	devices := make([]string, 0, len(subnet.devices))
	for _, v := range subnet.devices {
		devices = append(devices, v)
	}

	cacheValue, err := json.Marshal(devices)
	if err != nil {
		log.Errorf("subnet %s: Couldn't marshal cache: %s", d.config.Network, err)
		return
	}

	if err = persistentcache.Write(subnet.cacheKey, string(cacheValue)); err != nil {
		log.Errorf("subnet %s: Couldn't write cache: %s", d.config.Network, err)
	}
}

// NewDiscovery return a new Discovery instance
func NewDiscovery(config *checkconfig.CheckConfig, sessionFactory session.Factory) *Discovery {
	return &Discovery{
		discoveredDevices: make(map[checkconfig.DeviceDigest]Device),
		stop:              make(chan struct{}),
		config:            config,
		sessionFactory:    sessionFactory,
	}
}
