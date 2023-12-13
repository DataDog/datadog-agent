// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package listeners

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/cloudfoundry"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type bbsCacheFake struct {
	sync.RWMutex
	Updated                 time.Time
	ActualLRPs              map[string][]*cloudfoundry.ActualLRP
	ActualLRPByInstanceGUID map[string]*cloudfoundry.ActualLRP
	DesiredLRPs             map[string]*cloudfoundry.DesiredLRP
	tagsByCellID            map[string]map[string][]string
}

func (b *bbsCacheFake) LastUpdated() time.Time {
	b.RLock()
	defer b.RUnlock()
	return b.Updated
}

func (b *bbsCacheFake) UpdatedOnce() <-chan struct{} {
	panic("implement me")
}

func (b *bbsCacheFake) GetActualLRPsForProcessGUID(processGUID string) ([]*cloudfoundry.ActualLRP, error) {
	panic("implement me")
}

func (b *bbsCacheFake) GetActualLRPsForCell(cellID string) ([]*cloudfoundry.ActualLRP, error) {
	panic("implement me")
}

func (b *bbsCacheFake) GetDesiredLRPFor(processGUID string) (cloudfoundry.DesiredLRP, error) {
	panic("implement me")
}

func (b *bbsCacheFake) GetAllLRPs() (map[string][]*cloudfoundry.ActualLRP, map[string]*cloudfoundry.DesiredLRP) {
	b.RLock()
	defer b.RUnlock()
	return b.ActualLRPs, b.DesiredLRPs
}

func (b *bbsCacheFake) GetTagsForNode(nodename string) (map[string][]string, error) {
	b.RLock()
	defer b.RUnlock()
	return b.tagsByCellID[nodename], nil
}

var testBBSCache = &bbsCacheFake{}

