package discovery

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/session"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"net"
	"testing"
	"time"
)

func TestDiscovery(t *testing.T) {
	sess := session.CreateMockSession()
	session.NewSession = func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}

	packet := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.2.0",
				Type:  gosnmp.ObjectIdentifier,
				Value: "1.3.6.1.4.1.3375.2.1.3.4.1",
			},
		},
	}
	sess.On("Get", []string{"1.3.6.1.2.1.1.2.0"}).Return(&packet, nil)

	checkConfig := &checkconfig.CheckConfig{
		Network:            "192.168.0.0/29",
		CommunityString:    "public",
		DiscoveryInterval:  3600,
		DiscoveryWorkers:   1,
		IgnoredIPAddresses: map[string]bool{"192.168.0.5": true},
	}
	discovery := NewDiscovery(checkConfig)
	discovery.Start()
	time.Sleep(100 * time.Millisecond)
	discovery.Stop()

	deviceConfigs := discovery.GetDiscoveredDeviceConfigs()

	var actualDiscoveredIps []string
	for _, deviceCk := range deviceConfigs {
		actualDiscoveredIps = append(actualDiscoveredIps, deviceCk.GetIPAddress())
	}
	expectedDiscoveredIps := []string{
		"192.168.0.0",
		"192.168.0.1",
		"192.168.0.2",
		"192.168.0.3",
		"192.168.0.4",
		// 192.168.0.5 is ignored
		"192.168.0.6",
		"192.168.0.7",
	}
	assert.ElementsMatch(t, expectedDiscoveredIps, actualDiscoveredIps)
}

func TestDiscoveryCache(t *testing.T) {
	SetTestRunPath()
	sess := session.CreateMockSession()
	session.NewSession = func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}

	packet := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.2.0",
				Type:  gosnmp.ObjectIdentifier,
				Value: "1.3.6.1.4.1.3375.2.1.3.4.1",
			},
		},
	}
	sess.On("Get", []string{"1.3.6.1.2.1.1.2.0"}).Return(&packet, nil)

	checkConfig := &checkconfig.CheckConfig{
		Network:           "192.168.0.0/30",
		CommunityString:   "public",
		DiscoveryInterval: 3600,
		DiscoveryWorkers:  1,
	}
	discovery := NewDiscovery(checkConfig)
	discovery.Start()
	time.Sleep(100 * time.Millisecond)
	discovery.Stop()

	deviceConfigs := discovery.GetDiscoveredDeviceConfigs()

	var actualDiscoveredIps []string
	for _, deviceCk := range deviceConfigs {
		actualDiscoveredIps = append(actualDiscoveredIps, deviceCk.GetIPAddress())
	}
	expectedDiscoveredIps := []string{
		"192.168.0.0",
		"192.168.0.1",
		"192.168.0.2",
		"192.168.0.3",
	}
	assert.ElementsMatch(t, expectedDiscoveredIps, actualDiscoveredIps)

	// test cache
	// session is never used, the devices are loaded from cache
	sess2 := session.CreateMockSession()
	session.NewSession = func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess2, nil
	}

	checkConfig = &checkconfig.CheckConfig{
		Network:           "192.168.0.0/30",
		CommunityString:   "public",
		DiscoveryInterval: 3600,
		DiscoveryWorkers:  0, // no workers, the devices will be loaded from cache
	}
	discovery2 := NewDiscovery(checkConfig)
	discovery2.Start()
	time.Sleep(100 * time.Millisecond)
	discovery2.Stop()

	deviceConfigsFromCache := discovery2.GetDiscoveredDeviceConfigs()

	var actualDiscoveredIpsFromCache []string
	for _, deviceCk := range deviceConfigsFromCache {
		actualDiscoveredIpsFromCache = append(actualDiscoveredIpsFromCache, deviceCk.GetIPAddress())
	}
	assert.ElementsMatch(t, expectedDiscoveredIps, actualDiscoveredIpsFromCache)
}

