// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package listeners

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	"github.com/DataDog/datadog-agent/pkg/snmp"
	"github.com/DataDog/datadog-agent/pkg/snmp/devicededuper"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpintegration"
)

// ===========================================================================
// Unit tests: config parsing, job dispatch, cache, and service extra config
// ===========================================================================

func TestSNMPListener(t *testing.T) {
	newSvc := make(chan Service, 10)
	delSvc := make(chan Service, 10)
	testChan := make(chan snmpJob, 10)

	snmpConfig := map[string]interface{}{
		"network":   "192.168.0.0/24",
		"community": "public",
		"loader":    "core",
		"authentications": []interface{}{
			map[string]interface{}{
				"user": "someUser",
			},
		},
	}

	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("network_devices.autodiscovery.configs", []interface{}{snmpConfig})
	mockConfig.SetWithoutSource("network_devices.autodiscovery.workers", 1)

	worker = func(_ *SNMPListener, jobs <-chan snmpJob) {
		for {
			job := <-jobs
			testChan <- job
		}
	}

	l, err := NewSNMPListener(ServiceListernerDeps{})
	assert.Equal(t, nil, err)
	l.Listen(newSvc, delSvc)

	job := <-testChan

	assert.Equal(t, "core", job.subnet.config.Loader)
	assert.Equal(t, "snmp", job.subnet.adIdentifier)
	assert.Equal(t, "192.168.0.0", job.currentIP.String())
	assert.Equal(t, "192.168.0.0", job.subnet.startingIP.String())
	assert.Equal(t, "192.168.0.0/24", job.subnet.network.String())
	assert.Equal(t, "public", job.subnet.config.Community)
	assert.Equal(t, "public", job.subnet.config.Authentications[0].Community)
	assert.Equal(t, "someUser", job.subnet.config.Authentications[1].User)

	job = <-testChan
	assert.Equal(t, "192.168.0.1", job.currentIP.String())
	assert.Equal(t, "192.168.0.0", job.subnet.startingIP.String())
}

func TestSNMPListenerSubnets(t *testing.T) {
	newSvc := make(chan Service, 10)
	delSvc := make(chan Service, 10)
	testChan := make(chan snmpJob)

	configs := make([]map[string]interface{}, 0, 100)
	for i := 0; i < 100; i++ {
		snmpConfig := map[string]interface{}{
			"network":      "172.18.0.0/30",
			"community":    "f5-big-ip",
			"port":         1161,
			"context_name": "context" + strconv.Itoa(i),
		}
		configs = append(configs, snmpConfig)
	}

	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("network_devices.autodiscovery.configs", configs)
	mockConfig.SetWithoutSource("network_devices.autodiscovery.workers", 10)

	worker = func(_ *SNMPListener, jobs <-chan snmpJob) {
		for {
			job := <-jobs
			testChan <- job
		}
	}

	snmpListenerConfig, err := snmp.NewListenerConfig()
	assert.Equal(t, nil, err)

	services := map[string]*SNMPService{}
	l := &SNMPListener{
		services:       services,
		stop:           make(chan bool),
		config:         snmpListenerConfig,
		sessionFactory: newGosnmpSession,
	}

	l.Listen(newSvc, delSvc)

	subnets := make(map[string]bool)
	entities := make(map[string]bool)

	for i := 0; i < 400; i++ {
		job := <-testChan
		subnets[fmt.Sprintf("%p", job.subnet)] = true
		entities[job.subnet.config.Digest(job.currentIP.String())] = true
	}

	// make sure we have 100 subnets and 400 different entity hashes
	assert.Equal(t, 100, len(subnets))
	assert.Equal(t, 400, len(entities))
}

func TestSNMPListenerIgnoredAdresses(t *testing.T) {
	newSvc := make(chan Service, 10)
	delSvc := make(chan Service, 10)
	testChan := make(chan snmpJob, 10)

	snmpConfig := map[string]interface{}{
		"network":              "192.168.0.0/24",
		"community":            "public",
		"ignored_ip_addresses": []string{"192.168.0.0"},
	}

	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("network_devices.autodiscovery.configs", []interface{}{snmpConfig})
	mockConfig.SetWithoutSource("network_devices.autodiscovery.workers", 1)

	worker = func(_ *SNMPListener, jobs <-chan snmpJob) {
		for {
			job := <-jobs
			testChan <- job
		}
	}

	l, err := NewSNMPListener(ServiceListernerDeps{})
	assert.Equal(t, nil, err)
	l.Listen(newSvc, delSvc)

	job := <-testChan

	assert.Equal(t, "snmp", job.subnet.adIdentifier)
	assert.Equal(t, "192.168.0.1", job.currentIP.String())
	assert.Equal(t, "192.168.0.0", job.subnet.startingIP.String())

	job = <-testChan
	assert.Equal(t, "192.168.0.2", job.currentIP.String())
	assert.Equal(t, "192.168.0.0", job.subnet.startingIP.String())
}

