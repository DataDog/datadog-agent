// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build clusterchecks

package providers

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/cloudfoundry"
	"github.com/stretchr/testify/assert"
)

type bbsCacheFake struct {
	Updated     time.Time
	ActualLRPs  map[string][]cloudfoundry.ActualLRP
	DesiredLRPs map[string]cloudfoundry.DesiredLRP
}

func (b bbsCacheFake) LastUpdated() time.Time {
	return b.Updated
}

func (b bbsCacheFake) GetPollAttempts() int {
	panic("implement me")
}

func (b bbsCacheFake) GetPollSuccesses() int {
	panic("implement me")
}

func (b bbsCacheFake) GetActualLRPFor(instanceGUID string) cloudfoundry.ActualLRP {
	panic("implement me")
}

func (b bbsCacheFake) GetActualLRPsFor(appGUID string) []cloudfoundry.ActualLRP {
	panic("implement me")
}

func (b bbsCacheFake) GetDesiredLRPFor(appGUID string) cloudfoundry.DesiredLRP {
	panic("implement me")
}

func (b bbsCacheFake) GetAllLRPs() (map[string][]cloudfoundry.ActualLRP, map[string]cloudfoundry.DesiredLRP) {
	return b.ActualLRPs, b.DesiredLRPs
}

func (b bbsCacheFake) ExtractTags(nodename string) map[string][]string {
	panic("implement me")
}

var testBBSCache = &bbsCacheFake{}

func TestCloudFoundryConfigProvider_IsUpToDate(t *testing.T) {
	now := time.Now()
	then := now.Add(time.Duration(1))
	testBBSCache.Updated = now

	p := CloudFoundryConfigProvider{bbsCache: testBBSCache, lastCollected: then}
	upToDate, err := p.IsUpToDate()
	assert.Nil(t, err)
	assert.EqualValues(t, true, upToDate)

	testBBSCache.Updated = then
	p.lastCollected = now
	upToDate, err = p.IsUpToDate()
	assert.Nil(t, err)
	assert.EqualValues(t, false, upToDate)
}

func TestCloudFoundryConfigProvider_String(t *testing.T) {
	p := CloudFoundryConfigProvider{}
	assert.EqualValues(t, "cloudfoundry-bbs", p.String())
}

