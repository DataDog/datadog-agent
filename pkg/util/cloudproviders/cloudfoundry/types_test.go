// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && !windows

package cloudfoundry

import (
	"fmt"
	"regexp"
	"testing"

	"code.cloudfoundry.org/bbs/models"
	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/stretchr/testify/assert"
)

var v3App1 = cfclient.V3App{
	Name:          "name_of_app_cc",
	State:         "running",
	Lifecycle:     cfclient.V3Lifecycle{},
	GUID:          "random_app_guid",
	CreatedAt:     "",
	UpdatedAt:     "",
	Relationships: map[string]cfclient.V3ToOneRelationship{"space": {Data: cfclient.V3Relationship{GUID: "space_guid_1"}}},
	Links:         nil,
	Metadata: cfclient.V3Metadata{
		Labels:      map[string]string{"tags.datadoghq.com/env": "test-env", "toto": "tata"},
		Annotations: map[string]string{"tags.datadoghq.com/service": "test-service", "foo": "bar"},
	},
}

var v3App2 = cfclient.V3App{
	Name:          "app2",
	State:         "running",
	Lifecycle:     cfclient.V3Lifecycle{},
	GUID:          "guid2",
	CreatedAt:     "",
	UpdatedAt:     "",
	Relationships: map[string]cfclient.V3ToOneRelationship{"space": {Data: cfclient.V3Relationship{GUID: "space_guid_2"}}},
	Links:         nil,
	Metadata: cfclient.V3Metadata{
		Labels:      map[string]string{},
		Annotations: map[string]string{},
	},
}

var v3Space1 = cfclient.V3Space{
	Name:          "space_name_1",
	GUID:          "space_guid_1",
	Relationships: map[string]cfclient.V3ToOneRelationship{"organization": {Data: cfclient.V3Relationship{GUID: "org_guid_1"}}},
}

var v3Space2 = cfclient.V3Space{
	Name:          "space_name_2",
	GUID:          "space_guid_2",
	Relationships: map[string]cfclient.V3ToOneRelationship{"organization": {Data: cfclient.V3Relationship{GUID: "org_guid_2"}}},
}

var v3Org1 = cfclient.V3Organization{
	Name: "org_name_1",
	GUID: "org_guid_1",
}

var v3Org2 = cfclient.V3Organization{
	Name: "org_name_2",
	GUID: "org_guid_2",
}

var cfOrgQuota1 = cfclient.OrgQuota{
	Guid: "org_quota_guid_1",
	Name: "org_quota_name_1",
}

var cfOrgQuota2 = cfclient.OrgQuota{
	Guid: "org_quota_guid_2",
	Name: "org_quota_name_2",
}

var cfSidecar1 = CFSidecar{
	GUID: "sidecar_guid_1",
	Name: "sidecar_name_1",
}

var cfSidecar2 = CFSidecar{
	GUID: "sidecar_guid_2",
	Name: "sidecar_name_2",
}

var cfIsolationSegment1 = cfclient.IsolationSegment{
	GUID: "isolation_segment_guid_1",
	Name: "isolation_segment_name_1",
}

var cfIsolationSegment2 = cfclient.IsolationSegment{
	GUID: "isolation_segment_guid_2",
	Name: "isolation_segment_name_2",
}

type Links struct {
	Self  cfclient.Link `json:"self"`
	Scale cfclient.Link `json:"scale"`
	App   cfclient.Link `json:"app"`
	Space cfclient.Link `json:"space"`
	Stats cfclient.Link `json:"stats"`
}

var cfProcess1 = cfclient.Process{
	GUID: "process_guid_1",
	Type: "process_type_1",
	Links: Links{
		App: cfclient.Link{
			Href: "https://api.sys.integrations-lab.devenv.dog/v3/apps/random_app_guid",
		},
	},
}

var cfProcess2 = cfclient.Process{
	GUID: "process_guid_2",
	Type: "process_type_2",
	Links: Links{
		App: cfclient.Link{
			Href: "https://api.sys.integrations-lab.devenv.dog/v3/apps/random_app_guid",
		},
	},
}