func TestExtraConfig(t *testing.T) {
	truePtr := true
	fivePtr := 5
	sixPtr := 6
	threePtr := 3

	snmpConfig := snmp.Config{
		Network:      "192.168.0.0/24",
		Community:    "public",
		Timeout:      5,
		Retries:      2,
		OidBatchSize: 10,
		Namespace:    "my-ns",
		InterfaceConfigs: map[string][]snmpintegration.InterfaceConfig{
			"192.168.0.1": {{
				MatchField: "name",
				MatchValue: "eth0",
				InSpeed:    25,
				OutSpeed:   10,
				Tags: []string{
					"customTag1",
					"customTag2:value2",
				},
			}},
		},
		PingConfig: snmpintegration.PingConfig{
			Enabled: &truePtr,
			Linux: snmpintegration.PingLinuxConfig{
				UseRawSocket: &truePtr,
			},
			Interval: &fivePtr,
			Timeout:  &sixPtr,
			Count:    &threePtr,
		},
		UseRemoteConfigProfiles: true,
	}

	svc := SNMPService{
		adIdentifier: "snmp",
		entityID:     "id",
		deviceIP:     "192.168.0.1",
		config:       snmpConfig,
	}

	info, err := svc.GetExtraConfig("autodiscovery_subnet")
	assert.Equal(t, nil, err)
	assert.Equal(t, "192.168.0.0/24", info)

	info, err = svc.GetExtraConfig("community")
	assert.Equal(t, nil, err)
	assert.Equal(t, "public", info)

	info, err = svc.GetExtraConfig("timeout")
	assert.Equal(t, nil, err)
	assert.Equal(t, "5", info)

	info, err = svc.GetExtraConfig("retries")
	assert.Equal(t, nil, err)
	assert.Equal(t, "2", info)

	info, err = svc.GetExtraConfig("oid_batch_size")
	assert.Equal(t, nil, err)
	assert.Equal(t, "10", info)

	info, err = svc.GetExtraConfig("tags")
	assert.Equal(t, nil, err)
	assert.Equal(t, "", info)

	info, err = svc.GetExtraConfig("collect_device_metadata")
	assert.Equal(t, nil, err)
	assert.Equal(t, "false", info)

	svc.config.CollectDeviceMetadata = true
	info, err = svc.GetExtraConfig("collect_device_metadata")
	assert.Equal(t, nil, err)
	assert.Equal(t, "true", info)

	svc.config.CollectDeviceMetadata = false
	info, err = svc.GetExtraConfig("collect_device_metadata")
	assert.Equal(t, nil, err)
	assert.Equal(t, "false", info)

	info, err = svc.GetExtraConfig("collect_topology")
	assert.Equal(t, nil, err)
	assert.Equal(t, "false", info)

	svc.config.CollectTopology = true
	info, err = svc.GetExtraConfig("collect_topology")
	assert.Equal(t, nil, err)
	assert.Equal(t, "true", info)

	svc.config.CollectTopology = false
	info, err = svc.GetExtraConfig("collect_topology")
	assert.Equal(t, nil, err)
	assert.Equal(t, "false", info)

	info, err = svc.GetExtraConfig("collect_vpn")
	assert.Equal(t, nil, err)
	assert.Equal(t, "false", info)

	svc.config.CollectVPN = true
	info, err = svc.GetExtraConfig("collect_vpn")
	assert.Equal(t, nil, err)
	assert.Equal(t, "true", info)

	svc.config.CollectVPN = false
	info, err = svc.GetExtraConfig("collect_vpn")
	assert.Equal(t, nil, err)
	assert.Equal(t, "false", info)

	info, err = svc.GetExtraConfig("min_collection_interval")
	assert.Equal(t, nil, err)
	assert.Equal(t, "0", info)

	svc.config.UseDeviceIDAsHostname = false
	info, err = svc.GetExtraConfig("use_device_id_as_hostname")
	assert.Equal(t, nil, err)
	assert.Equal(t, "false", info)

	svc.config.MinCollectionInterval = 60
	info, err = svc.GetExtraConfig("min_collection_interval")
	assert.Equal(t, nil, err)
	assert.Equal(t, "60", info)

	info, err = svc.GetExtraConfig("namespace")
	assert.Equal(t, nil, err)
	assert.Equal(t, "my-ns", info)

	info, err = svc.GetExtraConfig("interface_configs")
	assert.Equal(t, nil, err)
	assert.Equal(t, `[{"match_field":"name","match_value":"eth0","in_speed":25,"out_speed":10,"tags":["customTag1","customTag2:value2"],"disabled":false}]`, info)

	info, err = svc.GetExtraConfig("ping")
	assert.Equal(t, nil, err)
	assert.Equal(t, `{"linux":{"use_raw_socket":true},"enabled":true,"interval":5,"timeout":6,"count":3}`, info)

	info, err = svc.GetExtraConfig("use_remote_config_profiles")
	assert.Equal(t, nil, err)
	assert.Equal(t, "true", info)

	svc = SNMPService{
		adIdentifier: "snmp",
		entityID:     "id",
		deviceIP:     "192.168.0.99", // without matching interface_configs
		config:       snmpConfig,
	}
	info, err = svc.GetExtraConfig("interface_configs")
	assert.Equal(t, nil, err)
	assert.Equal(t, ``, info)
}