func TestCloudFoundryConfigProvider_Collect(t *testing.T) {
	for _, tc := range []struct {
		tc       string
		aLRP     map[string][]cloudfoundry.ActualLRP
		dLRP     map[string]cloudfoundry.DesiredLRP
		expected []integration.Config
	}{
		{
			// empty inputs => no configs
			tc:       "empty_inputs",
			aLRP:     map[string][]cloudfoundry.ActualLRP{},
			dLRP:     map[string]cloudfoundry.DesiredLRP{},
			expected: []integration.Config{},
		},
		{
			// inputs with no AD_DATADOGHQ_COM set up => no configs
			tc: "no_ad_config",
			aLRP: map[string][]cloudfoundry.ActualLRP{
				"appguid1": {{AppGUID: "appguid1", CellID: "cellX", Index: 0}, {AppGUID: "appguid1", CellID: "cellY", Index: 1}},
			},
			dLRP: map[string]cloudfoundry.DesiredLRP{
				"appguid1": {AppGUID: "appguid1", ProcessGUID: "processguid1"},
			},
			expected: []integration.Config{},
		},
		{
			// inputs with AD_DATADOGHQ_COM containing config only for containers, but no containers of the app exist
			tc: "ad_config_present_but_no_containers_running",
			aLRP: map[string][]cloudfoundry.ActualLRP{
				"appguid1": {{AppGUID: "appguid1", CellID: "cellX", Index: 0}, {AppGUID: "appguid1", CellID: "cellY", Index: 1}},
			},
			dLRP: map[string]cloudfoundry.DesiredLRP{
				"differentappguid": {
					AppGUID:     "differentappguid",
					ProcessGUID: "processguid1",
					EnvAD: cloudfoundry.ADConfig{"flask-app": map[string]json.RawMessage{
						"check_names":  json.RawMessage(`["http_check"]`),
						"init_configs": json.RawMessage(`[{}]`),
						"instances":    json.RawMessage(`[{"name": "My Nginx", "url": "http://%%host%%:%%port_p8080%%", "timeout": 1}]`),
					}},
				},
			},
			expected: []integration.Config{},
		},
		{
			// inputs with AD_DATADOGHQ_COM containing config only for containers, 1 container exists for the app
			tc: "ad_config_present_1_container_running",
			aLRP: map[string][]cloudfoundry.ActualLRP{
				"appguid1":          {{AppGUID: "appguid1", CellID: "cellX", Index: 0}},
				"differentappguid1": {{AppGUID: "differentappguid1", CellID: "cellY", Index: 1}},
			},
			dLRP: map[string]cloudfoundry.DesiredLRP{
				"appguid1": {
					AppGUID:     "appguid1",
					ProcessGUID: "processguid1",
					EnvAD: cloudfoundry.ADConfig{"flask-app": map[string]json.RawMessage{
						"check_names":  json.RawMessage(`["http_check"]`),
						"init_configs": json.RawMessage(`[{}]`),
						"instances":    json.RawMessage(`[{"name": "My Nginx", "url": "http://%%host%%:%%port_p8080%%", "timeout": 1}]`),
					}},
				},
			},
			expected: []integration.Config{
				{
					ADIdentifiers: []string{"processguid1/flask-app/0"},
					ClusterCheck:  true,
					Entity:        "processguid1/flask-app/0",
					InitConfig:    []byte(`{}`),
					Instances:     []integration.Data{[]byte(`{"name":"My Nginx","timeout":1,"url":"http://%%host%%:%%port_p8080%%"}`)},
					Name:          "http_check",
					NodeName:      "cellX",
				},
			},
		},
		{
			// inputs with AD_DATADOGHQ_COM containing config only for containers, 2 containers exist for the app
			tc: "ad_config_present_2_containers_running",
			aLRP: map[string][]cloudfoundry.ActualLRP{
				"appguid1":          {{AppGUID: "appguid1", CellID: "cellX", Index: 0}, {AppGUID: "appguid1", CellID: "cellY", Index: 1}},
				"differentappguid1": {{AppGUID: "differentappguid1", CellID: "cellZ", Index: 1}},
			},
			dLRP: map[string]cloudfoundry.DesiredLRP{
				"appguid1": {
					AppGUID:     "appguid1",
					ProcessGUID: "processguid1",
					EnvAD: cloudfoundry.ADConfig{"flask-app": map[string]json.RawMessage{
						"check_names":  json.RawMessage(`["http_check"]`),
						"init_configs": json.RawMessage(`[{}]`),
						"instances":    json.RawMessage(`[{"name": "My Nginx", "url": "http://%%host%%:%%port_p8080%%", "timeout": 1}]`),
					}},
				},
			},
			expected: []integration.Config{
				{
					ADIdentifiers: []string{"processguid1/flask-app/0"},
					ClusterCheck:  true,
					Entity:        "processguid1/flask-app/0",
					InitConfig:    []byte(`{}`),
					Instances:     []integration.Data{[]byte(`{"name":"My Nginx","timeout":1,"url":"http://%%host%%:%%port_p8080%%"}`)},
					Name:          "http_check",
					NodeName:      "cellX",
				},
				{
					ADIdentifiers: []string{"processguid1/flask-app/1"},
					ClusterCheck:  true,
					Entity:        "processguid1/flask-app/1",
					InitConfig:    []byte(`{}`),
					Instances:     []integration.Data{[]byte(`{"name":"My Nginx","timeout":1,"url":"http://%%host%%:%%port_p8080%%"}`)},
					Name:          "http_check",
					NodeName:      "cellY",
				},
			},
		},
		{
			// inputs with AD_DATADOGHQ_COM containing config only for non-containers, no container exists for the app
			tc: "ad_config_present_for_non_container_no_container_running",
			aLRP: map[string][]cloudfoundry.ActualLRP{
				"differentappguid1": {{AppGUID: "differentappguid1", CellID: "cellX", Index: 1}},
			},
			dLRP: map[string]cloudfoundry.DesiredLRP{
				"appguid1": {
					AppGUID:     "appguid1",
					ProcessGUID: "processguid1",
					EnvAD: cloudfoundry.ADConfig{"my-postgres": map[string]json.RawMessage{
						"check_names":  json.RawMessage(`["postgres"]`),
						"init_configs": json.RawMessage(`[{}]`),
						"instances":    json.RawMessage(`[{"host": "%%host%%", "port": 5432, "username": "%%username%%", "dbname": "%%dbname%%", "password": "%%password%%"}]`),
						"variables":    json.RawMessage(`{"host":"$.credentials.host","username":"$.credentials.Username","password":"$.credentials.Password","dbname":"$.credentials.database_name"}`),
					}},
					EnvVcapServices: map[string][]byte{"my-postgres": []byte(`{"credentials":{"host":"a.b.c","Username":"me","Password":"secret","database_name":"mydb"}}`)},
				},
			},
			expected: []integration.Config{
				{
					ADIdentifiers: []string{"processguid1/my-postgres"},
					ClusterCheck:  true,
					Entity:        "processguid1/my-postgres",
					InitConfig:    []byte(`{}`),
					Instances:     []integration.Data{[]byte(`{"dbname":"mydb","host":"a.b.c","password":"secret","port":5432,"username":"me"}`)},
					Name:          "postgres",
					NodeName:      "",
				},
			},
		},
		{
			// inputs with AD_DATADOGHQ_COM containing config only for non-containers, 1 container exists for the app
			// NOTE: the only difference here is that the NodeName for the check should be the same as CellID of the container
			tc: "ad_config_present_for_non_container_1_container_running",
			aLRP: map[string][]cloudfoundry.ActualLRP{
				"appguid1": {{AppGUID: "appguid1", CellID: "cellX", Index: 1}},
			},
			dLRP: map[string]cloudfoundry.DesiredLRP{
				"appguid1": {
					AppGUID:     "appguid1",
					ProcessGUID: "processguid1",
					EnvAD: cloudfoundry.ADConfig{"my-postgres": map[string]json.RawMessage{
						"check_names":  json.RawMessage(`["postgres"]`),
						"init_configs": json.RawMessage(`[{}]`),
						"instances":    json.RawMessage(`[{"host": "%%host%%", "port": 5432, "username": "%%username%%", "dbname": "%%dbname%%", "password": "%%password%%"}]`),
						"variables":    json.RawMessage(`{"host":"$.credentials.host","username":"$.credentials.Username","password":"$.credentials.Password","dbname":"$.credentials.database_name"}`),
					}},
					EnvVcapServices: map[string][]byte{"my-postgres": []byte(`{"credentials":{"host":"a.b.c","Username":"me","Password":"secret","database_name":"mydb"}}`)},
				},
			},
			expected: []integration.Config{
				{
					ADIdentifiers: []string{"processguid1/my-postgres"},
					ClusterCheck:  true,
					Entity:        "processguid1/my-postgres",
					InitConfig:    []byte(`{}`),
					Instances:     []integration.Data{[]byte(`{"dbname":"mydb","host":"a.b.c","password":"secret","port":5432,"username":"me"}`)},
					Name:          "postgres",
					NodeName:      "cellX",
				},
			},
		},
		{
			// complex test with three apps, one having no AD configuration, two having different configurations for both container and non-container services
			tc: "complex",
			aLRP: map[string][]cloudfoundry.ActualLRP{
				"appguid1": {{AppGUID: "appguid1", CellID: "cellX", Index: 0}, {AppGUID: "appguid1", CellID: "cellY", Index: 1}},
				"appguid2": {{AppGUID: "appguid2", CellID: "cellY", Index: 0}, {AppGUID: "appguid2", CellID: "cellZ", Index: 1}},
				"appguid3": {{AppGUID: "appguid3", CellID: "cellZ", Index: 0}, {AppGUID: "appguid3", CellID: "cellZ", Index: 1}},
			},
			dLRP: map[string]cloudfoundry.DesiredLRP{
				"appguid1": {
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
				"appguid2": {
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
				"appguid3": {
					AppGUID:     "appguid3",
					ProcessGUID: "processguid3",
				},
			},
			expected: []integration.Config{
				{
					ADIdentifiers: []string{"processguid1/my-postgres"},
					ClusterCheck:  true,
					Entity:        "processguid1/my-postgres",
					InitConfig:    []byte(`{}`),
					Instances:     []integration.Data{[]byte(`{"dbname":"mydb","host":"a.b.c","password":"secret","port":5432,"username":"me"}`)},
					Name:          "postgres",
					NodeName:      "cellX",
				},
				{
					ADIdentifiers: []string{"processguid1/flask-app/0"},
					ClusterCheck:  true,
					Entity:        "processguid1/flask-app/0",
					InitConfig:    []byte(`{}`),
					Instances:     []integration.Data{[]byte(`{"name":"My Nginx","timeout":1,"url":"http://%%host%%:%%port_p8080%%"}`)},
					Name:          "http_check",
					NodeName:      "cellX",
				},
				{
					ADIdentifiers: []string{"processguid1/flask-app/1"},
					ClusterCheck:  true,
					Entity:        "processguid1/flask-app/1",
					InitConfig:    []byte(`{}`),
					Instances:     []integration.Data{[]byte(`{"name":"My Nginx","timeout":1,"url":"http://%%host%%:%%port_p8080%%"}`)},
					Name:          "http_check",
					NodeName:      "cellY",
				},
				{
					ADIdentifiers: []string{"processguid2/my-postgres"},
					ClusterCheck:  true,
					Entity:        "processguid2/my-postgres",
					InitConfig:    []byte(`{}`),
					Instances:     []integration.Data{[]byte(`{"dbname":"mydb","host":"a.b.c","password":"secret","port":5432,"username":"me"}`)},
					Name:          "postgres",
					NodeName:      "cellY",
				},
				{
					ADIdentifiers: []string{"processguid2/flask-app/0"},
					ClusterCheck:  true,
					Entity:        "processguid2/flask-app/0",
					InitConfig:    []byte(`{}`),
					Instances:     []integration.Data{[]byte(`{"name":"My Nginx","timeout":1,"url":"http://%%host%%:%%port_p8080%%"}`)},
					Name:          "http_check",
					NodeName:      "cellY",
				},
				{
					ADIdentifiers: []string{"processguid2/flask-app/1"},
					ClusterCheck:  true,
					Entity:        "processguid2/flask-app/1",
					InitConfig:    []byte(`{}`),
					Instances:     []integration.Data{[]byte(`{"name":"My Nginx","timeout":1,"url":"http://%%host%%:%%port_p8080%%"}`)},
					Name:          "http_check",
					NodeName:      "cellZ",
				},
			},
		},
	} {
		t.Run(tc.tc, func(t *testing.T) {
			p := CloudFoundryConfigProvider{bbsCache: testBBSCache}
			testBBSCache.ActualLRPs = tc.aLRP
			testBBSCache.DesiredLRPs = tc.dLRP
			result, err := p.Collect()
			assert.Nil(t, err)
			assert.Equal(t, len(tc.expected), len(result))
			for _, c := range result {
				assert.Contains(t, tc.expected, c)
			}
		})
	}
}
