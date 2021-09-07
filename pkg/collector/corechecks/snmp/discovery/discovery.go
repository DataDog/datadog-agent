package discovery

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/devicecheck"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/session"
)

const cacheKeyPrefix = "snmp_corecheck"

// Discovery handles snmp discovery states
type Discovery struct {
	sync.RWMutex
	config            *checkconfig.CheckConfig
	stop              chan bool
	discoveredDevices map[string]Device
}

// Device implements and store results from the Service interface for the SNMP listener
type Device struct {
	entityID string
	deviceIP string
	config   *checkconfig.CheckConfig
}
type snmpSubnet struct {
	config     *checkconfig.CheckConfig
	startingIP net.IP
	network    net.IPNet

	// TODO: Test caching
	cacheKey string
	devices  map[string]string

	// TODO: test device failures
	deviceFailures map[string]int
}

type snmpJob struct {
	subnet    *snmpSubnet
	currentIP net.IP
}

// Start discovery
func (d *Discovery) Start() {
	log.Debugf("Start discovery for subnet %s", d.config.Network)
	go d.checkDevices()
}

// Stop signal discovery to shut down
func (d *Discovery) Stop() {
	// TODO: test Stop
	log.Debugf("Stop discovery for subnet %s", d.config.Network)
	d.stop <- true
}

// GetDiscoveredDeviceConfigs returns discovered device configs
func (d *Discovery) GetDiscoveredDeviceConfigs() []*devicecheck.DeviceCheck {
	d.Lock()
	defer d.Unlock()
	var discoveredDevices []*devicecheck.DeviceCheck
	for _, device := range d.discoveredDevices {
		config := device.config
		deviceCk, err := devicecheck.NewDeviceCheck(config, device.deviceIP)
		if err != nil {
			log.Warnf("failed to create new device check `%s`: %s", device.deviceIP, err)
		}
		discoveredDevices = append(discoveredDevices, deviceCk)
	}
	return discoveredDevices
}

// Start discovery
func (d *Discovery) runWorker(jobs <-chan snmpJob) {
	for {
		select {
		case <-d.stop:
			log.Debug("Stopping SNMP worker")
			return
		case job := <-jobs:
			log.Debugf("Handling IP %s", job.currentIP.String())
			d.checkDevice(job)
		}
	}
}

func (d *Discovery) checkDevices() {
	ipAddr, ipNet, err := net.ParseCIDR(d.config.Network)
	if err != nil {
		log.Errorf("Couldn't parse SNMP network: %s", err)
		return
	}

	startingIP := ipAddr.Mask(ipNet.Mask)

	configHash := d.config.DiscoveryDigest(d.config.Network)
	cacheKey := fmt.Sprintf("%s:%s", cacheKeyPrefix, configHash)

	subnet := snmpSubnet{
		config:         d.config,
		startingIP:     startingIP,
		network:        *ipNet,
		cacheKey:       cacheKey,
		devices:        map[string]string{},
		deviceFailures: map[string]int{},
	}

	d.loadCache(&subnet)

	jobs := make(chan snmpJob)
	for w := 0; w < d.config.DiscoveryWorkers; w++ {
		go d.runWorker(jobs)
	}

	discoveryTicker := time.NewTicker(time.Duration(d.config.DiscoveryInterval) * time.Second)

	for {
		startingIP := make(net.IP, len(subnet.startingIP))
		copy(startingIP, subnet.startingIP)
		for currentIP := startingIP; subnet.network.Contains(currentIP); incrementIP(currentIP) {

			if ignored := subnet.config.IsIPIgnored(currentIP); ignored {
				continue
			}

			jobIP := make(net.IP, len(currentIP))
			copy(jobIP, currentIP)
			job := snmpJob{
				subnet:    &subnet,
				currentIP: jobIP,
			}
			jobs <- job

			select {
			case <-d.stop:
				// TODO: TEST ME
				return
			default:
			}
		}

		select {
		case <-d.stop:
			// TODO: TEST ME
			return
		case <-discoveryTicker.C:
			// TODO: TEST ME
		}
	}
}

func (d *Discovery) checkDevice(job snmpJob) {
	deviceIP := job.currentIP.String()
	config := *job.subnet.config // shallow copy
	config.IPAddress = deviceIP
	sess, err := session.NewSession(&config)
	if err != nil {
		log.Errorf("Error configure session %s: %v", deviceIP, err)
		return
	}
	entityID := job.subnet.config.DiscoveryDigest(deviceIP)
	if err := sess.Connect(); err != nil {
		log.Debugf("SNMP connect to %s error: %v", deviceIP, err)
		d.deleteDevice(entityID, job.subnet)
	} else {
		defer sess.Close()

		oids := []string{"1.3.6.1.2.1.1.2.0"}
		// Since `params<GoSNMP>.ContextEngineID` is empty
		// `params.Get` might lead to multiple SNMP GET calls when using SNMP v3
		value, err := sess.Get(oids)
		if err != nil {
			log.Debugf("SNMP get to %s error: %v", deviceIP, err)
			d.deleteDevice(entityID, job.subnet)
		} else if len(value.Variables) < 1 || value.Variables[0].Value == nil {
			log.Debugf("SNMP get to %s no data", deviceIP)
			d.deleteDevice(entityID, job.subnet)
		} else {
			log.Debugf("SNMP get to %s success: %v", deviceIP, value.Variables[0].Value)
			d.createDevice(entityID, job.subnet, deviceIP, true)
		}
	}
}

func (d *Discovery) createDevice(entityID string, subnet *snmpSubnet, deviceIP string, writeCache bool) {
	d.Lock()
	defer d.Unlock()
	if _, present := d.discoveredDevices[entityID]; present {
		return
	}
	svc := Device{
		entityID: entityID,
		deviceIP: deviceIP,
		config:   subnet.config.Copy(),
	}
	d.discoveredDevices[entityID] = svc
	subnet.devices[entityID] = deviceIP
	subnet.deviceFailures[entityID] = 0

	// TODO: TEST writeCache
	if writeCache {
		d.writeCache(subnet)
	}
}

func (d *Discovery) deleteDevice(entityID string, subnet *snmpSubnet) {
	// TODO: TEST deleteDevice
	d.Lock()
	defer d.Unlock()
	if _, present := d.discoveredDevices[entityID]; present {
		failure, present := subnet.deviceFailures[entityID]
		if !present {
			subnet.deviceFailures[entityID] = 1
			failure = 1
		} else {
			subnet.deviceFailures[entityID]++
			failure++
		}

		if d.config.DiscoveryAllowedFailures != -1 && failure >= d.config.DiscoveryAllowedFailures {
			delete(d.discoveredDevices, entityID)
			delete(subnet.devices, entityID)
			d.writeCache(subnet)
		}
	}
}

func (d *Discovery) loadCache(subnet *snmpSubnet) {
	// TODO: test loadCache
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
		entityID := subnet.config.DiscoveryDigest(deviceIP.String())
		d.createDevice(entityID, subnet, deviceIP.String(), false)
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
		log.Errorf("Couldn't marshal cache: %s", err)
		return
	}

	if err = persistentcache.Write(subnet.cacheKey, string(cacheValue)); err != nil {
		log.Errorf("Couldn't write cache: %s", err)
	}
}

// NewDiscovery return a new Discovery instance
func NewDiscovery(config *checkconfig.CheckConfig) Discovery {
	return Discovery{
		discoveredDevices: map[string]Device{},
		stop:              make(chan bool),
		config:            config,
	}
}