func TestExtraConfigExtraTags(t *testing.T) {
	snmpConfig := snmp.Config{
		Network:   "192.168.0.0/24",
		Community: "public",
		Timeout:   5,
		Retries:   2,
		Tags: []string{
			"tag1:val,1,2",
			"tag2:val_2",
		},
	}

	svc := SNMPService{
		adIdentifier: "snmp",
		entityID:     "id",
		deviceIP:     "192.168.0.1",
		config:       snmpConfig,
	}

	info, err := svc.GetExtraConfig("tags")
	assert.Equal(t, nil, err)
	assert.Equal(t, "tag1:val_1_2,tag2:val_2", info)
}

func TestExtraConfigv3(t *testing.T) {
	snmpConfig := snmp.Config{
		Network:      "192.168.0.0/24",
		User:         "admin",
		AuthKey:      "secret",
		AuthProtocol: "SHA",
		PrivKey:      "private",
		PrivProtocol: "DES",
		Loader:       "core",
	}

	svc := SNMPService{
		adIdentifier: "snmp",
		entityID:     "id",
		deviceIP:     "192.168.0.1",
		config:       snmpConfig,
	}

	info, err := svc.GetExtraConfig("user")
	assert.Equal(t, nil, err)
	assert.Equal(t, "admin", info)

	info, err = svc.GetExtraConfig("auth_key")
	assert.Equal(t, nil, err)
	assert.Equal(t, "secret", info)

	info, err = svc.GetExtraConfig("auth_protocol")
	assert.Equal(t, nil, err)
	assert.Equal(t, "SHA", info)

	info, err = svc.GetExtraConfig("priv_key")
	assert.Equal(t, nil, err)
	assert.Equal(t, "private", info)

	info, err = svc.GetExtraConfig("priv_protocol")
	assert.Equal(t, nil, err)
	assert.Equal(t, "DES", info)

	info, err = svc.GetExtraConfig("loader")
	assert.Equal(t, nil, err)
	assert.Equal(t, "core", info)
}

func TestExtraConfigPingEmpty(t *testing.T) {
	snmpConfig := snmp.Config{
		Network:      "192.168.0.0/24",
		Community:    "public",
		Timeout:      5,
		Retries:      2,
		OidBatchSize: 10,
		Namespace:    "my-ns",
		InterfaceConfigs: map[string][]snmpintegration.InterfaceConfig{
			"192.168.0.1": {{
				MatchField: "name",
				MatchValue: "eth0",
				InSpeed:    25,
				OutSpeed:   10,
				Tags: []string{
					"customTag1",
					"customTag2:value2",
				},
			}},
		},
	}

	svc := SNMPService{
		adIdentifier: "snmp",
		entityID:     "id",
		deviceIP:     "192.168.0.1",
		config:       snmpConfig,
	}

	info, err := svc.GetExtraConfig("ping")
	assert.Equal(t, nil, err)
	assert.Equal(t, `{"linux":{"use_raw_socket":null},"enabled":null,"interval":null,"timeout":null,"count":null}`, info)
}

func TestCache(t *testing.T) {
	tests := []struct {
		name                  string
		devices               map[string]deviceCache
		expectedCacheContents []string
	}{
		{
			name:                  "empty",
			devices:               map[string]deviceCache{},
			expectedCacheContents: []string{},
		},
		{
			name: "single device",
			devices: map[string]deviceCache{
				"device0": {
					IP:        net.ParseIP("192.168.0.2"),
					AuthIndex: 0,
					Failures:  2,
				},
			},
			expectedCacheContents: []string{
				`{"ip":"192.168.0.2", "auth_index":0, "failures":2}`,
			},
		},
		{
			name: "multiple devices",
			devices: map[string]deviceCache{
				"device0": {
					IP:        net.ParseIP("192.168.0.2"),
					AuthIndex: 0,
					Failures:  2,
				},
				"device1": {
					IP:        net.ParseIP("192.168.0.8"),
					AuthIndex: 2,
					Failures:  1,
				},
				"device2": {
					IP:        net.ParseIP("192.168.0.6"),
					AuthIndex: 1,
					Failures:  0,
				},
			},
			expectedCacheContents: []string{
				`{"ip":"192.168.0.2", "auth_index":0, "failures":2}`,
				`{"ip":"192.168.0.8", "auth_index":2, "failures":1}`,
				`{"ip":"192.168.0.6", "auth_index":1, "failures":0}`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDir := t.TempDir()
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("run_path", testDir)

			_, ipNet, err := net.ParseCIDR("192.168.0.0/24")
			assert.NoError(t, err)

			listenerConfigs := []interface{}{
				map[string]interface{}{
					"network":   ipNet.String(),
					"port":      1161,
					"community": "public",
				},
			}

			mockConfig.SetWithoutSource("network_devices.autodiscovery.configs", listenerConfigs)

			listener, err := NewSNMPListener(ServiceListernerDeps{})
			assert.NoError(t, err)

			l, ok := listener.(*SNMPListener)
			assert.True(t, ok)

			cacheKey := "snmp:abc123456abc"

			subnet := &snmpSubnet{
				network:  *ipNet,
				cacheKey: cacheKey,
				devices:  tt.devices,
			}

			l.writeCache(subnet)

			cacheContent, err := persistentcache.Read(cacheKey)
			assert.NoError(t, err)

			var devices []deviceCache
			err = json.Unmarshal([]byte(cacheContent), &devices)
			assert.NoError(t, err)

			assert.Equal(t, len(tt.expectedCacheContents), len(devices))

			for _, expectedCacheContent := range tt.expectedCacheContents {
				var compact bytes.Buffer
				err = json.Compact(&compact, []byte(expectedCacheContent))
				assert.NoError(t, err)

				assert.Contains(t, cacheContent, compact.String())
			}
		})
	}
}

