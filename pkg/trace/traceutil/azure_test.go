// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traceutil

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

var mockAppServiceEnv = map[string]string{
	"WEBSITE_SITE_NAME":            "site-name-test",
	"WEBSITE_OWNER_NAME":           "00000000-0000-0000-0000-000000000000+apm-dotnet-EastUSwebspace-Linux",
	"WEBSITE_RESOURCE_GROUP":       "test-resource-group",
	"WEBSITE_INSTANCE_ID":          "1234abcd",
	"COMPUTERNAME":                 "test-instance",
	"WEBSITE_STACK":                "NODE",
	"WEBSITE_NODE_DEFAULT_VERSION": "~18",
	"FUNCTIONS_EXTENSION_VERSION":  "~4",
}

func TestGetAppServiceTags(t *testing.T) {
	setEnvVars(t, mockAppServiceEnv)
	websiteOS := runtime.GOOS
	// Not in a function app
	linux := GetAppServicesTags()
	t.Setenv("FUNCTIONS_WORKER_RUNTIME", "node")
	functionApp := GetAppServicesTags()

	assert.Equal(t, "1234abcd", linux[aasInstanceID])
	assert.Equal(t, "test-instance", linux[aasInstanceName])
	assert.Equal(t, websiteOS, linux[aasOperatingSystem])
	assert.Equal(t, "Node.js", linux[aasRuntime])
	assert.Equal(t, "test-resource-group", linux[aasResourceGroup])
	assert.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/test-resource-group/providers/microsoft.web/sites/site-name-test", linux[aasResourceID])
	assert.Equal(t, "app", linux[aasSiteKind])
	assert.Equal(t, "site-name-test", linux[aasSiteName])
	assert.Equal(t, "app", linux[aasSiteType])
	assert.Equal(t, "00000000-0000-0000-0000-000000000000", linux[aasSubscriptionID])
	assert.Equal(t, "", linux[aasFunctionRuntime])
	assert.Equal(t, "~4", functionApp[aasFunctionRuntime])

}

func TestGetEnvOrUnknown(t *testing.T) {
	unknownEnvVar := getEnvOrUnknown("")
	assert.Equal(t, "unknown", unknownEnvVar)
}

func TestGetWindowsRuntime(t *testing.T) {
	// Java
	t.Setenv("WEBSITE_STACK", "JAVA")
	java := getRuntime("windows")

	// Node
	os.Unsetenv("WEBSITE_STACK")
	t.Setenv("WEBSITE_NODE_DEFAULT_VERSION", "18")
	node := getRuntime("windows")

	// .NET
	os.Unsetenv("WEBSITE_NODE_DEFAULT_VERSION")
	dotnet := getRuntime("windows")

	// Unset
	t.Setenv("WEBSITE_STACK", "")
	unknown := getRuntime("windows")

	assert.Equal(t, "Java", java)
	assert.Equal(t, "Node.js", node)
	assert.Equal(t, ".NET", dotnet)
	assert.Equal(t, ".NET", unknown)
}

func TestGetLinuxRuntime(t *testing.T) {
	var tests = []struct {
		envvar string
		name   string
		want   string
	}{
		{"WEBSITE_STACK", "DOCKER", "Container"},
		{"DOCKER_SERVER_VERSION", "19.03.15+azure", "Container"},
		{"WEBSITE_STACK", "JAVA", "Java"},
		{"WEBSITE_STACK", "TOMCAT", "Java"},
		{"WEBSITE_STACK", "NODE", "Node.js"},
		{"WEBSITE_STACK", "PYTHON", "Python"},
		{"WEBSITE_STACK", "DOTNETCORE", ".NET"},
		{"WEBSITE_STACK", "PHP", "PHP"},
		{"WEBSITE_STACK", "", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.envvar, tt.name)
			ans := getRuntime("linux")
			assert.Equal(t, tt.want, ans)
		})

	}
}
func TestParseAzureSubscriptionID(t *testing.T) {
	parsedSubID := parseAzureSubscriptionID(mockAppServiceEnv["WEBSITE_OWNER_NAME"])
	assert.Equal(t, "00000000-0000-0000-0000-000000000000", parsedSubID)
}

func TestCompileAzureResourceID(t *testing.T) {
	subID := "00000000"
	resourceGroup := "resource"
	siteName := "site-name"

	resourceID := compileAzureResourceID(subID, resourceGroup, siteName)
	assert.Equal(t, "/subscriptions/00000000/resourcegroups/resource/providers/microsoft.web/sites/site-name", resourceID)
}

func setEnvVars(t *testing.T, env map[string]string) {
	for k, v := range env {
		t.Setenv(k, v)
	}
}
