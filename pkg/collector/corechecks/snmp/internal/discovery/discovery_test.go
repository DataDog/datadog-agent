// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discovery

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	agentconfig "github.com/DataDog/datadog-agent/comp/core/config"
	snmpscanmanager "github.com/DataDog/datadog-agent/comp/snmpscanmanager/def"
	snmpscanmanagermock "github.com/DataDog/datadog-agent/comp/snmpscanmanager/mock"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/profile"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
)

func waitForDiscoveredDevices(discovery *Discovery, expectedDeviceCount int) error {
	synctest.Wait()
	deviceCount := len(discovery.GetDiscoveredDeviceConfigs())
	if deviceCount == expectedDeviceCount {
		return nil
	}
	return fmt.Errorf("expected exactly %d devices but %d has been discovered", expectedDeviceCount, deviceCount)
}

func cacheKey(checkConfig *checkconfig.CheckConfig) string {
	return fmt.Sprintf("%s:%s", cacheKeyPrefix, checkConfig.DeviceDigest(checkConfig.Network))
}

func readCachedIPs(t *testing.T, discovery *Discovery, cacheKey string) []string {
	t.Helper()
	ips, err := discovery.readCache(&snmpSubnet{cacheKey: cacheKey})
	assert.NoError(t, err)
	var ipStrings []string
	for _, ip := range ips {
		ipStrings = append(ipStrings, ip.String())
	}
	return ipStrings
}

func TestDiscovery(t *testing.T) {
	synctest.Test(t, syncTestDiscovery)
}

func syncTestDiscovery(t *testing.T) {
	config := agentconfig.NewMock(t)
	config.SetInTest("run_path", t.TempDir())

	sess := session.CreateMockSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
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
		ProfileProvider:    profile.StaticProvider(nil),
	}
	discovery := NewDiscovery(checkConfig, sessionFactory, config, nil)
	discovery.Start()
	assert.NoError(t, waitForDiscoveredDevices(discovery, 7))
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
	synctest.Test(t, syncTestDiscoveryCache)
}

func syncTestDiscoveryCache(t *testing.T) {
	config := agentconfig.NewMock(t)
	config.SetInTest("run_path", t.TempDir())

	sess := session.CreateMockSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
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
		ProfileProvider:   profile.StaticProvider(nil),
	}
	discovery := NewDiscovery(checkConfig, sessionFactory, config, nil)
	discovery.Start()
	assert.NoError(t, waitForDiscoveredDevices(discovery, 4))
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
	discovery.sessionFactory = func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess2, nil
	}

	checkConfig = &checkconfig.CheckConfig{
		Network:           "192.168.0.0/30",
		CommunityString:   "public",
		DiscoveryInterval: 3600,
		DiscoveryWorkers:  0, // no workers, the devices will be loaded from cache
		ProfileProvider:   profile.StaticProvider(nil),
	}
	discovery2 := NewDiscovery(checkConfig, sessionFactory, config, nil)
	discovery2.Start()
	assert.NoError(t, waitForDiscoveredDevices(discovery2, 4))
	discovery2.Stop()

	deviceConfigsFromCache := discovery2.GetDiscoveredDeviceConfigs()

	var actualDiscoveredIpsFromCache []string
	for _, deviceCk := range deviceConfigsFromCache {
		actualDiscoveredIpsFromCache = append(actualDiscoveredIpsFromCache, deviceCk.GetIPAddress())
	}
	assert.ElementsMatch(t, expectedDiscoveredIps, actualDiscoveredIpsFromCache)

	// test readCache directly against what discovery's pass persisted to disk
	assert.ElementsMatch(t, expectedDiscoveredIps, readCachedIPs(t, discovery2, cacheKey(checkConfig)))
}

func TestDiscoveryTicker(t *testing.T) {
	synctest.Test(t, syncTestDiscoveryTicker)
}

func syncTestDiscoveryTicker(t *testing.T) {
	config := agentconfig.NewMock(t)
	config.SetInTest("run_path", t.TempDir())

	sess := session.CreateMockSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
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
		ProfileProvider:   profile.StaticProvider(nil),
	}
	discovery := NewDiscovery(checkConfig, sessionFactory, config, nil)
	discovery.Start()
	time.Sleep(1500 * time.Millisecond)
	discovery.Stop()

	// expected to be called 3 times for 1.5 sec
	// first time on discovery.Start, then once every second for the first 1 sec
	assert.Equal(t, 2, len(sess.Calls))
}

