// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemprobe

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
)

func TestRenderTextCWSSelfTests(t *testing.T) {
	for _, test := range []struct {
		name      string
		selfTests *api.SelfTestsStatus
	}{
		{name: "disabled"},
		{name: "populated", selfTests: &api.SelfTestsStatus{LastTimestamp: "2026-09-17T12:00:00Z", Success: []string{"open"}, Fails: []string{"chmod"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			stats := map[string]any{"event_monitor": map[string]any{"cws": &api.Status{SelfTests: test.selfTests}}}
			require.NoError(t, RenderText(stats, &output))
			assert.Contains(t, output.String(), "Policies")
			if test.selfTests == nil {
				assert.NotContains(t, output.String(), "Self Tests")
			} else {
				assert.Contains(t, output.String(), "Self Tests")
				assert.Contains(t, output.String(), "Last execution: 2026-09-17T12:00:00Z")
				assert.Contains(t, output.String(), "Succeeded:\n        - open")
				assert.Contains(t, output.String(), "Failed:\n        - chmod")
			}
		})
	}
}
