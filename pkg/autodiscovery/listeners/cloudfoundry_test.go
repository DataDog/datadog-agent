// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build clusterchecks

package listeners

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/cloudfoundry"
	"github.com/stretchr/testify/assert"
)

type bbsCacheFake struct {
	sync.RWMutex
	Updated     time.Time
	ActualLRPs  map[string][]cloudfoundry.ActualLRP
	DesiredLRPs []cloudfoundry.DesiredLRP
}

func (b *bbsCacheFake) LastUpdated() time.Time {
	b.RLock()
	defer b.RUnlock()
	return b.Updated
}

func (b *bbsCacheFake) GetPollAttempts() int {
	panic("implement me")
}

func (b *bbsCacheFake) GetPollSuccesses() int {
	panic("implement me")
}

func (b *bbsCacheFake) GetActualLRPsFor(appGUID string) []cloudfoundry.ActualLRP {
	b.RLock()
	defer b.RUnlock()
	lrps, ok := b.ActualLRPs[appGUID]
	if !ok {
		lrps = []cloudfoundry.ActualLRP{}
	}
	return lrps
}

func (b *bbsCacheFake) GetDesiredLRPs() []cloudfoundry.DesiredLRP {
	b.RLock()
	defer b.RUnlock()
	return b.DesiredLRPs
}

func (b *bbsCacheFake) GetAllLRPs() (map[string][]cloudfoundry.ActualLRP, []cloudfoundry.DesiredLRP) {
	b.RLock()
	defer b.RUnlock()
	return b.ActualLRPs, b.DesiredLRPs
}

var testBBSCache *bbsCacheFake = &bbsCacheFake{}