var cfApp1 = CFApplication{
	GUID:           "random_app_guid",
	Name:           "name_of_app_cc",
	SpaceGUID:      "space_guid_1",
	SpaceName:      "space_name_1",
	OrgName:        "org_name_1",
	OrgGUID:        "org_guid_1",
	Instances:      0,
	Buildpacks:     nil,
	DiskQuota:      0,
	TotalDiskQuota: 0,
	Memory:         0,
	TotalMemory:    0,
	Labels: map[string]string{
		"tags.datadoghq.com/env": "test-env",
		"toto":                   "tata",
	},
	Annotations: map[string]string{
		"tags.datadoghq.com/service": "test-service",
		"foo":                        "bar",
	},
	Sidecars: []CFSidecar{
		cfSidecar1,
	},
}

var cfApp2 = CFApplication{
	GUID:           "guid2",
	Name:           "app2",
	SpaceGUID:      "space_guid_2",
	SpaceName:      "space_name_2",
	OrgName:        "org_name_2",
	OrgGUID:        "org_guid_2",
	Instances:      0,
	Buildpacks:     nil,
	DiskQuota:      0,
	TotalDiskQuota: 0,
	Memory:         0,
	TotalMemory:    0,
	Labels:         map[string]string{},
	Annotations:    map[string]string{},
	Sidecars: []CFSidecar{
		cfSidecar2,
	},
}

var BBSModelA1 = models.ActualLRP{
	ActualLRPNetInfo: models.ActualLRPNetInfo{
		InstanceAddress: "1.2.3.4",
		Ports: []*models.PortMapping{
			{ContainerPort: 1234},
			{ContainerPort: 5678},
		},
	},
	ActualLRPKey: models.ActualLRPKey{
		Index:       4,
		ProcessGuid: "0123456789012345678901234567890123456789",
	},
	ActualLRPInstanceKey: models.ActualLRPInstanceKey{
		CellId:       "cell123",
		InstanceGuid: "0123456789012345678",
	},
	State: "STATE",
}

var ExpectedA1 = ActualLRP{
	CellID:       "cell123",
	ContainerIP:  "1.2.3.4",
	Index:        4,
	Ports:        []uint32{1234, 5678},
	ProcessGUID:  "0123456789012345678901234567890123456789",
	InstanceGUID: "0123456789012345678",
	State:        "STATE",
}

var BBSModelA2 = models.ActualLRP{
	ActualLRPNetInfo: models.ActualLRPNetInfo{
		InstanceAddress: "1.2.3.5",
		Ports: []*models.PortMapping{
			{ContainerPort: 1234},
			{ContainerPort: 5678},
		},
	},
	ActualLRPKey: models.ActualLRPKey{
		Index:       3,
		ProcessGuid: "0123456789012345678901234567890123456789",
	},
	ActualLRPInstanceKey: models.ActualLRPInstanceKey{
		CellId:       "cell1234",
		InstanceGuid: "0123456789012345679",
	},
	State: "RUNNING",
}

var ExpectedA2 = ActualLRP{
	CellID:       "cell1234",
	ContainerIP:  "1.2.3.5",
	Index:        3,
	Ports:        []uint32{1234, 5678},
	ProcessGUID:  "0123456789012345678901234567890123456789",
	InstanceGUID: "0123456789012345679",
	State:        "RUNNING",
}

var BBSModelD1 = models.DesiredLRP{
	ProcessGuid: "0123456789012345678901234567890123456789",
	Action: &models.Action{
		CodependentAction: &models.CodependentAction{
			Actions: []*models.Action{
				{
					RunAction: &models.RunAction{
						// this run action will be ignored, as it only has VCAP_SERVICES
						Env: []*models.EnvironmentVariable{
							{
								Name:  "VCAP_SERVICES",
								Value: "{\"broker\": [{\"name\": \"yyy\"}]}",
							},
							{
								Name:  "CUSTOM_TAG_1",
								Value: "TEST1",
							},
							{
								Name:  "CUSTOM_TAG_2",
								Value: "TEST2",
							},
						},
					},
				},
				{
					RunAction: &models.RunAction{
						Env: []*models.EnvironmentVariable{
							{
								Name:  "we ignore this",
								Value: "some value",
							},
							{
								Name:  "AD_DATADOGHQ_COM",
								Value: "{\"xxx\": {}}", // make this a valid JSON
							},
							{
								Name:  "VCAP_SERVICES",
								Value: "{\"broker\": [{\"name\": \"xxx\"}]}",
							},
							{
								Name:  "VCAP_APPLICATION",
								Value: "{\"application_name\": \"name_of_the_app\", \"application_id\": \"random_app_guid\", \"space_name\": \"name_of_the_space\", \"space_id\": \"random_space_guid\", \"organization_name\": \"name_of_the_org\", \"organization_id\": \"random_org_guid\"}",
							},
						},
					},
				},
			},
		},
	},
}

