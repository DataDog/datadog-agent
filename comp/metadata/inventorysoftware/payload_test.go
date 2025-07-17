// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventorysoftware

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestPayloadMarshalJSON(t *testing.T) {
	p := &Payload{
		Metadata: []*SoftwareMetadata{
			{ProductCode: "foo", Metadata: map[string]string{"DisplayName": "FooApp"}},
			{ProductCode: "bar", Metadata: map[string]string{"DisplayName": "BarApp"}},
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