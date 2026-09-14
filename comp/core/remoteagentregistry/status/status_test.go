// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package status

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"

	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
)

func findElement(node *html.Node, name string) *html.Node {
	if node.Type == html.ElementNode && node.Data == name {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findElement(child, name); found != nil {
			return found
		}
	}
	return nil
}

func findElements(node *html.Node, name string) []*html.Node {
	var elements []*html.Node
	if node.Type == html.ElementNode && node.Data == name {
		elements = append(elements, node)
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		elements = append(elements, findElements(child, name)...)
	}
	return elements
}

func textContent(node *html.Node) string {
	if node.Type == html.TextNode {
		return node.Data
	}
	var content strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		content.WriteString(textContent(child))
	}
	return content.String()
}

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
			wantContains:   "<pre><span>line one\n  line &lt;two&gt; &amp; details</span></pre>",
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

func TestHTMLRawStatusPreservesLeadingNewline(t *testing.T) {
	const rawStatus = "\nline one\nline two"
	registry := statusRegistry{
		statuses: []remoteagentregistry.StatusData{
			{
				RegisteredAgent: remoteagentregistry.RegisteredAgent{DisplayName: "Test Agent"},
				NamedSections: map[string]remoteagentregistry.StatusSection{
					"Details": {"": rawStatus},
				},
			},
		},
	}

	var output bytes.Buffer
	err := GetProvider(registry).HTML(false, &output)
	require.NoError(t, err)

	document, err := html.Parse(strings.NewReader(output.String()))
	require.NoError(t, err)
	pre := findElement(document, "pre")
	require.NotNil(t, pre)
	assert.Equal(t, rawStatus, textContent(pre))
}

func TestHTMLRawStatusIsNotAListItem(t *testing.T) {
	registry := statusRegistry{
		statuses: []remoteagentregistry.StatusData{
			{
				RegisteredAgent: remoteagentregistry.RegisteredAgent{DisplayName: "Test Agent"},
				NamedSections: map[string]remoteagentregistry.StatusSection{
					"Details": {
						"":      "raw status",
						"State": "running",
					},
				},
			},
		},
	}

	var output bytes.Buffer
	err := GetProvider(registry).HTML(false, &output)
	require.NoError(t, err)

	document, err := html.Parse(strings.NewReader(output.String()))
	require.NoError(t, err)
	pre := findElement(document, "pre")
	require.NotNil(t, pre)
	require.NotNil(t, pre.Parent)
	assert.NotEqual(t, "ul", pre.Parent.Data)
	assert.Len(t, findElements(document, "li"), 1)
}
