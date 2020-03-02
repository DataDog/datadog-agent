// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build clusterchecks

package cloudfoundry

import (
	"fmt"
	"testing"

	"code.cloudfoundry.org/bbs/models"
	"github.com/stretchr/testify/assert"
)

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
	AppGUID:      "012345678901234567890123456789012345",
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
	AppGUID:      "012345678901234567890123456789012345",
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
								Value: "{\"application_name\": \"name_of_the_app\"}",
							},
						},
					},
				},
			},
		},
	},
}

var ExpectedD1 = DesiredLRP{
	AppGUID:            "012345678901234567890123456789012345",
	EnvAD:              ADConfig{"xxx": {}},
	EnvVcapServices:    map[string][]byte{"xxx": []byte("{\"name\":\"xxx\"}")},
	EnvVcapApplication: map[string]interface{}{"application_name": "name_of_the_app"},
	ProcessGUID:        "0123456789012345678901234567890123456789",
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
				ProcessGUID: "1234",
			},
			svcName:  "postgres",
			aLRP:     nil,
			expected: "1234/postgres",
		},
		{
			dLRP: DesiredLRP{
				ProcessGUID: "1234",
			},
			svcName: "flask-app",
			aLRP: &ActualLRP{
				Index: 2,
			},
			expected: "1234/flask-app/2",
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

func TestActualLRPFromBBSModel(t *testing.T) {
	result := ActualLRPFromBBSModel(&BBSModelA1)
	assert.EqualValues(t, ExpectedA1, result)
}

func TestDesiredLRPFromBBSModel(t *testing.T) {
	result := DesiredLRPFromBBSModel(&BBSModelD1)
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