func TestSubnetIndex(t *testing.T) {
	configs := make([]map[string]interface{}, 0, 100)
	for i := 0; i < 100; i++ {
		snmpConfig := map[string]interface{}{
			"network":      "172.18.0.0/30",
			"community":    "f5-big-ip",
			"port":         1161,
			"context_name": "context" + strconv.Itoa(i),
		}
		configs = append(configs, snmpConfig)
	}

	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("network_devices.autodiscovery.configs", configs)

	listener, err := NewSNMPListener(ServiceListernerDeps{})
	assert.NoError(t, err)

	l, ok := listener.(*SNMPListener)
	assert.True(t, ok)

	subnets := l.initializeSubnets()
	for i, subnet := range subnets {
		assert.Equal(t, i, subnet.index)
	}
}

func TestCreateServiceFromCacheRegistersImmediately(t *testing.T) {
	testDir := t.TempDir()
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("run_path", testDir)
	mockConfig.SetWithoutSource("network_devices.autodiscovery.configs", []interface{}{
		map[string]interface{}{
			"network":   "192.168.0.0/30",
			"community": "public",
		},
	})

	listener, err := NewSNMPListener(ServiceListernerDeps{})
	assert.NoError(t, err)
	l := listener.(*SNMPListener)

	newSvc := make(chan Service, 10)
	delSvc := make(chan Service, 10)
	l.newService = newSvc
	l.delService = delSvc

	_, ipNet, err := net.ParseCIDR("192.168.0.0/30")
	assert.NoError(t, err)

	subnet := &snmpSubnet{
		adIdentifier: "snmp",
		config:       l.config.Configs[0],
		network:      *ipNet,
		cacheKey:     "snmp:test123",
		devices:      map[string]deviceCache{},
	}

	deviceInfo := devicededuper.DeviceInfo{
		Name:        "router-1",
		Description: "Test Router",
		BootTimeMs:  1000000,
		SysObjectID: "1.3.6.1.4.1.9.1.1",
	}

	// Simulate loading 192.168.0.1 from cache — should register immediately
	entityID1 := subnet.config.Digest("192.168.0.1")
	l.createService(entityID1, subnet, "192.168.0.1", deviceInfo, 0, 0, true)

	assert.Equal(t, 1, len(newSvc))
	svc := (<-newSvc).(*SNMPService)
	assert.Equal(t, "192.168.0.1", svc.deviceIP)
	assert.False(t, svc.pending)

	// Simulate scan discovering 192.168.0.2 — same physical device, should NOT register
	entityID2 := subnet.config.Digest("192.168.0.2")
	l.createService(entityID2, subnet, "192.168.0.2", deviceInfo, 0, 0, false)

	assert.Equal(t, 0, len(newSvc), "second IP for same device should not be registered")
}

func TestBuildCacheKey(t *testing.T) {
	tests := []struct {
		name       string
		configHash string
		expected   string
	}{
		{
			name:       "empty",
			configHash: "",
			expected:   "snmp:",
		},
		{
			name:       "hash",
			configHash: "abc123456abc",
			expected:   "snmp:abc123456abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, buildCacheKey(tt.configHash))
		})
	}
}

