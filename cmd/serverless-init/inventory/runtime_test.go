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
		{name: "override beats Python launcher", override: "MyPython", metadata: []string{"Java"}, command: []string{"ddtrace-run", "python", "app.py"}, want: "MyPython"},
		{name: "metadata beats Ruby launcher", metadata: []string{"MyRuby"}, command: []string{"bundle", "exec", "ruby", "app.rb"}, want: "MyRuby"},
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

func TestResolveRuntimeLauncher(t *testing.T) {
	t.Setenv("DD_SERVERLESS_INVENTORY_RUNTIME", "")
	for _, tt := range []struct {
		name    string
		command []string
		want    string
	}{
		{name: "Python launcher", command: []string{"ddtrace-run", "python", "app.py"}, want: "Python"},
		{name: "Python launcher paths", command: []string{"/opt/venv/bin/ddtrace-run", "/opt/venv/bin/python3.12", "app.py"}, want: "Python"},
		{name: "Python version suffix", command: []string{"ddtrace-run", "python3", "-m", "app"}, want: "Python"},
		{name: "Ruby launcher", command: []string{"bundle", "exec", "ruby", "app.rb"}, want: "Ruby"},
		{name: "Ruby launcher paths", command: []string{"/usr/local/bin/bundle", "exec", "/usr/bin/ruby", "app.rb"}, want: "Ruby"},
		{name: "Python gunicorn entry point", command: []string{"ddtrace-run", "/opt/venv/bin/gunicorn", "app:app"}, want: "Python"},
		{name: "Python uvicorn entry point", command: []string{"ddtrace-run", "uvicorn", "app:app"}, want: "Python"},
		{name: "Python Flask entry point", command: []string{"ddtrace-run", "flask", "run"}, want: "Python"},
		{name: "Python Celery entry point", command: []string{"ddtrace-run", "celery", "-A", "app", "worker"}, want: "Python"},
		{name: "Ruby Rails entry point", command: []string{"bundle", "exec", "./bin/rails", "server"}, want: "Ruby"},
		{name: "Ruby Rake entry point", command: []string{"bundle", "exec", "rake", "task"}, want: "Ruby"},
		{name: "Ruby Puma entry point", command: []string{"bundle", "exec", "puma", "-C", "config/puma.rb"}, want: "Ruby"},
		{name: "Ruby Sidekiq entry point", command: []string{"bundle", "exec", "sidekiq", "-r", "./app.rb"}, want: "Ruby"},
		{name: "Python launcher without interpreter", command: []string{"ddtrace-run"}},
		{name: "Python launcher empty interpreter", command: []string{"ddtrace-run", ""}},
		{name: "Python launcher diagnostic only", command: []string{"ddtrace-run", "--info"}},
		{name: "Python launcher requires Python command", command: []string{"ddtrace-run", "puma"}},
		{name: "Python launcher arbitrary executable", command: []string{"ddtrace-run", "node", "app.py"}},
		{name: "Python launcher script not inferred", command: []string{"ddtrace-run", "./app.py"}},
		{name: "Python launcher command prefix not inferred", command: []string{"ddtrace-run", "gunicorn-custom", "app:app"}},
		{name: "Python launcher invalid version", command: []string{"ddtrace-run", "python3.", "app.py"}},
		{name: "Python launcher configuration executable", command: []string{"ddtrace-run", "python3.12-config"}},
		{name: "Python launcher options not parsed", command: []string{"ddtrace-run", "--debug", "python", "app.py"}},
		{name: "Ruby launcher without arguments", command: []string{"bundle"}},
		{name: "Ruby launcher without interpreter", command: []string{"bundle", "exec"}},
		{name: "Ruby launcher empty interpreter", command: []string{"bundle", "exec", ""}},
		{name: "Ruby launcher requires exec", command: []string{"bundle", "ruby", "app.rb"}},
		{name: "Ruby launcher requires Ruby command", command: []string{"bundle", "exec", "gunicorn", "app:app"}},
		{name: "Ruby launcher arbitrary executable", command: []string{"bundle", "exec", "node", "app.rb"}},
		{name: "Ruby launcher script not inferred", command: []string{"bundle", "exec", "./app.rb"}},
		{name: "Ruby launcher command prefix not inferred", command: []string{"bundle", "exec", "puma-custom"}},
		{name: "Ruby launcher options not parsed", command: []string{"bundle", "--verbose", "exec", "ruby", "app.rb"}},
		{name: "launcher shell expression", command: []string{"sh", "-c", "ddtrace-run python app.py"}},
		{name: "combined launcher string", command: []string{"bundle exec ruby app.rb"}},
		{name: "nested launchers not parsed", command: []string{"ddtrace-run", "bundle", "exec", "ruby", "app.rb"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resolveRuntime(nil, tt.command))
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
		{"/opt/venv/bin/gunicorn", "Python"},
		{"uvicorn", "Python"},
		{"flask", "Python"},
		{"celery", "Python"},
		{"/usr/bin/java", "Java"},
		{"dotnet", ".NET"},
		{"/usr/bin/php", "PHP"},
		{"ruby", "Ruby"},
		{"./bin/rails", "Ruby"},
		{"rake", "Ruby"},
		{"/usr/local/bin/puma", "Ruby"},
		{"sidekiq", "Ruby"},
		{"", ""},
		{"sh", ""},
		{"/bin/bash", ""},
		{"/usr/bin/env", ""},
		{"npm", ""},
		{"bundle", ""},
		{"ddtrace-run", ""},
		{"gunicorn-custom", ""},
		{"puma-custom", ""},
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
			command := []string{tt.executable, "-c", "node app.js"}
			assert.Equal(t, tt.want, resolveRuntime(nil, command))
			switch tt.want {
			case "Python":
				assert.Equal(t, tt.want, resolveRuntime(nil, append([]string{"ddtrace-run"}, command...)))
			case "Ruby":
				assert.Equal(t, tt.want, resolveRuntime(nil, append([]string{"bundle", "exec"}, command...)))
			}
		})
	}
}
