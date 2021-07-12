// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build clusterchecks

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
	Relationships: nil,
	Links:         nil,
	Metadata:      cfclient.V3Metadata{},
}

var cfApp1 = CFApp{
	Name: "name_of_app_cc",
}

var v3App2 = cfclient.V3App{
	Name:          "app2",
	State:         "running",
	Lifecycle:     cfclient.V3Lifecycle{},
	GUID:          "guid2",
	CreatedAt:     "",
	UpdatedAt:     "",
	Relationships: nil,
	Links:         nil,
	Metadata:      cfclient.V3Metadata{},
}

var cfApp2 = CFApp{
	Name: "app2",
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
	OrganizationGUID: "random_org_guid",
	OrganizationName: "name_of_the_org",
	ProcessGUID:      "0123456789012345678901234567890123456789",
	SpaceGUID:        "random_space_guid",
	SpaceName:        "name_of_the_space",
	TagsFromEnv:      nil,
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
	OrganizationGUID: "random_org_guid",
	OrganizationName: "name_of_the_org",
	ProcessGUID:      "0123456789012345678901234567890123456789",
	SpaceGUID:        "random_space_guid",
	SpaceName:        "name_of_the_space",
	TagsFromEnv: []string{
		"CUSTOM_TAG_1:TEST1",
		"CUSTOM_TAG_2:TEST2",
	},
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
				Index: 2,
			},
			expected: "4321/flask-app/2",
		},
	} {
		t.Run(fmt.Sprintf(""), func(t *testing.T) {
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

func TestCFAppFromV3App(t *testing.T) {
	result := CFAppFromV3App(&v3App1)
	assert.EqualValues(t, cfApp1, *result)
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
