// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventorysoftware

import (
	"encoding/json"
	softwareinventory "github.com/DataDog/datadog-agent/pkg/inventory/software"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestPayloadMarshalJSON(t *testing.T) {
	p := &Payload{
		Metadata: HostSoftware{
			Software: []softwareinventory.SoftwareEntry{
				{DisplayName: "FooApp", ProductCode: "foo"},
				{DisplayName: "BarApp", ProductCode: "bar"},
			},
		},
	}
	b, err := p.MarshalJSON()
	assert.NoError(t, err)
	var out map[string]interface{}
	assert.NoError(t, json.Unmarshal(b, &out))
	assert.Contains(t, string(b), "FooApp")
	assert.Contains(t, string(b), "BarApp")
}

func TestPayloadSplitPayload(t *testing.T) {
	p := &Payload{}
	res, err := p.SplitPayload(1)
	assert.Nil(t, res)
	assert.Error(t, err)
}
