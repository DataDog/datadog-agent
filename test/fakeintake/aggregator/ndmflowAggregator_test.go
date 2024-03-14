// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	_ "embed"
	//JMW"sort"
	//JMW"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/stretchr/testify/assert"
)

//go:embed fixtures/ndmflow_bytes
var ndmflowData []byte

func TestNDMFlowAggregator(t *testing.T) {
	t.Run("parseNDMFlowPayload should return empty NDMFlow array on empty data", func(t *testing.T) {
		ndmflows, err := ParseNDMFlowPayload(api.Payload{Data: []byte(""), Encoding: encodingEmpty})
		assert.NoError(t, err)
		assert.Empty(t, ndmflows)
	})

	t.Run("parseNDMFlowPayload should return empty NDMFlow array on empty json object", func(t *testing.T) {
		ndmflows, err := ParseNDMFlowPayload(api.Payload{Data: []byte("{}"), Encoding: encodingJSON})
		assert.NoError(t, err)
		assert.Empty(t, ndmflows)
	})

	t.Run("parseNDMFlowPayload should return valid NDMFlows on valid payload", func(t *testing.T) {
		ndmflows, err := ParseNDMFlowPayload(api.Payload{Data: ndmflowData, Encoding: encodingGzip})
		assert.NoError(t, err)
		assert.Equal(t, 16, len(ndmflows))
		assert.Equal(t, "ndmflow", ndmflows[0].name())
		//JMWexpectedTags := []string{"singer:adele"}
		//JMWsort.Strings(expectedTags)
		//JMWgotTags := logs[0].GetTags()
		//JMWsort.Strings(gotTags)
		//JMWassert.Equal(t, expectedTags, gotTags)
	})
}
