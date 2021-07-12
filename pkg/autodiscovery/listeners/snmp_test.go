// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package listeners

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/snmp"

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

	mockConfig := config.Mock()
	mockConfig.Set("snmp_listener", listenerConfig)

	worker = func(l *SNMPListener, jobs <-chan snmpJob) {
		for {
			job := <-jobs
			testChan <- job
		}
	}

	l, err := NewSNMPListener()
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

	mockConfig := config.Mock()
	mockConfig.Set("snmp_listener", listenerConfig)

	worker = func(l *SNMPListener, jobs <-chan snmpJob) {
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

	mockConfig := config.Mock()
	mockConfig.Set("snmp_listener", listenerConfig)

	worker = func(l *SNMPListener, jobs <-chan snmpJob) {
		for {
			job := <-jobs
			testChan <- job
		}
	}

	l, err := NewSNMPListener()
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
	snmpConfig := snmp.Config{
		Network:      "192.168.0.0/24",
		Community:    "public",
		Timeout:      5,
		Retries:      2,
		OidBatchSize: 10,
	}

	svc := SNMPService{
		adIdentifier: "snmp",
		entityID:     "id",
		deviceIP:     "192.168.0.1",
		creationTime: integration.Before,
		config:       snmpConfig,
	}

	info, err := svc.GetExtraConfig([]byte("autodiscovery_subnet"))
	assert.Equal(t, nil, err)
	assert.Equal(t, "192.168.0.0/24", string(info))

	info, err = svc.GetExtraConfig([]byte("community"))
	assert.Equal(t, nil, err)
	assert.Equal(t, "public", string(info))

	info, err = svc.GetExtraConfig([]byte("timeout"))
	assert.Equal(t, nil, err)
	assert.Equal(t, "5", string(info))

	info, err = svc.GetExtraConfig([]byte("retries"))
	assert.Equal(t, nil, err)
	assert.Equal(t, "2", string(info))

	info, err = svc.GetExtraConfig([]byte("oid_batch_size"))
	assert.Equal(t, nil, err)
	assert.Equal(t, "10", string(info))

	info, err = svc.GetExtraConfig([]byte("tags"))
	assert.Equal(t, nil, err)
	assert.Equal(t, "", string(info))

	info, err = svc.GetExtraConfig([]byte("collect_device_metadata"))
	assert.Equal(t, nil, err)
	assert.Equal(t, "false", string(info))

	svc.config.CollectDeviceMetadata = true
	info, err = svc.GetExtraConfig([]byte("collect_device_metadata"))
	assert.Equal(t, nil, err)
	assert.Equal(t, "true", string(info))

	svc.config.CollectDeviceMetadata = false
	info, err = svc.GetExtraConfig([]byte("collect_device_metadata"))
	assert.Equal(t, nil, err)
	assert.Equal(t, "false", string(info))
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
		creationTime: integration.Before,
		config:       snmpConfig,
	}

	info, err := svc.GetExtraConfig([]byte("tags"))
	assert.Equal(t, nil, err)
	assert.Equal(t, "tag1:val_1_2,tag2:val_2", string(info))
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
		creationTime: integration.Before,
		config:       snmpConfig,
	}

	info, err := svc.GetExtraConfig([]byte("user"))
	assert.Equal(t, nil, err)
	assert.Equal(t, "admin", string(info))

	info, err = svc.GetExtraConfig([]byte("auth_key"))
	assert.Equal(t, nil, err)
	assert.Equal(t, "secret", string(info))

	info, err = svc.GetExtraConfig([]byte("auth_protocol"))
	assert.Equal(t, nil, err)
	assert.Equal(t, "SHA", string(info))

	info, err = svc.GetExtraConfig([]byte("priv_key"))
	assert.Equal(t, nil, err)
	assert.Equal(t, "private", string(info))

	info, err = svc.GetExtraConfig([]byte("priv_protocol"))
	assert.Equal(t, nil, err)
	assert.Equal(t, "DES", string(info))

	info, err = svc.GetExtraConfig([]byte("loader"))
	assert.Equal(t, nil, err)
	assert.Equal(t, "core", string(info))
}