func TestMigrateCache(t *testing.T) {
	tests := []struct {
		name                  string
		subnet                string
		newCacheExists        bool
		legacyCacheExists     bool
		expectMigration       bool
		expectNewCacheUsed    bool
		expectLegacyCacheUsed bool
	}{
		{
			name:                  "no cache exists",
			subnet:                "192.168.1.0/24",
			newCacheExists:        false,
			legacyCacheExists:     false,
			expectMigration:       false,
			expectNewCacheUsed:    true,
			expectLegacyCacheUsed: false,
		},
		{
			name:                  "both caches exist",
			subnet:                "192.168.1.0/24",
			newCacheExists:        true,
			legacyCacheExists:     true,
			expectMigration:       false,
			expectNewCacheUsed:    true,
			expectLegacyCacheUsed: false,
		},
		{
			name:                  "new cache exists and legacy cache does not exist",
			subnet:                "192.168.1.0/24",
			newCacheExists:        true,
			legacyCacheExists:     false,
			expectMigration:       false,
			expectNewCacheUsed:    true,
			expectLegacyCacheUsed: false,
		},
		{
			name:                  "new cache does not exist and legacy cache exists",
			subnet:                "192.168.1.0/24",
			newCacheExists:        false,
			legacyCacheExists:     true,
			expectMigration:       true,
			expectNewCacheUsed:    true,
			expectLegacyCacheUsed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDir := t.TempDir()
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("run_path", testDir)

			mockSnmpConfig := snmp.Config{
				Network:   tt.subnet,
				Port:      1161,
				Loader:    "core",
				Community: "cisco",
				Authentications: []snmp.Authentication{
					{
						Community: "public",
					},
				},
			}
			mockListenerConfigs := []interface{}{
				map[string]interface{}{
					"network":         mockSnmpConfig.Network,
					"port":            mockSnmpConfig.Port,
					"loader":          mockSnmpConfig.Loader,
					"community":       mockSnmpConfig.Community,
					"authentications": mockSnmpConfig.Authentications,
				},
			}
			mockConfig.SetWithoutSource("network_devices.autodiscovery.configs", mockListenerConfigs)
			mockConfig.SetWithoutSource("network_devices.autodiscovery.workers", 1)

			listenerConfig, err := snmp.NewListenerConfig()
			assert.NoError(t, err)

			newConfigHash := listenerConfig.Configs[0].Digest(tt.subnet)
			newCacheKey := buildCacheKey(newConfigHash)
			legacyConfigHash := listenerConfig.Configs[0].LegacyDigest(tt.subnet)
			legacyCacheKey := buildCacheKey(legacyConfigHash)

			if tt.newCacheExists {
				err = persistentcache.Write(newCacheKey, `[{"ip":"192.168.1.6","auth_index":1}]`)
				assert.NoError(t, err)
			}
			if tt.legacyCacheExists {
				err = persistentcache.Write(legacyCacheKey, `[{"ip":"192.168.1.6","auth_index":1}]`)
				assert.NoError(t, err)
			}

			cacheKey := migrateCache(listenerConfig.Configs[0])

			if tt.expectMigration {
				assert.True(t, persistentcache.Exists(newCacheKey))
				assert.False(t, persistentcache.Exists(legacyCacheKey))
			}

			if tt.expectNewCacheUsed {
				assert.Equal(t, newCacheKey, cacheKey)
			}
			if tt.expectLegacyCacheUsed {
				assert.Equal(t, legacyCacheKey, cacheKey)
			}
		})
	}
}

// ===========================================================================
// Integration tests: device discovery flow with fake SNMP sessions
// ===========================================================================

var defaultWorkerFunc = worker

func setupTestListener(t *testing.T, configs []interface{}, extraOpts map[string]interface{}) (*SNMPListener, *testSessionFactory) {
	t.Helper()

	// Restore worker to the default in case a previous test overrode it
	worker = defaultWorkerFunc

	testDir := t.TempDir()
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("run_path", testDir)
	mockConfig.SetWithoutSource("network_devices.autodiscovery.configs", configs)
	mockConfig.SetWithoutSource("network_devices.autodiscovery.workers", 1)
	for k, v := range extraOpts {
		mockConfig.SetWithoutSource(k, v)
	}

	factory := newTestSessionFactory()

	listenerConfig, err := snmp.NewListenerConfig()
	require.NoError(t, err)

	l := &SNMPListener{
		services:       map[string]*SNMPService{},
		stop:           make(chan bool),
		config:         listenerConfig,
		deviceDeduper:  devicededuper.NewDeviceDeduper(listenerConfig),
		sessionFactory: factory.build,
	}
	return l, factory
}

func collectServices(t *testing.T, ch <-chan Service, expected int, timeout time.Duration) []*SNMPService {
	t.Helper()
	var services []*SNMPService
	for i := 0; i < expected; i++ {
		select {
		case svc := <-ch:
			services = append(services, svc.(*SNMPService))
		case <-time.After(timeout):
			t.Fatalf("timed out waiting for service %d/%d", i+1, expected)
		}
	}
	return services
}

func noMoreServices(t *testing.T, ch <-chan Service, wait time.Duration) {
	t.Helper()
	select {
	case svc := <-ch:
		t.Fatalf("unexpected service: %v", svc.(*SNMPService).deviceIP)
	case <-time.After(wait):
		// OK
	}
}

func TestCheckDeviceReachableSuccess(t *testing.T) {
	l, factory := setupTestListener(t, []interface{}{
		map[string]interface{}{"network": "192.168.0.0/30", "community": "public"},
	}, nil)

	factory.sessions["192.168.0.1"] = makeReachableSession()

	auth := snmp.Authentication{Community: "public"}
	assert.True(t, l.checkDeviceReachable(auth, 161, "192.168.0.1"))
}