func TestDiscoveryTicker(t *testing.T) {
	t.Skip() // TODO: FIX ME, currently this test is leading to data race when ran with other tests

	sess := session.CreateMockSession()
	session.NewSession = func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}

	packet := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.2.0",
				Type:  gosnmp.ObjectIdentifier,
				Value: "1.3.6.1.4.1.3375.2.1.3.4.1",
			},
		},
	}
	sess.On("Get", []string{"1.3.6.1.2.1.1.2.0"}).Return(&packet, nil)

	checkConfig := &checkconfig.CheckConfig{
		Network:           "192.168.0.0/32",
		CommunityString:   "public",
		DiscoveryInterval: 1,
		DiscoveryWorkers:  1,
	}
	discovery := NewDiscovery(checkConfig)
	discovery.Start()
	time.Sleep(1500 * time.Millisecond)
	discovery.Stop()

	// expected to be called 3 times for 1.5 sec
	// first time on discovery.Start, then once every second for the first 1 sec
	assert.Equal(t, 2, len(sess.Calls))
}

func TestDiscovery_checkDevice(t *testing.T) {
	SetTestRunPath()
	checkConfig := &checkconfig.CheckConfig{
		Network:           "192.168.0.0/32",
		CommunityString:   "public",
		DiscoveryInterval: 1,
		DiscoveryWorkers:  1,
	}
	discovery := NewDiscovery(checkConfig)
	ipAddr, ipNet, err := net.ParseCIDR(checkConfig.Network)
	assert.Nil(t, err)
	startingIP := ipAddr.Mask(ipNet.Mask)

	subnet := snmpSubnet{
		config:         checkConfig,
		startingIP:     startingIP,
		network:        *ipNet,
		cacheKey:       "abc:123",
		devices:        map[checkconfig.DeviceDigest]string{},
		deviceFailures: map[checkconfig.DeviceDigest]int{},
	}

	job := checkDeviceJob{
		subnet:    &subnet,
		currentIP: startingIP,
	}

	packet := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.2.0",
				Type:  gosnmp.ObjectIdentifier,
				Value: "1.3.6.1.4.1.3375.2.1.3.4.1",
			},
		},
	}

	var sess *session.MockSession

	checkDeviceOnce := func() {
		sess = session.CreateMockSession()
		session.NewSession = func(*checkconfig.CheckConfig) (session.Session, error) {
			return sess, nil
		}
		sess.On("Get", []string{"1.3.6.1.2.1.1.2.0"}).Return(&packet, nil)
		err = discovery.checkDevice(job) // add device
		assert.Nil(t, err)
		assert.Equal(t, 1, len(discovery.discoveredDevices))
	}

	// session configuration error
	session.NewSession = func(*checkconfig.CheckConfig) (session.Session, error) {
		return nil, fmt.Errorf("some error")
	}
	err = discovery.checkDevice(job)
	assert.EqualError(t, err, "error configure session for ip 192.168.0.0: some error")
	assert.Equal(t, 0, len(discovery.discoveredDevices))
	assert.Equal(t, "", discovery.config.IPAddress)

	// Test session.Connect() error
	checkDeviceOnce()
	sess.ConnectErr = fmt.Errorf("connection error")
	err = discovery.checkDevice(job)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(discovery.discoveredDevices))

	// Test session.Get() error
	checkDeviceOnce()
	sess = session.CreateMockSession()
	session.NewSession = func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}
	var nilPacket *gosnmp.SnmpPacket
	sess.On("Get", []string{"1.3.6.1.2.1.1.2.0"}).Return(nilPacket, fmt.Errorf("get error"))
	err = discovery.checkDevice(job) // check device with Get error
	assert.Nil(t, err)
	assert.Equal(t, 0, len(discovery.discoveredDevices))

	// Test session.Get() packet with no variable
	checkDeviceOnce()
	sess = session.CreateMockSession()
	session.NewSession = func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}
	packetNoVariable := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{},
	}
	sess.On("Get", []string{"1.3.6.1.2.1.1.2.0"}).Return(&packetNoVariable, nil)
	err = discovery.checkDevice(job) // check device with Get error
	assert.Nil(t, err)
	assert.Equal(t, 0, len(discovery.discoveredDevices))

	// Test session.Get() packet with nil value
	checkDeviceOnce()
	sess = session.CreateMockSession()
	session.NewSession = func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}
	packetNilValue := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.2.0",
				Type:  gosnmp.ObjectIdentifier,
				Value: nil,
			},
		},
	}
	sess.On("Get", []string{"1.3.6.1.2.1.1.2.0"}).Return(&packetNilValue, nil)
	err = discovery.checkDevice(job) // check device with Get error
	assert.Nil(t, err)
	assert.Equal(t, 0, len(discovery.discoveredDevices))
}

