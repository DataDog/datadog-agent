// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

package metrics

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

type DummyMetric struct {
	Type APIMetricType `json:"type"`
}

func TestAPIMetricTypeMarshal(t *testing.T) {
	for _, tc := range []struct {
		In  *DummyMetric
		Out string
	}{
		{
			&DummyMetric{Type: APIGaugeType},
			`{"type":"gauge"}`,
		},
		{
			&DummyMetric{Type: APICountType},
			`{"type":"count"}`,
		},
		{
			&DummyMetric{Type: APIRateType},
			`{"type":"rate"}`,
		},
	} {
		t.Run(fmt.Sprintf(tc.Out), func(t *testing.T) {
			out, err := json.Marshal(tc.In)
			assert.NoError(t, err)
			assert.Equal(t, tc.Out, string(out))

			back := &DummyMetric{}
			err = json.Unmarshal(out, back)
			assert.NoError(t, err)
			assert.Equal(t, tc.In, back)
		})
	}
}
