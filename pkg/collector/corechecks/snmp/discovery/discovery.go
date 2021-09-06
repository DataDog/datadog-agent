package discovery

import (
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/devicecheck"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/session"
)

// TODO: move to separate package ?

// Discovery handles snmp discovery states
type Discovery struct {
	sync.RWMutex
	config            *checkconfig.CheckConfig
	stop              chan bool
	discoveredDevices map[string]Device
}

// Device implements and store results from the Service interface for the SNMP listener
type Device struct {
	entityID     string
	deviceIP     string
	creationTime integration.CreationTime
	config       *checkconfig.CheckConfig
}
type snmpSubnet struct {
	config         *checkconfig.CheckConfig
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

// TODO: move to pkg/snmp
func incrementIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] > 0 {
			break
		}
	}
}

// Don't make it a method, to be overridden in tests
var worker = func(d *Discovery, jobs <-chan snmpJob) {
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

// Start discovery
func (d *Discovery) Start() {
	go d.checkDevices()
}

func (d *Discovery) checkDevice(job snmpJob) {
	deviceIP := job.currentIP.String()
	log.Warnf("[DEV] check Device %s", deviceIP)
	config := *job.subnet.config // shallow copy
	config.IPAddress = deviceIP
	sess := session.GosnmpSession{}
	err := sess.Configure(config)
	if err != nil {
		log.Errorf("Error configure session %s: %v", deviceIP, err)
		return
	}
	entityID := job.subnet.config.Digest(deviceIP)
	if err := sess.Connect(); err != nil {
		log.Debugf("SNMP connect to %s error: %v", deviceIP, err)
		d.deleteService(entityID, job.subnet)
	} else {
		defer sess.Close()

		oids := []string{"1.3.6.1.2.1.1.2.0"}
		// Since `params<GoSNMP>.ContextEngineID` is empty
		// `params.Get` might lead to multiple SNMP GET calls when using SNMP v3
		value, err := sess.Get(oids)
		if err != nil {
			log.Debugf("SNMP get to %s error: %v", deviceIP, err)
			d.deleteService(entityID, job.subnet)
		} else if len(value.Variables) < 1 || value.Variables[0].Value == nil {
			log.Debugf("SNMP get to %s no data", deviceIP)
			d.deleteService(entityID, job.subnet)
		} else {
			log.Debugf("SNMP get to %s success: %v", deviceIP, value.Variables[0].Value)
			log.Warnf("SNMP get to %s success: %v", deviceIP, value.Variables[0].Value)
			d.createService(entityID, job.subnet, deviceIP, true)
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

	configHash := d.config.Digest(d.config.Network)
	cacheKey := fmt.Sprintf("snmp:%s", configHash)

	subnet := snmpSubnet{
		config:         d.config,
		startingIP:     startingIP,
		network:        *ipNet,
		cacheKey:       cacheKey,
		devices:        map[string]string{},
		deviceFailures: map[string]int{},
	}

	//l.loadCache(&subnet)

	jobs := make(chan snmpJob)
	for w := 0; w < d.config.DiscoveryWorkers; w++ {
		go worker(d, jobs)
	}

	log.Warnf("[DEV] jobs %v", jobs)
	discoveryTicker := time.NewTicker(time.Duration(d.config.DiscoveryInterval) * time.Second)

	for {
		log.Warnf("[DEV] start discovery")

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
				return
			default:
			}
		}

		select {
		case <-d.stop:
			return
		case <-discoveryTicker.C:
		}
	}
}

func (d *Discovery) createService(entityID string, subnet *snmpSubnet, deviceIP string, writeCache bool) {
	d.Lock()
	defer d.Unlock()
	if _, present := d.discoveredDevices[entityID]; present {
		return
	}
	svc := Device{
		entityID:     entityID,
		deviceIP:     deviceIP,
		creationTime: integration.Before,
		config:       subnet.config.Copy(),
	}
	d.discoveredDevices[entityID] = svc
	subnet.devices[entityID] = deviceIP
	subnet.deviceFailures[entityID] = 0
	log.Warnf("[DEV] Create service : %s, discoveredDevices: %v", deviceIP, len(d.discoveredDevices))

	//if writeCache {
	//	d.writeCache(subnet)
	//}
	//d.newService <- svc
}

func (d *Discovery) deleteService(entityID string, subnet *snmpSubnet) {
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
			//d.delService <- svc
			delete(d.discoveredDevices, entityID)
			delete(subnet.devices, entityID)
			//d.writeCache(subnet)
		}
	}
}

// GetDiscoveredDeviceConfigs returns discovered device configs
func (d *Discovery) GetDiscoveredDeviceConfigs(sender aggregator.Sender) []*devicecheck.DeviceCheck {
	d.Lock()
	defer d.Unlock()
	var discoveredDevices []*devicecheck.DeviceCheck
	// TODO: store config instead of discoveredDevices ?
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

// GetDiscoveredDeviceConfigsTestInstances returns discovered test instances
func (d *Discovery) GetDiscoveredDeviceConfigsTestInstances(testInstances int, sender aggregator.Sender) []*devicecheck.DeviceCheck {
	d.Lock()
	defer d.Unlock()
	var discoveredDevices []*devicecheck.DeviceCheck
	for _, device := range d.discoveredDevices {
		for i := 0; i < testInstances; i++ {
			config := device.config.Copy()
			config.ExtraTags = append(common.CopyStrings(config.ExtraTags), "test_instance:"+strconv.Itoa(i)) // TODO: for testing only
			deviceCk, err := devicecheck.NewDeviceCheck(config, device.deviceIP)
			if err != nil {
				log.Warnf("failed to create new device check `%s`: %s", device.deviceIP, err)
			}
			discoveredDevices = append(discoveredDevices, deviceCk)

		}
	}
	return discoveredDevices
}

// Stop signal discovery to shut down
func (d *Discovery) Stop() {
	d.stop <- true
}

// NewDiscovery return a new Discovery instance
func NewDiscovery(config *checkconfig.CheckConfig) Discovery {
	return Discovery{
		discoveredDevices: map[string]Device{},
		stop:              make(chan bool),
		config:            config,
	}
}
