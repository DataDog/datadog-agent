// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package serverlessinitimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	serverlessdef "github.com/DataDog/datadog-agent/comp/metadata/serverlessinit/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestBuildFieldsMissingValues(t *testing.T) {
	t.Setenv("DD_SERVERLESS_METADATA_RUNTIME", "")
	conf := configmock.New(t)
	conf.Set("site", "", model.SourceAgentRuntime)
	inv := serverlessdef.CloudServiceMetadata{
		ResourceID:   "test-resource",
		ResourceName: "test-app",
		WorkloadType: "azure_app_service",
	}

	fields := buildFields(inv, serverlessdef.Params{Sidecar: true}, conf)

	for _, key := range []string{
		"parent_resource_id", "region", "gcp_project_id", "aws_account_id",
		"azure_subscription_id", "azure_resource_group", "runtime",
		"dd_env", "dd_site", "dd_version", "dd_service", "wrapped_command",
	} {
		assert.Contains(t, fields, key, "missing fields must remain explicit nulls")
		assert.Nil(t, fields[key], key)
	}
	assert.Equal(t, inv.ResourceID, fields["resource_id"])
	assert.Equal(t, inv.ResourceName, fields["resource_name"])
	assert.Equal(t, inv.WorkloadType, fields["workload_type"])
	assert.Equal(t, "sidecar", fields["deployment_model"])
	assert.NotContains(t, fields, "deployment_id")
	assert.NotContains(t, fields, "tags")
}

func TestBuildFieldsPreservesPopulatedValues(t *testing.T) {
	t.Setenv("DD_SERVERLESS_METADATA_RUNTIME", "")
	conf := configmock.New(t)
	conf.Set("site", "datadoghq.eu", model.SourceAgentRuntime)
	inv := serverlessdef.CloudServiceMetadata{
		ResourceID:          "test-resource",
		ResourceName:        "test-app",
		WorkloadType:        "azure_app_service",
		ParentResourceID:    "test-parent",
		Region:              "test-region",
		GCPProjectID:        "test-project",
		AWSAccountID:        "123456789012",
		AzureSubscriptionID: "test-subscription",
		AzureResourceGroup:  "test-group",
		RuntimeCandidates:   []string{"python"},
	}

	fields := buildFields(inv, serverlessdef.Params{
		Sidecar: true,
		Tags: map[string]string{
			"env": "test-env", "service": "test-service", "version": "test-version",
		},
	}, conf)

	for key, expected := range map[string]string{
		"resource_id": "test-resource", "resource_name": "test-app", "workload_type": "azure_app_service",
		"parent_resource_id": "test-parent", "region": "test-region", "gcp_project_id": "test-project",
		"aws_account_id": "123456789012", "azure_subscription_id": "test-subscription", "azure_resource_group": "test-group",
		"runtime": "python", "dd_env": "test-env", "dd_service": "test-service", "dd_version": "test-version", "dd_site": "datadoghq.eu",
	} {
		assert.Equal(t, expected, fields[key], key)
	}
}

func TestBuildFieldsPreservesNonemptyStrings(t *testing.T) {
	conf := configmock.New(t)
	for _, value := range []string{"unknown", "null", " ", " python "} {
		t.Run(value, func(t *testing.T) {
			// Generic missing-value normalization only changes empty strings. Runtime has its
			// own semantics; unrelated fields must still preserve these values.
			inv := serverlessdef.CloudServiceMetadata{Region: value}
			fields := buildFields(inv, serverlessdef.Params{Sidecar: true}, conf)
			assert.Equal(t, value, fields["region"])
		})
	}
}

