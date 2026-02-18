// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package listeners

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	"github.com/DataDog/datadog-agent/pkg/snmp"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpintegration"
)

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
		services: services,
		stop:     make(chan bool),
		config:   snmpListenerConfig,
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