var ExpectedD1 = DesiredLRP{
	AppGUID:         "random_app_guid",
	AppName:         "name_of_app_cc",
	EnvAD:           ADConfig{"xxx": {}},
	EnvVcapServices: map[string][]byte{"xxx": []byte("{\"name\":\"xxx\"}")},
	EnvVcapApplication: map[string]string{
		"application_name":  "name_of_the_app",
		"application_id":    "random_app_guid",
		"organization_name": "name_of_the_org",
		"organization_id":   "random_org_guid",
		"space_name":        "name_of_the_space",
		"space_id":          "random_space_guid",
	},
	OrganizationGUID: "org_guid_1",
	OrganizationName: "org_name_1",
	ProcessGUID:      "0123456789012345678901234567890123456789",
	SpaceGUID:        "space_guid_1",
	SpaceName:        "space_name_1",
	CustomTags:       []string{"env:test-env", "service:test-service", "sidecar_present:true", "sidecar_count:1", "segment_id:isolation_segment_guid_1", "segment_name:isolation_segment_name_1"},
}

var ExpectedD2 = DesiredLRP{
	AppGUID:         "random_app_guid",
	AppName:         "name_of_app_cc",
	EnvAD:           ADConfig{"xxx": {}},
	EnvVcapServices: map[string][]byte{"xxx": []byte("{\"name\":\"xxx\"}")},
	EnvVcapApplication: map[string]string{
		"application_name":  "name_of_the_app",
		"application_id":    "random_app_guid",
		"organization_name": "name_of_the_org",
		"organization_id":   "random_org_guid",
		"space_name":        "name_of_the_space",
		"space_id":          "random_space_guid",
	},
	OrganizationGUID: "org_guid_1",
	OrganizationName: "org_name_1",
	ProcessGUID:      "0123456789012345678901234567890123456789",
	SpaceGUID:        "space_guid_1",
	SpaceName:        "space_name_1",
	CustomTags: []string{
		"CUSTOM_TAG_1:TEST1",
		"CUSTOM_TAG_2:TEST2",
		"env:test-env",
		"service:test-service",
		"sidecar_present:true",
		"sidecar_count:1",
		"segment_id:isolation_segment_guid_1",
		"segment_name:isolation_segment_name_1",
	},
}

var ExpectedD3NoCCCache = DesiredLRP{
	AppGUID:         "random_app_guid",
	AppName:         "name_of_the_app",
	EnvAD:           ADConfig{"xxx": {}},
	EnvVcapServices: map[string][]byte{"xxx": []byte("{\"name\":\"xxx\"}")},
	EnvVcapApplication: map[string]string{
		"application_name":  "name_of_the_app",
		"application_id":    "random_app_guid",
		"organization_name": "name_of_the_org",
		"organization_id":   "random_org_guid",
		"space_name":        "name_of_the_space",
		"space_id":          "random_space_guid",
	},
	OrganizationGUID: "random_org_guid",
	OrganizationName: "name_of_the_org",
	ProcessGUID:      "0123456789012345678901234567890123456789",
	SpaceGUID:        "random_space_guid",
	SpaceName:        "name_of_the_space",
}