func TestCheckDeviceReachableConnectError(t *testing.T) {
	l, factory := setupTestListener(t, []interface{}{
		map[string]interface{}{"network": "192.168.0.0/30", "community": "public"},
	}, nil)

	factory.sessions["192.168.0.1"] = &errorSession{connectErr: errors.New("timeout")}

	auth := snmp.Authentication{Community: "public"}
	assert.False(t, l.checkDeviceReachable(auth, 161, "192.168.0.1"))
}

func TestCheckDeviceReachableGetNextError(t *testing.T) {
	l, factory := setupTestListener(t, []interface{}{
		map[string]interface{}{"network": "192.168.0.0/30", "community": "public"},
	}, nil)

	factory.sessions["192.168.0.1"] = &errorSession{getNextErr: errors.New("request timeout")}

	auth := snmp.Authentication{Community: "public"}
	assert.False(t, l.checkDeviceReachable(auth, 161, "192.168.0.1"))
}

func TestCheckDeviceReachableEmptyVars(t *testing.T) {
	l, factory := setupTestListener(t, []interface{}{
		map[string]interface{}{"network": "192.168.0.0/30", "community": "public"},
	}, nil)

	factory.sessions["192.168.0.1"] = &errorSession{
		getNextPkt: &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{}},
	}

	auth := snmp.Authentication{Community: "public"}
	assert.False(t, l.checkDeviceReachable(auth, 161, "192.168.0.1"))
}

func TestCheckDeviceReachableNilValue(t *testing.T) {
	l, factory := setupTestListener(t, []interface{}{
		map[string]interface{}{"network": "192.168.0.0/30", "community": "public"},
	}, nil)

	factory.sessions["192.168.0.1"] = &errorSession{
		getNextPkt: &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{
			{Name: "1.0.0.1", Type: gosnmp.NoSuchObject, Value: nil},
		}},
	}

	auth := snmp.Authentication{Community: "public"}
	assert.False(t, l.checkDeviceReachable(auth, 161, "192.168.0.1"))
}

func TestCheckDeviceInfoParsing(t *testing.T) {
	l, _ := setupTestListener(t, []interface{}{
		map[string]interface{}{"network": "192.168.0.0/30", "community": "public"},
	}, map[string]interface{}{
		"network_devices.autodiscovery.use_deduplication": true,
	})

	sess := makeReachableSessionWithDeviceInfo("router-1", "Cisco IOS", "1.3.6.1.4.1.9.1.1", 100000)
	l.sessionFactory = func(_ snmp.Authentication, _ string, _ uint16) (snmpSession, error) {
		return sess, nil
	}

	auth := snmp.Authentication{Community: "public"}
	info := l.checkDeviceInfo(auth, 161, "192.168.0.1")

	assert.Equal(t, "router-1", info.Name)
	assert.Equal(t, "Cisco IOS", info.Description)
	assert.Equal(t, "1.3.6.1.4.1.9.1.1", info.SysObjectID)
	// sysUptime = 100000 hundredths of a second = 1000 seconds
	// BootTimeMs should be approximately now - 1000s
	assert.InDelta(t, time.Now().Add(-1000*time.Second).UnixMilli(), info.BootTimeMs, 5000)
}

func TestCheckDeviceInfoNoDedup(t *testing.T) {
	l, factory := setupTestListener(t, []interface{}{
		map[string]interface{}{"network": "192.168.0.0/30", "community": "public"},
	}, nil)
	// Deduplicate defaults to false

	// Should not make any SNMP calls
	auth := snmp.Authentication{Community: "public"}
	info := l.checkDeviceInfo(auth, 161, "192.168.0.1")

	assert.Equal(t, devicededuper.DeviceInfo{}, info)
	assert.Empty(t, factory.calls, "no SNMP calls should be made when dedup is disabled")
}

func TestListenCreatesService(t *testing.T) {
	l, factory := setupTestListener(t, []interface{}{
		map[string]interface{}{"network": "192.168.0.0/30", "community": "public"},
	}, nil)
	t.Cleanup(func() { l.Stop() })

	factory.sessions["192.168.0.1"] = makeReachableSession()

	newSvc := make(chan Service, 10)
	delSvc := make(chan Service, 10)
	l.Listen(newSvc, delSvc)

	services := collectServices(t, newSvc, 1, 5*time.Second)
	svc := services[0]

	assert.Equal(t, "192.168.0.1", svc.deviceIP)
	assert.Equal(t, "snmp", svc.adIdentifier)

	// Verify Service interface methods
	hosts, err := svc.GetHosts()
	require.NoError(t, err)
	assert.Equal(t, "192.168.0.1", hosts[""])

	ports, err := svc.GetPorts()
	require.NoError(t, err)
	assert.Equal(t, 161, ports[0].Port)

	adIDs := svc.GetADIdentifiers()
	assert.Equal(t, []string{"snmp"}, adIDs)

	community, err := svc.GetExtraConfig("community")
	require.NoError(t, err)
	assert.Equal(t, "public", community)
}