func TestDiscovery_checkDevice(t *testing.T) {
	config := agentconfig.NewMock(t)
	config.SetInTest("run_path", t.TempDir())
	checkConfig := &checkconfig.CheckConfig{
		Network:           "192.168.0.0/32",
		CommunityString:   "public",
		DiscoveryInterval: 1,
		DiscoveryWorkers:  1,
		ProfileProvider:   profile.StaticProvider(nil),
	}
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
	discovery := NewDiscovery(checkConfig, session.NewMockSession, config, nil)

	checkDeviceOnce := func() {
		sess = session.CreateMockSession()
		sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
			return sess, nil
		}
		discovery.sessionFactory = sessionFactory
		sess.On("Get", []string{"1.3.6.1.2.1.1.2.0"}).Return(&packet, nil)
		err = discovery.checkDevice(job) // add device
		assert.Nil(t, err)
		assert.Equal(t, 1, len(discovery.discoveredDevices))
	}

	// session configuration error
	discovery.sessionFactory = func(*checkconfig.CheckConfig) (session.Session, error) {
		return nil, errors.New("some error")
	}

	err = discovery.checkDevice(job)
	assert.EqualError(t, err, "error configure session for ip 192.168.0.0: some error")
	assert.Equal(t, 0, len(discovery.discoveredDevices))
	assert.Equal(t, "", discovery.config.IPAddress)

	// Test session.Connect() error
	checkDeviceOnce()
	sess.ConnectErr = errors.New("connection error")
	err = discovery.checkDevice(job)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(discovery.discoveredDevices))

	// Test session.Get() error
	checkDeviceOnce()
	sess = session.CreateMockSession()
	discovery.sessionFactory = func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}
	var nilPacket *gosnmp.SnmpPacket
	sess.On("Get", []string{"1.3.6.1.2.1.1.2.0"}).Return(nilPacket, errors.New("get error"))
	err = discovery.checkDevice(job) // check device with Get error
	assert.Nil(t, err)
	assert.Equal(t, 0, len(discovery.discoveredDevices))

	// Test session.Get() packet with no variable
	checkDeviceOnce()
	sess = session.CreateMockSession()
	discovery.sessionFactory = func(*checkconfig.CheckConfig) (session.Session, error) {
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
	discovery.sessionFactory = func(*checkconfig.CheckConfig) (session.Session, error) {
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
	config := agentconfig.NewMock(t)
	config.SetInTest("run_path", t.TempDir())

	checkConfig := &checkconfig.CheckConfig{
		Network:                  "192.168.0.0/32",
		CommunityString:          "public",
		DiscoveryInterval:        1,
		DiscoveryWorkers:         1,
		DiscoveryAllowedFailures: 3,
		Namespace:                "default",
		ProfileProvider:          profile.StaticProvider(nil),
	}
	discovery := NewDiscovery(checkConfig, session.NewMockSession, config, nil)
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

	// loading device1 at startup should not mark the cache dirty
	assert.False(t, subnet.cacheDirty)
	discovery.createDevice(device1Digest, subnet, "192.168.0.1", false)
	assert.False(t, subnet.cacheDirty)
	discovery.writeCache(subnet)
	assert.Empty(t, readCachedIPs(t, discovery, subnet.cacheKey))

	// discovering device2 should mark the cache dirty, and writing encompass device1
	discovery.createDevice(device2Digest, subnet, "192.168.0.2", true)
	assert.True(t, subnet.cacheDirty)
	discovery.writeCache(subnet)
	assert.False(t, subnet.cacheDirty)
	assert.ElementsMatch(t, []string{"192.168.0.1", "192.168.0.2"}, readCachedIPs(t, discovery, subnet.cacheKey))

	// discovering device3 should also mark the cache dirty, and writing encompass all 3
	discovery.createDevice(device3Digest, subnet, "192.168.0.3", true)
	assert.True(t, subnet.cacheDirty)
	discovery.writeCache(subnet)
	assert.False(t, subnet.cacheDirty)
	assert.ElementsMatch(t, []string{"192.168.0.1", "192.168.0.2", "192.168.0.3"}, readCachedIPs(t, discovery, subnet.cacheKey))

	assert.Equal(t, 3, len(discovery.discoveredDevices))

	assert.Equal(t, device1Digest, discovery.discoveredDevices[device1Digest].deviceDigest)
	assert.Equal(t, "192.168.0.1", discovery.discoveredDevices[device1Digest].deviceIP)
	assert.Equal(t, "192.168.0.1", discovery.discoveredDevices[device1Digest].deviceCheck.GetIPAddress())
	assert.Equal(t, []string{"device_namespace:default", "snmp_device:192.168.0.1"}, discovery.discoveredDevices[device1Digest].deviceCheck.GetIDTags())

	assert.Equal(t, device2Digest, discovery.discoveredDevices[device2Digest].deviceDigest)
	assert.Equal(t, "192.168.0.2", discovery.discoveredDevices[device2Digest].deviceIP)

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
	discovery.writeCache(subnet)
	assert.False(t, subnet.cacheDirty)
	assert.ElementsMatch(t, []string{"192.168.0.2", "192.168.0.3"}, readCachedIPs(t, discovery, subnet.cacheKey))
}

func TestDeviceScansAreRequested(t *testing.T) {
	synctest.Test(t, syncTestDeviceScansAreRequested)
}

func syncTestDeviceScansAreRequested(t *testing.T) {
	config := agentconfig.NewMock(t)
	config.SetInTest("run_path", t.TempDir())

	sess := session.CreateMockSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
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
		ProfileProvider:   profile.StaticProvider(nil),
	}

	scanManager := snmpscanmanagermock.Mock(t)
	mockScanManager, ok := scanManager.(*snmpscanmanagermock.SnmpScanManagerMock)
	assert.True(t, ok)

	mockScanManager.On("RequestScan", snmpscanmanager.ScanRequest{
		DeviceIP: "192.168.0.0",
	}, false).Once()
	mockScanManager.On("RequestScan", snmpscanmanager.ScanRequest{
		DeviceIP: "192.168.0.1",
	}, false).Once()
	mockScanManager.On("RequestScan", snmpscanmanager.ScanRequest{
		DeviceIP: "192.168.0.2",
	}, false).Once()
	mockScanManager.On("RequestScan", snmpscanmanager.ScanRequest{
		DeviceIP: "192.168.0.3",
	}, false).Once()

	discovery := NewDiscovery(checkConfig, sessionFactory, config, scanManager)
	discovery.Start()
	assert.NoError(t, waitForDiscoveredDevices(discovery, 4))
	discovery.Stop()

	mockScanManager.AssertExpectations(t)
}

