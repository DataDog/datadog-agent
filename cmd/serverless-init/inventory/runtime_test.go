// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package inventory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveRuntimePrecedence(t *testing.T) {
	for _, tt := range []struct {
		name     string
		override string
		metadata []string
		command  []string
		want     string
	}{
		{name: "override wins", override: "Ruby", metadata: []string{"Java", "Python"}, command: []string{"node"}, want: "Ruby"},
		{name: "custom override trimmed with case preserved", override: " \tMyCustomRuntime\n", metadata: []string{"Python"}, want: "MyCustomRuntime"},
		{name: "override without detection", override: "Go", want: "Go"},
		{name: "empty override", metadata: []string{"Java"}, command: []string{"node"}, want: "Java"},
		{name: "blank override", override: " \t\n", command: []string{"python3.12"}, want: "Python"},
		{name: "unknown override falls back to metadata", override: " UnKnOwN ", metadata: []string{"PHP"}, want: "PHP"},
		{name: "container override falls back to command", override: " CONTAINER ", command: []string{"dotnet"}, want: ".NET"},
		{name: "null override falls back to command", override: " NuLl ", command: []string{"ruby"}, want: "Ruby"},
		{name: "invalid override without detection", override: "unknown"},
		{name: "known metadata beats command", metadata: []string{"Node.js"}, command: []string{"java"}, want: "Node.js"},
		{name: "worker metadata preserved", metadata: []string{"dotnet-isolated", "Java"}, command: []string{"node"}, want: "dotnet-isolated"},
		{name: "metadata whitespace trimmed", metadata: []string{" Python "}, want: "Python"},
		{name: "unknown metadata falls back", metadata: []string{" UnKnOwN "}, command: []string{"node"}, want: "Node.js"},
		{name: "container metadata falls back", metadata: []string{" CONTAINER "}, command: []string{"node"}, want: "Node.js"},
		{name: "null metadata falls back", metadata: []string{" NuLl "}, command: []string{"node"}, want: "Node.js"},
		{name: "blank metadata falls back", metadata: []string{" \t\n"}, command: []string{"node"}, want: "Node.js"},
		{name: "invalid candidate falls back to stack", metadata: []string{" UnKnOwN ", "Python"}, command: []string{"node"}, want: "Python"},
		{name: "all candidates normalized", metadata: []string{"", " \t", " CONTAINER ", " NuLl ", " MyWorker "}, want: "MyWorker"},
		{name: "placeholder substring preserved", metadata: []string{"ContainerRuntime"}, want: "ContainerRuntime"},
		{name: "all candidates missing", metadata: []string{"unknown", "container", "null"}},
		{name: "no evidence"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DD_SERVERLESS_INVENTORY_RUNTIME", tt.override)
			assert.Equal(t, tt.want, resolveRuntime(tt.metadata, tt.command))
		})
	}
}

func TestResolveRuntimeWrappedExecutable(t *testing.T) {
	t.Setenv("DD_SERVERLESS_INVENTORY_RUNTIME", "")
	for _, tt := range []struct {
		executable string
		want       string
	}{
		{"node", "Node.js"},
		{"/usr/local/bin/nodejs", "Node.js"},
		{"python", "Python"},
		{"/opt/venv/bin/python3", "Python"},
		{"python2.7", "Python"},
		{"/usr/bin/python3.12", "Python"},
		{"python3.12.1", "Python"},
		{"/usr/bin/java", "Java"},
		{"dotnet", ".NET"},
		{"/usr/bin/php", "PHP"},
		{"ruby", "Ruby"},
		{"", ""},
		{"sh", ""},
		{"/bin/bash", ""},
		{"/usr/bin/env", ""},
		{"npm", ""},
		{"bundle", ""},
		{"gunicorn", ""},
		{"node app.js", ""},
		{"exec python3 app.py", ""},
		{"python3.", ""},
		{"python3..12", ""},
		{"python3.12-config", ""},
		{"python-custom", ""},
		{"my-node-app", ""},
		{"/node/custom-app", ""},
		{"./app.py", ""},
		{"app.jar", ""},
		{"./my-go-app", ""},
		{"serverless-init", ""},
	} {
		t.Run(tt.executable, func(t *testing.T) {
			// Later arguments, including shell expressions, must not affect detection.
			assert.Equal(t, tt.want, resolveRuntime(nil, []string{tt.executable, "-c", "node app.js"}))
		})
	}
}
