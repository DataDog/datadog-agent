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

func TestHTMLRawStatusFieldRendering(t *testing.T) {
	const rawStatus = "\nline <two> & details"
	registry := statusRegistry{
		statuses: []remoteagentregistry.StatusData{
			{
				RegisteredAgent: remoteagentregistry.RegisteredAgent{DisplayName: "Test Agent"},
				NamedSections: map[string]remoteagentregistry.StatusSection{
					"Details": {
						"":      rawStatus,
						"State": "running",
					},
				},
			},
		},
	}

	var output bytes.Buffer
	require.NoError(t, GetProvider(registry).HTML(false, &output))
	assert.Contains(t, output.String(), "<li> State: running</li>")
	assert.Contains(t, output.String(), "line &lt;two&gt; &amp; details")

	document, err := html.Parse(strings.NewReader(output.String()))
	require.NoError(t, err)
	pre := findElement(document, "pre")
	require.NotNil(t, pre)
	require.NotNil(t, pre.Parent)
	assert.Equal(t, "li", pre.Parent.Data)
	require.Len(t, pre.Parent.Attr, 1)
	assert.Equal(t, "style", pre.Parent.Attr[0].Key)
	assert.Equal(t, "list-style: none;", pre.Parent.Attr[0].Val)
	assert.Equal(t, rawStatus, textContent(pre))
}