func TestCloudFoundryListener(t *testing.T) {
	var lastRefreshCount int64
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
		aLRP         map[string][]*cloudfoundry.ActualLRP
		dLRP         map[string]*cloudfoundry.DesiredLRP
		tagsByCellID map[string]map[string][]string
		expNew       map[string]Service
		expDel       map[string]Service
	}{
		{
			// inputs with no AD_DATADOGHQ_COM set up => no services
			aLRP: map[string][]*cloudfoundry.ActualLRP{
				"processguid1": {{ProcessGUID: "processguid1", CellID: "cellX", InstanceGUID: "instance-guid-1-0"}, {ProcessGUID: "processguid1", CellID: "cellY", InstanceGUID: "instance-guid-1-1"}},
			},
			dLRP: map[string]*cloudfoundry.DesiredLRP{
				"processguid1": {AppGUID: "appguid1", ProcessGUID: "processguid1"},
			},
			expNew: map[string]Service{},
			expDel: map[string]Service{},
		},
		{
			// inputs with AD_DATADOGHQ_COM containing config only for containers, but no containers of the app exist
			aLRP: map[string][]*cloudfoundry.ActualLRP{
				"processguid1": {{ProcessGUID: "processguid1", CellID: "cellX", InstanceGUID: "instance-guid-1-0"}, {ProcessGUID: "processguid1", CellID: "cellY", InstanceGUID: "instance-guid-1-1"}},
			},
			dLRP: map[string]*cloudfoundry.DesiredLRP{
				"differentappguid": {
					AppGUID:     "differentappguid",
					ProcessGUID: "differentprocessguid",
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
			aLRP: map[string][]*cloudfoundry.ActualLRP{
				"processguid1": {
					{
						ProcessGUID:  "processguid1",
						CellID:       "cellX",
						InstanceGUID: "instance-guid-1-0",
						ContainerIP:  "1.2.3.4",
						Ports:        []uint32{11, 22},
						State:        cloudfoundry.ActualLrpStateRunning,
					},
				},
				"differentprocessguid1": {
					{
						ProcessGUID:  "differentprocessguid1",
						CellID:       "cellY",
						ContainerIP:  "1.2.3.5",
						InstanceGUID: "different-instance-guid-1-0",
						Ports:        []uint32{33, 44},
						State:        cloudfoundry.ActualLrpStateRunning,
					},
				},
			},
			dLRP: map[string]*cloudfoundry.DesiredLRP{
				"processguid1": {
					AppGUID:     "appguid1",
					ProcessGUID: "processguid1",
					EnvAD: cloudfoundry.ADConfig{"flask-app": map[string]json.RawMessage{
						"check_names":  json.RawMessage(`["http_check"]`),
						"init_configs": json.RawMessage(`[{}]`),
						"instances":    json.RawMessage(`[{"name": "My Nginx", "url": "http://%%host%%:%%port_p8080%%", "timeout": 1}]`),
					}},
				},
			},
			tagsByCellID: map[string]map[string][]string{"cellX": {"instance-guid-1-0": {"tag:x"}}, "cellY": {"different-instance-guid-1-0": {"tag:y"}}},
			expNew: map[string]Service{
				"processguid1/flask-app/instance-guid-1-0": &CloudFoundryService{
					containerIPs:   map[string]string{CfServiceContainerIP: "1.2.3.4"},
					containerPorts: []ContainerPort{{Port: 11, Name: "p11"}, {Port: 22, Name: "p22"}},
					tags:           []string{"tag:x"},
				},
			},
			expDel: map[string]Service{},
		},
		{
			// now the service created above should be deleted
			aLRP:   map[string][]*cloudfoundry.ActualLRP{},
			dLRP:   map[string]*cloudfoundry.DesiredLRP{},
			expNew: map[string]Service{},
			expDel: map[string]Service{
				"processguid1/flask-app/instance-guid-1-0": &CloudFoundryService{
					containerIPs:   map[string]string{CfServiceContainerIP: "1.2.3.4"},
					containerPorts: []ContainerPort{{Port: 11, Name: "p11"}, {Port: 22, Name: "p22"}},
					tags:           []string{"tag:x"},
				},
			},
		},
		{
			// inputs with AD_DATADOGHQ_COM containing config only for non-containers, no container exists for the app
			aLRP: map[string][]*cloudfoundry.ActualLRP{
				"differentprocessguid1": {{ProcessGUID: "differentprocessguid1", CellID: "cellX", InstanceGUID: "different-instance-guid-1-0"}},
			},
			dLRP: map[string]*cloudfoundry.DesiredLRP{
				"myprocessguid1": {
					AppGUID:     "myappguid1",
					AppName:     "myappname1",
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
			tagsByCellID: map[string]map[string][]string{"cellX": {"different-instance-guid-1-0": {"tag:x"}}},
			expNew: map[string]Service{
				"myappguid1/my-postgres": &CloudFoundryService{
					containerIPs:   map[string]string{},
					containerPorts: []ContainerPort{},
					tags:           []string{"app_guid:myappguid1", "app_id:myappguid1", "app_name:myappname1"},
				},
			},
			expDel: map[string]Service{},
		},
		{
			// complex test with three apps, one having no AD configuration, two having different configurations
			// for both container and non-container services (plus also a non-running container)
			aLRP: map[string][]*cloudfoundry.ActualLRP{
				"processguid1": {
					{
						ProcessGUID:  "processguid1",
						InstanceGUID: "instance-guid-1-0",
						CellID:       "cellX",
						ContainerIP:  "1.2.3.4",
						Ports:        []uint32{11, 22},
						State:        cloudfoundry.ActualLrpStateRunning,
					},
					{
						ProcessGUID:  "processguid1",
						InstanceGUID: "instance-guid-1-1",
						CellID:       "cellY",
						ContainerIP:  "1.2.3.5",
						Ports:        []uint32{33, 44},
						State:        cloudfoundry.ActualLrpStateRunning,
					},
					{
						ProcessGUID:  "processguid1",
						InstanceGUID: "instance-guid-1-2",
						CellID:       "cellZ",
						ContainerIP:  "1.2.3.6",
						Ports:        []uint32{55, 66},
						State:        "NOTRUNNING",
					},
				},
				"processguid2": {
					{
						ProcessGUID:  "processguid2",
						InstanceGUID: "instance-guid-2-0",
						CellID:       "cellY",
						ContainerIP:  "1.2.3.7",
						Ports:        []uint32{77, 88},
						State:        cloudfoundry.ActualLrpStateRunning,
					},
					{
						ProcessGUID:  "processguid2",
						InstanceGUID: "instance-guid-2-1",
						CellID:       "cellZ",
						ContainerIP:  "1.2.3.8",
						Ports:        []uint32{99, 111},
						State:        cloudfoundry.ActualLrpStateRunning,
					},
				},
				"processguid3": {
					{
						ProcessGUID:  "processguid3",
						InstanceGUID: "instance-guid-3-0",
						CellID:       "cellZ",
						ContainerIP:  "1.2.3.9",
						Ports:        []uint32{222, 333},
						State:        cloudfoundry.ActualLrpStateRunning,
					},
					{
						ProcessGUID:  "processguid3",
						InstanceGUID: "instance-guid-3-1",
						CellID:       "cellZ",
						ContainerIP:  "1.2.3.10",
						Index:        1,
						Ports:        []uint32{444, 555},
						State:        cloudfoundry.ActualLrpStateRunning,
					},
				},
			},
			dLRP: map[string]*cloudfoundry.DesiredLRP{
				"processguid1": {
					AppGUID:     "appguid1",
					AppName:     "appname1",
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
					EnvVcapServices:  map[string][]byte{"my-postgres": []byte(`{"credentials":{"host":"a.b.c","Username":"me","Password":"secret","database_name":"mydb"}}`)},
					OrganizationGUID: "orgguid1",
					OrganizationName: "orgname1",
					SpaceGUID:        "spaceguid1",
					SpaceName:        "spacename1",
				},
				"processguid2": {
					AppGUID:     "appguid2",
					AppName:     "appname2",
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
					EnvVcapServices:  map[string][]byte{"my-postgres": []byte(`{"credentials":{"host":"a.b.c","Username":"me","Password":"secret","database_name":"mydb"}}`)},
					OrganizationGUID: "orgguid2",
					OrganizationName: "orgname2",
					SpaceGUID:        "spaceguid2",
					SpaceName:        "spacename2",
				},
				"processguid3": {
					AppGUID:     "appguid3",
					ProcessGUID: "processguid3",
				},
			},
			tagsByCellID: map[string]map[string][]string{
				"cellX": {
					"instance-guid-1-0": {"tag:11"},
				},
				"cellY": {
					"instance-guid-1-1": {"tag:12"},
					"instance-guid-2-0": {"tag:21"},
				},
				"cellZ": {
					"instance-guid-1-2": {"tag:13"},
					"instance-guid-2-1": {"tag:22"},
					"instance-guid-3-0": {"tag:31"},
					"instance-guid-3-1": {"tag:32"},
				},
			},
			expDel: map[string]Service{
				"myappguid1/my-postgres": &CloudFoundryService{
					containerIPs:   map[string]string{},
					containerPorts: []ContainerPort{},
					tags:           []string{"app_guid:myappguid1", "app_id:myappguid1", "app_name:myappname1"},
				},
			},
			expNew: map[string]Service{
				"processguid1/flask-app/instance-guid-1-0": &CloudFoundryService{
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
					tags: []string{"tag:11"},
				},
				"processguid1/flask-app/instance-guid-1-1": &CloudFoundryService{
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
					tags: []string{"tag:12"},
				},
				"appguid1/my-postgres": &CloudFoundryService{
					containerIPs:   map[string]string{},
					containerPorts: []ContainerPort{},
					tags:           []string{"app_guid:appguid1", "app_id:appguid1", "app_name:appname1", "org_id:orgguid1", "org_name:orgname1", "space_id:spaceguid1", "space_name:spacename1"},
				},
				"processguid2/flask-app/instance-guid-2-0": &CloudFoundryService{
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
					tags: []string{"tag:21"},
				},
				"processguid2/flask-app/instance-guid-2-1": &CloudFoundryService{
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
					tags: []string{"tag:22"},
				},
				"appguid2/my-postgres": &CloudFoundryService{
					containerIPs:   map[string]string{},
					containerPorts: []ContainerPort{},
					tags:           []string{"app_guid:appguid2", "app_id:appguid2", "app_name:appname2", "org_id:orgguid2", "org_name:orgname2", "space_id:spaceguid2", "space_name:spacename2"},
				},
			},
		},
	} {
		ctx := context.Background()
		// NOTE: we don't use t.Run here, since the executions are chained (every test case is expected to delete some
		// services created by the previous test case), so once something is wrong, we just fail the whole test case
		testBBSCache.Lock()
		testBBSCache.ActualLRPs = tc.aLRP
		testBBSCache.DesiredLRPs = tc.dLRP
		testBBSCache.tagsByCellID = tc.tagsByCellID
		testBBSCache.Unlock()

		// make sure at least one refresh loop of the listener has passed *since we updated the cache*
		cfl.RLock()
		lastRefreshCount = cfl.refreshCount
		cfl.RUnlock()
		testutil.RequireTrueBeforeTimeout(t, 15*time.Millisecond, 250*time.Millisecond, func() bool {
			cfl.RLock()
			defer cfl.RUnlock()
			return cfl.refreshCount > lastRefreshCount
		})

		// we have to fail now, otherwise we might get blocked trying to read from the channel
		require.Equal(t, len(tc.expNew), len(newSvc))
		require.Equal(t, len(tc.expDel), len(delSvc))

		for range tc.expNew {
			s := (<-newSvc).(*CloudFoundryService)
			adID, err := s.GetADIdentifiers(ctx)
			assert.Nil(t, err)
			// we make the comparison easy by leaving out the ADIdentifier structs out
			oldID := s.adIdentifier
			s.adIdentifier = cloudfoundry.ADIdentifier{}
			assert.Equal(t, tc.expNew[adID[0]], s)
			s.adIdentifier = oldID
		}
		for range tc.expDel {
			s := (<-delSvc).(*CloudFoundryService)
			adID, err := s.GetADIdentifiers(ctx)
			assert.Nil(t, err)
			s.adIdentifier = cloudfoundry.ADIdentifier{}
			assert.Equal(t, tc.expDel[adID[0]], s)
		}
	}
}