func TestDiscoveryWritesCacheOnceAfterTheWholePass(t *testing.T) {
	synctest.Test(t, syncTestDiscoveryWritesCacheOnceAfterTheWholePass)
}

func syncTestDiscoveryWritesCacheOnceAfterTheWholePass(t *testing.T) {
	config := agentconfig.NewMock(t)
	config.SetInTest("run_path", t.TempDir())

	sess := session.CreateMockSession()
	packet := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.2.0",
				Type:  gosnmp.ObjectIdentifier,
				Value: "1.3.6.1.4.1.3375.2.1.3.4.1",
			},
		},
	}
	device1Chan := make(chan time.Time)
	sess.On("Get", []string{"1.3.6.1.2.1.1.2.0"}).WaitUntil(device1Chan).Return(&packet, nil).Once()
	device2Chan := make(chan time.Time)
	sess.On("Get", []string{"1.3.6.1.2.1.1.2.0"}).WaitUntil(device2Chan).Return(&packet, nil).Once()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}

	checkConfig := &checkconfig.CheckConfig{
		Network:            "192.168.0.0/30",
		CommunityString:    "public",
		DiscoveryInterval:  3600,
		DiscoveryWorkers:   1,
		IgnoredIPAddresses: map[string]bool{"192.168.0.2": true, "192.168.0.3": true},
		ProfileProvider:    profile.StaticProvider(nil),
	}
	discovery := NewDiscovery(checkConfig, sessionFactory, config, nil)

	returnDevice1 := sync.OnceFunc(func() { close(device1Chan) })
	returnDevice2 := sync.OnceFunc(func() { close(device2Chan) })
	discovery.Start()
	t.Cleanup(func() {
		// never leave a worker goroutine blocked, even if an assertion fails
		returnDevice1()
		returnDevice2()
		discovery.Stop()
	})

	// the worker is now durably blocked inside device1's Get
	synctest.Wait()

	// let device1's check complete: the worker moves on to device2's Get
	returnDevice1()
	synctest.Wait()

	// nothing should be written yet: device1 succeeded, but the pass isn't done
	assert.Empty(t, readCachedIPs(t, discovery, cacheKey(checkConfig)))

	// let device2's check complete: this finishes the pass
	returnDevice2()
	synctest.Wait()

	// the completed pass writes the cache once, covering both devices
	assert.ElementsMatch(t, []string{"192.168.0.0", "192.168.0.1"}, readCachedIPs(t, discovery, cacheKey(checkConfig)))
}

