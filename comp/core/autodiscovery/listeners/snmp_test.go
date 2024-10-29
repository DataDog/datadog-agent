// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package listeners

import (
	"fmt"
	"strconv"
	"testing"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/snmp"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpintegration"

	"github.com/stretchr/testify/assert"
)

func TestSNMPListener(t *testing.T) {
	newSvc := make(chan Service, 10)
	delSvc := make(chan Service, 10)
	testChan := make(chan snmpJob, 10)

	snmpConfig := snmp.Config{
		Network:   "192.168.0.0/24",
		Community: "public",
		Loader:    "core",
	}
	listenerConfig := snmp.ListenerConfig{
		Configs: []snmp.Config{snmpConfig},
		Workers: 1,
	}

	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("network_devices.autodiscovery", listenerConfig)

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

	job = <-testChan
	assert.Equal(t, "192.168.0.1", job.currentIP.String())
	assert.Equal(t, "192.168.0.0", job.subnet.startingIP.String())
}

func TestSNMPListenerSubnets(t *testing.T) {
	newSvc := make(chan Service, 10)
	delSvc := make(chan Service, 10)
	testChan := make(chan snmpJob)

	listenerConfig := snmp.ListenerConfig{
		Configs: []snmp.Config{},
		Workers: 10,
	}

	for i := 0; i < 100; i++ {
		snmpConfig := snmp.Config{
			Network:     "172.18.0.0/30",
			Community:   "f5-big-ip",
			Port:        1161,
			ContextName: "context" + strconv.Itoa(i),
		}
		listenerConfig.Configs = append(listenerConfig.Configs, snmpConfig)
	}

	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("network_devices.autodiscovery", listenerConfig)

	worker = func(_ *SNMPListener, jobs <-chan snmpJob) {
		for {
			job := <-jobs
			testChan <- job
		}
	}

	snmpListenerConfig, err := snmp.NewListenerConfig()
	assert.Equal(t, nil, err)

	services := map[string]Service{}
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

	snmpConfig := snmp.Config{
		Network:            "192.168.0.0/24",
		Community:          "public",
		IgnoredIPAddresses: map[string]bool{"192.168.0.0": true},
	}
	listenerConfig := snmp.ListenerConfig{
		Configs: []snmp.Config{snmpConfig},
		Workers: 1,
	}

	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("network_devices.autodiscovery", listenerConfig)

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
	assert.Equal(t, `[{"match_field":"name","match_value":"eth0","in_speed":25,"out_speed":10,"tags":["customTag1","customTag2:value2"]}]`, info)

	info, err = svc.GetExtraConfig("ping")
	assert.Equal(t, nil, err)
	assert.Equal(t, `{"linux":{"use_raw_socket":true},"enabled":true,"interval":5,"timeout":6,"count":3}`, info)

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
