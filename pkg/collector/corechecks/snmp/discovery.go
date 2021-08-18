package snmp

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"net"
	"strconv"
	"sync"
	"time"
)

// TODO: move to separate package ?

type snmpDiscovery struct {
	sync.RWMutex
	config   snmpConfig
	stop     chan bool
	services map[string]Device
}

// Device implements and store results from the Service interface for the SNMP listener
type Device struct {
	//adIdentifier string
	entityID     string
	deviceIP     string
	creationTime integration.CreationTime
	config       snmpConfig
}
type snmpSubnet struct {
	adIdentifier   string
	config         snmpConfig
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
var worker = func(d *snmpDiscovery, jobs <-chan snmpJob) {
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

func (d *snmpDiscovery) Start() {
	go d.checkDevices()
}

func (d *snmpDiscovery) checkDevice(job snmpJob) {
	deviceIP := job.currentIP.String()
	log.Warnf("[DEV] check Device %s", deviceIP)
	config := job.subnet.config // TODO: avoid full copy ?
	config.ipAddress = deviceIP
	sess := snmpSession{}
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

func (d *snmpDiscovery) checkDevices() {
	subnets := []snmpSubnet{}
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
	subnets = append(subnets, subnet)

	//l.loadCache(&subnet)

	jobs := make(chan snmpJob)
	for w := 0; w < d.config.Workers; w++ {
		go worker(d, jobs)
	}

	log.Warnf("[DEV] jobs %v", jobs)
	discoveryTicker := time.NewTicker(time.Duration(d.config.DiscoveryInterval) * time.Second)

	log.Warnf("[DEV] subnets len: %d", len(subnets))

	for {
		log.Warnf("[DEV] start discovery")
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
				case <-d.stop:
					return
				default:
				}
			}
		}

		select {
		case <-d.stop:
			return
		case <-discoveryTicker.C:
		}
	}
}

func (d *snmpDiscovery) createService(entityID string, subnet *snmpSubnet, deviceIP string, writeCache bool) {
	d.Lock()
	defer d.Unlock()
	if _, present := d.services[entityID]; present {
		return
	}
	svc := Device{
		//adIdentifier: subnet.adIdentifier,
		entityID:     entityID,
		deviceIP:     deviceIP,
		creationTime: integration.Before,
		config:       subnet.config,
	}
	d.services[entityID] = svc
	subnet.devices[entityID] = deviceIP
	subnet.deviceFailures[entityID] = 0
	log.Warnf("[DEV] Create service : %s, services: %v", deviceIP, len(d.services))

	//if writeCache {
	//	d.writeCache(subnet)
	//}
	//d.newService <- svc
}

func (d *snmpDiscovery) deleteService(entityID string, subnet *snmpSubnet) {
	d.Lock()
	defer d.Unlock()
	if _, present := d.services[entityID]; present {
		failure, present := subnet.deviceFailures[entityID]
		if !present {
			subnet.deviceFailures[entityID] = 1
			failure = 1
		} else {
			subnet.deviceFailures[entityID]++
			failure++
		}

		if d.config.AllowedFailures != -1 && failure >= d.config.AllowedFailures {
			//d.delService <- svc
			delete(d.services, entityID)
			delete(subnet.devices, entityID)
			//d.writeCache(subnet)
		}
	}
}

func (d *snmpDiscovery) getDiscoveredDeviceConfigs() []snmpConfig {
	d.Lock()
	defer d.Unlock()
	var discoveredDevices []snmpConfig
	for _, device := range d.services {
		config := device.config
		config.Network = ""
		config.ipAddress = device.deviceIP

		// TODO: Refactor to avoid duplication of logic with https://github.com/DataDog/datadog-agent/blob/0e88b93d1902eddc1542aa15c41b91fcbeecc588/pkg/collector/corechecks/snmp/config.go#L388
		config.deviceID, config.deviceIDTags = buildDeviceID(config.getDeviceIDTags())

		err := config.session.Configure(config)
		if err != nil {
			log.Warnf("failed to configure device `%s`: %s", device.deviceIP, err)
			continue
		}
		discoveredDevices = append(discoveredDevices, config)
	}
	return discoveredDevices
}


func (d *snmpDiscovery) getDiscoveredDeviceConfigsTestInstances(testInstances int) []snmpConfig {
	d.Lock()
	defer d.Unlock()
	var discoveredDevices []snmpConfig
	for _, device := range d.services {
		for i := 0; i < testInstances; i++ {
			config := device.config // TODO: this is only a shallow copy
			config.Network = ""
			config.ipAddress = device.deviceIP
			config.extraTags = append(copyStrings(config.extraTags), "test_instance:" + strconv.Itoa(i)) // TODO: for testing only

			// TODO: Refactor to avoid duplication of logic with https://github.com/DataDog/datadog-agent/blob/0e88b93d1902eddc1542aa15c41b91fcbeecc588/pkg/collector/corechecks/snmp/config.go#L388
			config.deviceID, config.deviceIDTags = buildDeviceID(config.getDeviceIDTags())

			err := config.session.Configure(config)
			if err != nil {
				log.Warnf("failed to configure device `%s`: %s", device.deviceIP, err)
				continue
			}
			discoveredDevices = append(discoveredDevices, config)
		}
	}
	return discoveredDevices
}

func newSnmpDiscovery(config snmpConfig) snmpDiscovery {
	return snmpDiscovery{
		services: map[string]Device{},
		stop:     make(chan bool),
		config:   config,
	}
}