func TestDiscoveryStopWaitsForInFlightCheck(t *testing.T) {
	synctest.Test(t, syncTestDiscoveryStopWaitsForInFlightCheck)
}

func syncTestDiscoveryStopWaitsForInFlightCheck(t *testing.T) {
	config := agentconfig.NewMock(t)
	config.SetInTest("run_path", t.TempDir())

	sess := session.CreateMockSession()
	packet := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.2.0",
				Type:  gosnmp.ObjectIdentifier,
				Value: "1.3.6.1.4.1.3375.2.1.3.4.1",
			},
		},
	}
	deviceChan := make(chan time.Time)
	sess.On("Get", []string{"1.3.6.1.2.1.1.2.0"}).WaitUntil(deviceChan).Return(&packet, nil)
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}

	checkConfig := &checkconfig.CheckConfig{
		Network:           "192.168.0.0/32",
		CommunityString:   "public",
		DiscoveryInterval: 3600,
		DiscoveryWorkers:  1,
		ProfileProvider:   profile.StaticProvider(nil),
	}
	discovery := NewDiscovery(checkConfig, sessionFactory, config, nil)
	discovery.Start()

	returnDevice := sync.OnceFunc(func() { close(deviceChan) })
	t.Cleanup(returnDevice) // never leave the worker goroutine blocked, even if an assertion fails

	// the worker is now durably blocked inside Get, mid-pass
	synctest.Wait()

	// Stop is now durably blocked on <-d.done, waiting on the check above
	stopped := make(chan struct{})
	go func() {
		discovery.Stop()
		close(stopped)
	}()
	synctest.Wait()
	select {
	case <-stopped:
		t.Fatal("Stop returned before the in-flight device check completed")
	default:
	}

	// releasing the blocked check lets the pass, and Stop, run to completion
	returnDevice()
	synctest.Wait()
	select {
	case <-stopped:
	default:
		t.Fatal("Stop did not return after the in-flight device check completed")
	}

	// by the time Stop returned, the pass's trailing cache write must already be on disk
	assert.ElementsMatch(t, []string{"192.168.0.0"}, readCachedIPs(t, discovery, cacheKey(checkConfig)))
}

func TestDiscoveryStopCancelsPendingSchedule(t *testing.T) {
	synctest.Test(t, syncTestDiscoveryStopCancelsPendingSchedule)
}

func syncTestDiscoveryStopCancelsPendingSchedule(t *testing.T) {
	config := agentconfig.NewMock(t)
	config.SetInTest("run_path", t.TempDir())

	sess := session.CreateMockSession()
	packet := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.2.0",
				Type:  gosnmp.ObjectIdentifier,
				Value: "1.3.6.1.4.1.3375.2.1.3.4.1",
			},
		},
	}
	deviceChan := make(chan time.Time)
	sess.On("Get", []string{"1.3.6.1.2.1.1.2.0"}).WaitUntil(deviceChan).Return(&packet, nil)
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}

	checkConfig := &checkconfig.CheckConfig{
		Network:            "192.168.0.0/30",
		CommunityString:    "public",
		DiscoveryInterval:  3600,
		DiscoveryWorkers:   1,
		IgnoredIPAddresses: map[string]bool{"192.168.0.2": true, "192.168.0.3": true},
		ProfileProvider:    profile.StaticProvider(nil),
	}
	discovery := NewDiscovery(checkConfig, sessionFactory, config, nil)
	discovery.Start()

	returnDevice := sync.OnceFunc(func() { close(deviceChan) })
	t.Cleanup(returnDevice) // never leave the worker goroutine blocked, even if an assertion fails

	// the single worker is now durably blocked checking 192.168.0.0; with no
	// worker free, scheduling 192.168.0.1 is durably blocked too
	synctest.Wait()

	// Stop must not wait for a job that was never picked up by a worker
	stopped := make(chan struct{})
	go func() {
		discovery.Stop()
		close(stopped)
	}()
	synctest.Wait()
	select {
	case <-stopped:
		t.Fatal("Stop returned before the in-flight device check completed")
	default:
	}

	// releasing the blocked check lets the pass, and Stop, run to completion
	returnDevice()
	synctest.Wait()
	select {
	case <-stopped:
	default:
		t.Fatal("Stop did not return: it hung waiting on the abandoned job")
	}

	// only the device that was actually checked made it into the cache
	assert.ElementsMatch(t, []string{"192.168.0.0"}, readCachedIPs(t, discovery, cacheKey(checkConfig)))
}