func TestListenMultipleSubnets(t *testing.T) {
	l, factory := setupTestListener(t, []interface{}{
		map[string]interface{}{"network": "192.168.0.0/30", "community": "public"},
		map[string]interface{}{"network": "10.0.0.0/30", "community": "private"},
	}, nil)
	t.Cleanup(func() { l.Stop() })

	factory.sessions["192.168.0.1"] = makeReachableSession()
	factory.sessions["10.0.0.1"] = makeReachableSession()

	newSvc := make(chan Service, 10)
	delSvc := make(chan Service, 10)
	l.Listen(newSvc, delSvc)

	services := collectServices(t, newSvc, 2, 5*time.Second)

	ips := map[string]string{}
	for _, svc := range services {
		community, _ := svc.GetExtraConfig("community")
		ips[svc.deviceIP] = community
	}

	assert.Equal(t, "public", ips["192.168.0.1"])
	assert.Equal(t, "private", ips["10.0.0.1"])
}

func TestListenMultipleAuthFirstMatch(t *testing.T) {
	l, factory := setupTestListener(t, []interface{}{
		map[string]interface{}{
			"network": "192.168.0.0/30",
			"authentications": []interface{}{
				map[string]interface{}{"community_string": "first"},
				map[string]interface{}{"community_string": "second"},
			},
		},
	}, nil)
	t.Cleanup(func() { l.Stop() })

	// Device responds to first auth
	factory.sessions["192.168.0.1:first"] = makeReachableSession()

	newSvc := make(chan Service, 10)
	delSvc := make(chan Service, 10)
	l.Listen(newSvc, delSvc)

	services := collectServices(t, newSvc, 1, 5*time.Second)
	community, _ := services[0].GetExtraConfig("community")
	assert.Equal(t, "first", community)
}

func TestListenMultipleAuthFirstMatchWithDeduplication(t *testing.T) {
	l, factory := setupTestListener(t, []interface{}{
		map[string]interface{}{
			"network": "192.168.0.0/30",
			"authentications": []interface{}{
				map[string]interface{}{"community_string": "first"},
				map[string]interface{}{"community_string": "second"},
			},
		},
	}, map[string]interface{}{
		"network_devices.autodiscovery.use_deduplication": true,
	})
	t.Cleanup(func() { l.Stop() })

	// Device responds to first auth with full device info so dedup path is exercised
	factory.sessions["192.168.0.1:first"] = makeReachableSessionWithDeviceInfo(
		"router-1", "Cisco IOS", "1.3.6.1.4.1.9.1.1", 100000,
	)

	newSvc := make(chan Service, 10)
	delSvc := make(chan Service, 10)
	l.Listen(newSvc, delSvc)

	services := collectServices(t, newSvc, 1, 5*time.Second)
	community, _ := services[0].GetExtraConfig("community")
	assert.Equal(t, "first", community)
}

func TestListenMultipleAuthSecondMatch(t *testing.T) {
	l, factory := setupTestListener(t, []interface{}{
		map[string]interface{}{
			"network": "192.168.0.0/30",
			"authentications": []interface{}{
				map[string]interface{}{"community_string": "wrong"},
				map[string]interface{}{"community_string": "correct"},
			},
		},
	}, nil)
	t.Cleanup(func() { l.Stop() })

	// Device responds only to second auth
	factory.sessions["192.168.0.1:correct"] = makeReachableSession()

	newSvc := make(chan Service, 10)
	delSvc := make(chan Service, 10)
	l.Listen(newSvc, delSvc)

	services := collectServices(t, newSvc, 1, 5*time.Second)
	community, _ := services[0].GetExtraConfig("community")
	assert.Equal(t, "correct", community)
}

func TestListenUnreachable(t *testing.T) {
	l, _ := setupTestListener(t, []interface{}{
		map[string]interface{}{"network": "192.168.0.0/30", "community": "public"},
	}, nil)
	t.Cleanup(func() { l.Stop() })

	// No sessions configured — all devices unreachable

	newSvc := make(chan Service, 10)
	delSvc := make(chan Service, 10)
	l.Listen(newSvc, delSvc)

	// /30 has 4 IPs; with 1 worker processing sequentially, allow time for all
	noMoreServices(t, newSvc, 3*time.Second)
}