func TestCloudFoundryListener(t *testing.T) {
	newSvc := make(chan Service, 10)
	delSvc := make(chan Service, 10)
	cfl := CloudFoundryListener{
		bbsCache:      testBBSCache,
		refreshTicker: time.NewTicker(10 * time.Millisecond),
		stop:          make(chan bool),
		services:      map[string]Service{},
	}
	cfl.Listen(newSvc, delSvc)
	defer cfl.Stop()

	for _, tc := range []struct {
		aLRP   map[string][]cloudfoundry.ActualLRP
		dLRP   []cloudfoundry.DesiredLRP
		expNew map[string]Service
		expDel map[string]Service
	}{
		{
			// inputs with no AD_DATADOGHQ_COM set up => no services
			aLRP: map[string][]cloudfoundry.ActualLRP{
				"appguid1": {{AppGUID: "appguid1", CellID: "cellX", Index: 0}, {AppGUID: "appguid1", CellID: "cellY", Index: 1}},
			},
			dLRP: []cloudfoundry.DesiredLRP{
				{AppGUID: "appguid1", ProcessGUID: "processguid1"},
			},
			expNew: map[string]Service{},
			expDel: map[string]Service{},
		},
		{
			// inputs with AD_DATADOGHQ_COM containing config only for containers, but no containers of the app exist
			aLRP: map[string][]cloudfoundry.ActualLRP{
				"appguid1": {{AppGUID: "appguid1", CellID: "cellX", Index: 0}, {AppGUID: "appguid1", CellID: "cellY", Index: 1}},
			},
			dLRP: []cloudfoundry.DesiredLRP{
				{
					AppGUID:     "differentappguid",
					ProcessGUID: "processguid1",
					EnvAD: cloudfoundry.ADConfig{"flask-app": map[string]json.RawMessage{
						"check_names":  json.RawMessage(`["http_check"]`),
						"init_configs": json.RawMessage(`[{}]`),
						"instances":    json.RawMessage(`[{"name": "My Nginx", "url": "http://%%host%%:%%port_p8080%%", "timeout": 1}]`),
					}},
				},
			},
			expNew: map[string]Service{},
			expDel: map[string]Service{},
		},
		{
			// inputs with AD_DATADOGHQ_COM containing config only for containers, 1 container exists for the app
			aLRP: map[string][]cloudfoundry.ActualLRP{
				"appguid1": {
					{
						AppGUID:     "appguid1",
						CellID:      "cellX",
						ContainerIP: "1.2.3.4",
						Index:       0,
						Ports:       []uint32{11, 22},
						State:       cloudfoundry.ActualLrpStateRunning,
					},
				},
				"differentappguid1": {
					{
						AppGUID:     "differentappguid1",
						CellID:      "cellY",
						ContainerIP: "1.2.3.5",
						Index:       1,
						Ports:       []uint32{33, 44},
						State:       cloudfoundry.ActualLrpStateRunning,
					},
				},
			},
			dLRP: []cloudfoundry.DesiredLRP{
				{
					AppGUID:     "appguid1",
					ProcessGUID: "processguid1",
					EnvAD: cloudfoundry.ADConfig{"flask-app": map[string]json.RawMessage{
						"check_names":  json.RawMessage(`["http_check"]`),
						"init_configs": json.RawMessage(`[{}]`),
						"instances":    json.RawMessage(`[{"name": "My Nginx", "url": "http://%%host%%:%%port_p8080%%", "timeout": 1}]`),
					}},
				},
			},
			expNew: map[string]Service{
				"processguid1/flask-app/0": &CloudFoundryService{
					containerIPs:   map[string]string{CfServiceContainerIP: "1.2.3.4"},
					containerPorts: []ContainerPort{{Port: 11, Name: "p11"}, {Port: 22, Name: "p22"}},
					creationTime:   integration.After,
				},
			},
			expDel: map[string]Service{},
		},
		{
			// now the service created above should be deleted
			aLRP:   map[string][]cloudfoundry.ActualLRP{},
			dLRP:   []cloudfoundry.DesiredLRP{},
			expNew: map[string]Service{},
			expDel: map[string]Service{
				"processguid1/flask-app/0": &CloudFoundryService{
					containerIPs:   map[string]string{CfServiceContainerIP: "1.2.3.4"},
					containerPorts: []ContainerPort{{Port: 11, Name: "p11"}, {Port: 22, Name: "p22"}},
					creationTime:   integration.After,
				},
			},
		},
		{
			// inputs with AD_DATADOGHQ_COM containing config only for non-containers, no container exists for the app
			aLRP: map[string][]cloudfoundry.ActualLRP{
				"differentappguid1": {{AppGUID: "differentappguid1", CellID: "cellX", Index: 1}},
			},
			dLRP: []cloudfoundry.DesiredLRP{
				{
					AppGUID:     "myappguid1",
					ProcessGUID: "myprocessguid1",
					EnvAD: cloudfoundry.ADConfig{"my-postgres": map[string]json.RawMessage{
						"check_names":  json.RawMessage(`["postgres"]`),
						"init_configs": json.RawMessage(`[{}]`),
						"instances":    json.RawMessage(`[{"host": "%%host%%", "port": 5432, "username": "%%username%%", "dbname": "%%dbname%%", "password": "%%password%%"}]`),
						"variables":    json.RawMessage(`{"host":"$.credentials.host","username":"$.credentials.Username","password":"$.credentials.Password","dbname":"$.credentials.database_name"}`),
					}},
					EnvVcapServices: map[string][]byte{"my-postgres": []byte(`{"credentials":{"host":"a.b.c","Username":"me","Password":"secret","database_name":"mydb"}}`)},
				},
			},
			expNew: map[string]Service{
				"myprocessguid1/my-postgres": &CloudFoundryService{
					containerIPs:   map[string]string{},
					containerPorts: []ContainerPort{},
					creationTime:   integration.After,
				},
			},
			expDel: map[string]Service{},
		},
		{
			// complex test with three apps, one having no AD configuration, two having different configurations
			// for both container and non-container services (plus also a non-running container)
			aLRP: map[string][]cloudfoundry.ActualLRP{
				"appguid1": {
					{
						AppGUID:     "appguid1",
						CellID:      "cellX",
						ContainerIP: "1.2.3.4",
						Index:       0,
						Ports:       []uint32{11, 22},
						State:       cloudfoundry.ActualLrpStateRunning,
					},
					{
						AppGUID:     "appguid1",
						CellID:      "cellY",
						ContainerIP: "1.2.3.5",
						Index:       1,
						Ports:       []uint32{33, 44},
						State:       cloudfoundry.ActualLrpStateRunning,
					},
					{
						AppGUID:     "appguid1",
						CellID:      "cellZ",
						ContainerIP: "1.2.3.6",
						Index:       2,
						Ports:       []uint32{55, 66},
						State:       "NOTRUNNING",
					},
				},
				"appguid2": {
					{
						AppGUID:     "appguid2",
						CellID:      "cellY",
						ContainerIP: "1.2.3.7",
						Index:       0,
						Ports:       []uint32{77, 88},
						State:       cloudfoundry.ActualLrpStateRunning,
					},
					{
						AppGUID:     "appguid2",
						CellID:      "cellZ",
						ContainerIP: "1.2.3.8",
						Index:       1,
						Ports:       []uint32{99, 111},
						State:       cloudfoundry.ActualLrpStateRunning,
					},
				},
				"appguid3": {
					{
						AppGUID:     "appguid3",
						CellID:      "cellZ",
						ContainerIP: "1.2.3.9",
						Index:       0,
						Ports:       []uint32{222, 333},
						State:       cloudfoundry.ActualLrpStateRunning,
					},
					{
						AppGUID:     "appguid3",
						CellID:      "cellZ",
						ContainerIP: "1.2.3.10",
						Index:       1,
						Ports:       []uint32{444, 555},
						State:       cloudfoundry.ActualLrpStateRunning,
					},
				},
			},
			dLRP: []cloudfoundry.DesiredLRP{
				{
					AppGUID:     "appguid1",
					ProcessGUID: "processguid1",
					EnvAD: cloudfoundry.ADConfig{
						"my-postgres": map[string]json.RawMessage{
							"check_names":  json.RawMessage(`["postgres"]`),
							"init_configs": json.RawMessage(`[{}]`),
							"instances":    json.RawMessage(`[{"host": "%%host%%", "port": 5432, "username": "%%username%%", "dbname": "%%dbname%%", "password": "%%password%%"}]`),
							"variables":    json.RawMessage(`{"host":"$.credentials.host","username":"$.credentials.Username","password":"$.credentials.Password","dbname":"$.credentials.database_name"}`),
						},
						"flask-app": map[string]json.RawMessage{
							"check_names":  json.RawMessage(`["http_check"]`),
							"init_configs": json.RawMessage(`[{}]`),
							"instances":    json.RawMessage(`[{"name": "My Nginx", "url": "http://%%host%%:%%port_p8080%%", "timeout": 1}]`),
						},
					},
					EnvVcapServices: map[string][]byte{"my-postgres": []byte(`{"credentials":{"host":"a.b.c","Username":"me","Password":"secret","database_name":"mydb"}}`)},
				},
				{
					AppGUID:     "appguid2",
					ProcessGUID: "processguid2",
					EnvAD: cloudfoundry.ADConfig{
						"my-postgres": map[string]json.RawMessage{
							"check_names":  json.RawMessage(`["postgres"]`),
							"init_configs": json.RawMessage(`[{}]`),
							"instances":    json.RawMessage(`[{"host": "%%host%%", "port": 5432, "username": "%%username%%", "dbname": "%%dbname%%", "password": "%%password%%"}]`),
							"variables":    json.RawMessage(`{"host":"$.credentials.host","username":"$.credentials.Username","password":"$.credentials.Password","dbname":"$.credentials.database_name"}`),
						},
						"flask-app": map[string]json.RawMessage{
							"check_names":  json.RawMessage(`["http_check"]`),
							"init_configs": json.RawMessage(`[{}]`),
							"instances":    json.RawMessage(`[{"name": "My Nginx", "url": "http://%%host%%:%%port_p8080%%", "timeout": 1}]`),
						},
					},
					EnvVcapServices: map[string][]byte{"my-postgres": []byte(`{"credentials":{"host":"a.b.c","Username":"me","Password":"secret","database_name":"mydb"}}`)},
				},
				{
					AppGUID:     "appguid3",
					ProcessGUID: "processguid3",
				},
			},
			expDel: map[string]Service{
				"myprocessguid1/my-postgres": &CloudFoundryService{
					containerIPs:   map[string]string{},
					containerPorts: []ContainerPort{},
					creationTime:   integration.After,
				},
			},
			expNew: map[string]Service{
				"processguid1/flask-app/0": &CloudFoundryService{
					containerIPs: map[string]string{CfServiceContainerIP: "1.2.3.4"},
					containerPorts: []ContainerPort{
						{
							Name: "p11",
							Port: 11,
						},
						{
							Name: "p22",
							Port: 22,
						},
					},
					creationTime: integration.After,
				},
				"processguid1/flask-app/1": &CloudFoundryService{
					containerIPs: map[string]string{CfServiceContainerIP: "1.2.3.5"},
					containerPorts: []ContainerPort{
						{
							Name: "p33",
							Port: 33,
						},
						{
							Name: "p44",
							Port: 44,
						},
					},
					creationTime: integration.After,
				},
				"processguid1/my-postgres": &CloudFoundryService{
					containerIPs:   map[string]string{},
					containerPorts: []ContainerPort{},
					creationTime:   integration.After,
				},
				"processguid2/flask-app/0": &CloudFoundryService{
					containerIPs: map[string]string{CfServiceContainerIP: "1.2.3.7"},
					containerPorts: []ContainerPort{
						{
							Name: "p77",
							Port: 77,
						},
						{
							Name: "p88",
							Port: 88,
						},
					},
					creationTime: integration.After,
				},
				"processguid2/flask-app/1": &CloudFoundryService{
					containerIPs: map[string]string{CfServiceContainerIP: "1.2.3.8"},
					containerPorts: []ContainerPort{
						{
							Name: "p99",
							Port: 99,
						},
						{
							Name: "p111",
							Port: 111,
						},
					},
					creationTime: integration.After,
				},
				"processguid2/my-postgres": &CloudFoundryService{
					containerIPs:   map[string]string{},
					containerPorts: []ContainerPort{},
					creationTime:   integration.After,
				},
			},
		},
	} {
		// NOTE: we don't use t.Run here, since the executions are chained (every test case is expected to delete some
		// services created by the previous test case), so once something is wrong, we just fail the whole test case
		testBBSCache.Lock()
		testBBSCache.ActualLRPs = tc.aLRP
		testBBSCache.DesiredLRPs = tc.dLRP
		testBBSCache.Unlock()

		time.Sleep(20 * time.Millisecond)
		// we have to fail now, otherwise we might get blocked trying to read from the channel
		if !assert.Equal(t, len(tc.expNew), len(newSvc)) {
			t.FailNow()
		}
		if !assert.Equal(t, len(tc.expDel), len(delSvc)) {
			t.FailNow()
		}
		for range tc.expNew {
			s := (<-newSvc).(*CloudFoundryService)
			adID, err := s.GetADIdentifiers()
			assert.Nil(t, err)
			// we make the comparison easy by leaving out the ADIdentifier structs out
			oldID := s.adIdentifier
			s.adIdentifier = cloudfoundry.ADIdentifier{}
			assert.Equal(t, tc.expNew[adID[0]], s)
			s.adIdentifier = oldID
		}
		for range tc.expDel {
			s := (<-delSvc).(*CloudFoundryService)
			adID, err := s.GetADIdentifiers()
			assert.Nil(t, err)
			s.adIdentifier = cloudfoundry.ADIdentifier{}
			assert.Equal(t, tc.expDel[adID[0]], s)
		}
	}
}