func TestDiscovery_createDevice(t *testing.T) {
	SetTestRunPath()
	checkConfig := &checkconfig.CheckConfig{
		Network:                  "192.168.0.0/32",
		CommunityString:          "public",
		DiscoveryInterval:        1,
		DiscoveryWorkers:         1,
		DiscoveryAllowedFailures: 3,
	}
	discovery := NewDiscovery(checkConfig)
	ipAddr, ipNet, err := net.ParseCIDR(checkConfig.Network)
	assert.Nil(t, err)
	startingIP := ipAddr.Mask(ipNet.Mask)

	subnet := &snmpSubnet{
		config:         checkConfig,
		startingIP:     startingIP,
		network:        *ipNet,
		cacheKey:       "abc:123",
		devices:        map[checkconfig.DeviceDigest]string{},
		deviceFailures: map[checkconfig.DeviceDigest]int{},
	}

	device1Digest := subnet.config.DeviceDigest("192.168.0.1")
	device2Digest := subnet.config.DeviceDigest("192.168.0.2")
	device3Digest := subnet.config.DeviceDigest("192.168.0.3")
	discovery.createDevice(device1Digest, subnet, "192.168.0.1", true)
	discovery.createDevice(device2Digest, subnet, "192.168.0.2", true)
	discovery.createDevice(device3Digest, subnet, "192.168.0.3", false)

	assert.Equal(t, 3, len(discovery.discoveredDevices))

	assert.Equal(t, device1Digest, discovery.discoveredDevices[device1Digest].deviceDigest)
	assert.Equal(t, "192.168.0.1", discovery.discoveredDevices[device1Digest].deviceIP)
	assert.Equal(t, "192.168.0.1", discovery.discoveredDevices[device1Digest].deviceCheck.GetIPAddress())
	assert.Equal(t, []string{"snmp_device:192.168.0.1"}, discovery.discoveredDevices[device1Digest].deviceCheck.GetIDTags())

	assert.Equal(t, device2Digest, discovery.discoveredDevices[device2Digest].deviceDigest)
	assert.Equal(t, "192.168.0.2", discovery.discoveredDevices[device2Digest].deviceIP)

	// test that only createDevice with writeCache:true are written to cache
	ips, err := discovery.readCache(subnet)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(ips))

	// test deleteDevice
	assert.Equal(t, 0, subnet.deviceFailures[device1Digest])
	assert.Equal(t, 3, len(discovery.discoveredDevices))
	discovery.deleteDevice(device1Digest, subnet) // increment failure count
	assert.Equal(t, 1, subnet.deviceFailures[device1Digest])
	assert.Equal(t, 3, len(discovery.discoveredDevices))
	discovery.deleteDevice(device1Digest, subnet) // increment failure count
	assert.Equal(t, 2, subnet.deviceFailures[device1Digest])
	assert.Equal(t, 3, len(discovery.discoveredDevices))
	_, present := subnet.deviceFailures[device1Digest]
	assert.Equal(t, true, present)
	discovery.deleteDevice(device1Digest, subnet) // really deletes the device
	assert.Equal(t, 2, len(discovery.discoveredDevices))
}