func TestListenDeduplication(t *testing.T) {
	l, factory := setupTestListener(t, []interface{}{
		map[string]interface{}{"network": "192.168.0.0/30", "community": "public"},
	}, map[string]interface{}{
		"network_devices.autodiscovery.use_deduplication": true,
	})
	t.Cleanup(func() { l.Stop() })

	// Both IPs respond with identical device info → only lowest IP should be registered
	factory.sessions["192.168.0.0"] = makeReachableSessionWithDeviceInfo("router-1", "Cisco IOS", "1.3.6.1.4.1.9.1.1", 100000)
	factory.sessions["192.168.0.1"] = makeReachableSessionWithDeviceInfo("router-1", "Cisco IOS", "1.3.6.1.4.1.9.1.1", 100000)
	factory.sessions["192.168.0.2"] = makeReachableSessionWithDeviceInfo("router-1", "Cisco IOS", "1.3.6.1.4.1.9.1.1", 100000)
	factory.sessions["192.168.0.3"] = makeReachableSessionWithDeviceInfo("router-1", "Cisco IOS", "1.3.6.1.4.1.9.1.1", 100000)

	newSvc := make(chan Service, 10)
	delSvc := make(chan Service, 10)
	l.Listen(newSvc, delSvc)

	services := collectServices(t, newSvc, 1, 5*time.Second)
	assert.Equal(t, "192.168.0.0", services[0].deviceIP)

	noMoreServices(t, newSvc, 2*time.Second)
}

func TestListenDeduplicationDifferentDevices(t *testing.T) {
	l, factory := setupTestListener(t, []interface{}{
		map[string]interface{}{"network": "192.168.0.0/30", "community": "public"},
	}, map[string]interface{}{
		"network_devices.autodiscovery.use_deduplication": true,
	})
	t.Cleanup(func() { l.Stop() })

	// Two IPs respond with different sysName → both should be registered
	factory.sessions["192.168.0.0"] = makeReachableSessionWithDeviceInfo("router-1", "Cisco IOS", "1.3.6.1.4.1.9.1.1", 100000)
	factory.sessions["192.168.0.1"] = makeReachableSessionWithDeviceInfo("switch-1", "Cisco NX-OS", "1.3.6.1.4.1.9.1.2", 200000)

	newSvc := make(chan Service, 10)
	delSvc := make(chan Service, 10)
	l.Listen(newSvc, delSvc)

	services := collectServices(t, newSvc, 2, 5*time.Second)
	ips := []string{services[0].deviceIP, services[1].deviceIP}
	sort.Strings(ips)
	assert.Equal(t, []string{"192.168.0.0", "192.168.0.1"}, ips)
}

func TestCheckDeviceDeleteAfterAllowedFailures(t *testing.T) {
	l, factory := setupTestListener(t, []interface{}{
		map[string]interface{}{"network": "192.168.0.0/30", "community": "public"},
	}, nil)

	newSvc := make(chan Service, 10)
	delSvc := make(chan Service, 10)
	l.newService = newSvc
	l.delService = delSvc
	l.config.AllowedFailures = 3

	subnets := l.initializeSubnets()
	subnet := &subnets[0]

	// First scan: device is reachable
	factory.sessions["192.168.0.1"] = makeReachableSession()
	job := snmpJob{subnet: subnet, currentIP: net.ParseIP("192.168.0.1")}
	l.checkDevice(job)

	services := collectServices(t, newSvc, 1, 2*time.Second)
	assert.Equal(t, "192.168.0.1", services[0].deviceIP)

	// Remove the reachable session so device becomes unreachable
	factory.mu.Lock()
	delete(factory.sessions, "192.168.0.1")
	factory.mu.Unlock()

	// Scan 3 more times (failures 1, 2, 3)
	for i := 0; i < 3; i++ {
		l.checkDevice(job)
	}

	// After 3 failures, device should be deleted
	deleted := collectServices(t, delSvc, 1, 2*time.Second)
	assert.Equal(t, "192.168.0.1", deleted[0].deviceIP)
}

func TestCheckDeviceFailureResetOnSuccess(t *testing.T) {
	l, factory := setupTestListener(t, []interface{}{
		map[string]interface{}{"network": "192.168.0.0/30", "community": "public"},
	}, nil)

	newSvc := make(chan Service, 10)
	delSvc := make(chan Service, 10)
	l.newService = newSvc
	l.delService = delSvc
	l.config.AllowedFailures = 3

	subnets := l.initializeSubnets()
	subnet := &subnets[0]

	// First scan: device is reachable
	factory.sessions["192.168.0.1"] = makeReachableSession()
	job := snmpJob{subnet: subnet, currentIP: net.ParseIP("192.168.0.1")}
	l.checkDevice(job)
	collectServices(t, newSvc, 1, 2*time.Second)

	// Make device unreachable for 2 scans (below threshold of 3)
	factory.mu.Lock()
	delete(factory.sessions, "192.168.0.1")
	factory.mu.Unlock()

	l.checkDevice(job)
	l.checkDevice(job)

	// Make device reachable again
	factory.mu.Lock()
	factory.sessions["192.168.0.1"] = makeReachableSession()
	factory.mu.Unlock()

	l.checkDevice(job)

	// Should not have been deleted
	noMoreServices(t, delSvc, 500*time.Millisecond)

	// Verify failures were reset
	entityID := subnet.config.Digest("192.168.0.1")
	device, exists := subnet.devices[entityID]
	assert.True(t, exists)
	assert.Equal(t, 0, device.Failures)
}
