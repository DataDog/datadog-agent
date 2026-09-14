// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package status

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
)

type statusRegistry struct {
	remoteagentregistry.Component
	statuses []remoteagentregistry.StatusData
}

func (r statusRegistry) GetRegisteredAgents() []remoteagentregistry.RegisteredAgent {
	return nil
}

func (r statusRegistry) GetRegisteredAgentStatuses() []remoteagentregistry.StatusData {
	return r.statuses
}

func TestHTMLStatusFieldRendering(t *testing.T) {
	tests := []struct {
		name           string
		fieldKey       string
		fieldValue     string
		wantContains   string
		wantNotContain string
	}{
		{
			name:         "keyed field",
			fieldKey:     "State",
			fieldValue:   "running",
			wantContains: "<li> State: running",
		},
		{
			name:           "raw status block",
			fieldValue:     "line one\n  line <two> & details",
			wantContains:   "<pre>line one\n  line &lt;two&gt; &amp; details</pre>",
			wantNotContain: "<li> : line one",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := statusRegistry{
				statuses: []remoteagentregistry.StatusData{
					{
						RegisteredAgent: remoteagentregistry.RegisteredAgent{DisplayName: "Test Agent"},
						NamedSections: map[string]remoteagentregistry.StatusSection{
							"Details": {test.fieldKey: test.fieldValue},
						},
					},
				},
			}

			var output bytes.Buffer
			err := GetProvider(registry).HTML(false, &output)
			require.NoError(t, err)

			assert.Contains(t, output.String(), test.wantContains)
			if test.wantNotContain != "" {
				assert.NotContains(t, output.String(), test.wantNotContain)
			}
		})
	}
}