func TestADIdentifier(t *testing.T) {
	for _, tc := range []struct {
		dLRP     DesiredLRP
		svcName  string
		aLRP     *ActualLRP
		expected string
	}{
		{
			dLRP: DesiredLRP{
				AppGUID:     "1234",
				ProcessGUID: "4321",
			},
			svcName:  "postgres",
			aLRP:     nil,
			expected: "1234/postgres",
		},
		{
			dLRP: DesiredLRP{
				AppGUID:     "1234",
				ProcessGUID: "4321",
			},
			svcName: "flask-app",
			aLRP: &ActualLRP{
				InstanceGUID: "instance-guid",
			},
			expected: "4321/flask-app/instance-guid",
		},
	} {
		t.Run(fmt.Sprintf("svcName=%s", tc.svcName), func(t *testing.T) {
			var i ADIdentifier
			if tc.aLRP == nil {
				i = NewADNonContainerIdentifier(tc.dLRP, tc.svcName)
			} else {
				i = NewADContainerIdentifier(tc.dLRP, tc.svcName, *tc.aLRP)
			}
			assert.EqualValues(t, tc.expected, i.String())
		})
	}
}

func TestActualLRPFromBBSModel(t *testing.T) {
	result := ActualLRPFromBBSModel(&BBSModelA1)
	assert.EqualValues(t, ExpectedA1, result)
}

func TestDesiredLRPFromBBSModel(t *testing.T) {
	includeList := []*regexp.Regexp{regexp.MustCompile("CUSTOM_*")}
	excludeList := []*regexp.Regexp{regexp.MustCompile("NOT_CUSTOM_*")}
	result := DesiredLRPFromBBSModel(&BBSModelD1, includeList, excludeList)
	assert.EqualValues(t, ExpectedD2, result)

	includeList = []*regexp.Regexp{}
	excludeList = []*regexp.Regexp{}
	result = DesiredLRPFromBBSModel(&BBSModelD1, includeList, excludeList)
	assert.EqualValues(t, ExpectedD1, result)

	// Temporarily disable global CC cache and acquire lock to prevent any refresh of the BBS cache in the background
	globalBBSCache.Lock()
	defer globalBBSCache.Unlock()
	globalCCCache.configured = false
	result = DesiredLRPFromBBSModel(&BBSModelD1, includeList, excludeList)
	globalCCCache.configured = true
	assert.EqualValues(t, ExpectedD3NoCCCache, result)
}

func TestGetVcapServicesMap(t *testing.T) {
	input := `{
		"broker1": [{"name": "s1", "attr1": "a1"}, {"name": "s2", "attr2": "a2"}],
		"broker2": [{"name": "s3", "attr3": "a3"}]
	}`
	expected := map[string][]byte{
		"s1": []byte(`{"attr1":"a1","name":"s1"}`),
		"s2": []byte(`{"attr2":"a2","name":"s2"}`),
		"s3": []byte(`{"attr3":"a3","name":"s3"}`),
	}
	result, err := getVcapServicesMap(input, "xxx")
	assert.Nil(t, err)
	assert.EqualValues(t, expected, result)
}

func TestIsAllowedTag(t *testing.T) {
	// when both empty, exclude everything
	includeList := []*regexp.Regexp{}
	excludeList := []*regexp.Regexp{}

	result := isAllowedTag("aRandomValue", includeList, excludeList)
	assert.EqualValues(t, false, result)

	// include strings in the includeList and not in the excludeList
	includeList = []*regexp.Regexp{regexp.MustCompile("include.*")}
	excludeList = []*regexp.Regexp{}

	result = isAllowedTag("includeTag", includeList, excludeList)
	assert.EqualValues(t, true, result)

	result = isAllowedTag("excludeTag", includeList, excludeList)
	assert.EqualValues(t, false, result)

	// reject strings in the excludeList
	includeList = []*regexp.Regexp{}
	excludeList = []*regexp.Regexp{regexp.MustCompile("exclude.*")}

	result = isAllowedTag("aRandomValue", includeList, excludeList)
	assert.EqualValues(t, true, result)

	result = isAllowedTag("excludeTag", includeList, excludeList)
	assert.EqualValues(t, false, result)

	// reject strings in the excludeList even if they exist in the includeList
	includeList = []*regexp.Regexp{regexp.MustCompile("include.*")}
	excludeList = []*regexp.Regexp{regexp.MustCompile("includeExclude.*")}

	result = isAllowedTag("includeTag", includeList, excludeList)
	assert.EqualValues(t, true, result)

	result = isAllowedTag("includeExcludeTag", includeList, excludeList)
	assert.EqualValues(t, false, result)
}