func TestBuildFieldsRuntime(t *testing.T) {
	conf := configmock.New(t)

	for _, tt := range []struct {
		name     string
		sidecar  bool
		command  []string
		override string
		metadata string
		want     interface{}
	}{
		{name: "wrapped command", command: []string{"/usr/bin/python3.12", "app.py"}, want: "Python"},
		{name: "Python launcher", command: []string{"ddtrace-run", "python", "app.py"}, want: "Python"},
		{name: "Ruby launcher", command: []string{"bundle", "exec", "ruby", "app.rb"}, want: "Ruby"},
		{name: "Python entry point", command: []string{"ddtrace-run", "gunicorn", "app:app"}, want: "Python"},
		{name: "Ruby entry point", command: []string{"bundle", "exec", "puma"}, want: "Ruby"},
		{name: "bare Python entry point", command: []string{"gunicorn", "app:app"}, want: "Python"},
		{name: "bare Ruby entry point", command: []string{"puma"}, want: "Ruby"},
		{name: "sidecar ignores bare Python entry point", sidecar: true, command: []string{"gunicorn", "app:app"}},
		{name: "sidecar ignores bare Ruby entry point", sidecar: true, command: []string{"puma"}},
		{name: "sidecar ignores Python entry point", sidecar: true, command: []string{"ddtrace-run", "gunicorn", "app:app"}},
		{name: "sidecar ignores Ruby entry point", sidecar: true, command: []string{"bundle", "exec", "puma"}},
		{name: "sidecar ignores Python launcher", sidecar: true, command: []string{"ddtrace-run", "python", "app.py"}},
		{name: "sidecar ignores Ruby launcher", sidecar: true, command: []string{"bundle", "exec", "ruby", "app.rb"}},
		{name: "incomplete Python launcher", command: []string{"ddtrace-run"}},
		{name: "incomplete Ruby launcher", command: []string{"bundle", "exec"}},
		{name: "no command"},
		{name: "own executable is not workload", command: nil},
		{name: "sidecar ignores command", sidecar: true, command: []string{"node", "app.js"}},
		{name: "sidecar metadata", sidecar: true, command: []string{"node"}, metadata: "Java", want: "Java"},
		{name: "sidecar override", sidecar: true, command: []string{"node"}, override: " MyRuntime ", metadata: "Java", want: "MyRuntime"},
		{name: "sidecar placeholder", sidecar: true, command: []string{"node"}, metadata: "Container"},
		{name: "unknown missing", metadata: "unknown"},
		{name: "null missing", metadata: " NuLl "},
		{name: "blank missing", metadata: " \t"},
		{name: "invalid override still detects", override: "null", command: []string{"ruby"}, want: "Ruby"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DD_SERVERLESS_METADATA_RUNTIME", tt.override)
			inv := serverlessdef.CloudServiceMetadata{RuntimeCandidates: []string{tt.metadata}}
			fields := buildFields(inv, serverlessdef.Params{
				Sidecar:        tt.sidecar,
				WrappedCommand: tt.command,
			}, conf)
			assert.Contains(t, fields, "runtime", "missing runtime must clear the cached value")
			assert.Equal(t, tt.want, fields["runtime"])
		})
	}
}

func TestBuildFieldsWrappedCommand(t *testing.T) {
	t.Setenv("DD_SERVERLESS_INIT_METADATA_WRAPPED_COMMAND_ENABLED", "")
	t.Setenv("DD_SERVERLESS_METADATA_RUNTIME", "")

	for _, scenario := range []struct {
		name    string
		command []string
		sidecar bool
	}{
		{name: "no command"},
		{name: "sidecar", command: []string{"python", "app.py"}, sidecar: true},
		{name: "equals password", command: []string{"python", "app.py", "--password=secret"}},
		{name: "equals token", command: []string{"python", "app.py", "--token=secret"}},
		{name: "separated password", command: []string{"python", "app.py", "--password", "secret"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			conf := configmock.New(t)
			inv := serverlessdef.CloudServiceMetadata{ResourceID: "test-resource"}
			tags := map[string]string{"service": "test-service", "env": "test-env", "version": "1.2.3"}
			var defaultFields map[string]interface{}
			for _, enabled := range []string{"unset", "false", "true"} {
				t.Run(enabled, func(t *testing.T) {
					if enabled != "unset" {
						conf.Set("serverless.metadata_wrapped_command_enabled", enabled == "true", model.SourceAgentRuntime)
					}
					fields := buildFields(inv, serverlessdef.Params{
						Sidecar:        scenario.sidecar,
						WrappedCommand: scenario.command,
						Tags:           tags,
					}, conf)
					assert.Contains(t, fields, "wrapped_command", "nil must clear a previously cached value")
					if enabled != "true" || scenario.sidecar || len(scenario.command) == 0 {
						assert.Nil(t, fields["wrapped_command"])
					} else if scenario.name == "equals password" {
						assert.Equal(t, "python app.py --password=********", fields["wrapped_command"])
					} else {
						assert.NotEmpty(t, fields["wrapped_command"], "opt-in redaction is best effort, not complete")
					}
					if !scenario.sidecar && len(scenario.command) > 0 {
						assert.Equal(t, "Python", fields["runtime"])
						assert.Equal(t, "in-container", fields["deployment_model"])
					}
					delete(fields, "wrapped_command")
					if enabled == "unset" {
						defaultFields = fields
					} else {
						assert.Equal(t, defaultFields, fields, "all other metadata must remain unchanged")
					}
				})
			}
		})
	}
}
