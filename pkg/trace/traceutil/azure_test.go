// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traceutil

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

var aasMetadata map[string]string

func TestGetAppServiceTags(t *testing.T) {
	metadata := getAppServicesTags(mockGetEnvVar)
	os := runtime.GOOS

	assert.Equal(t, "1234abcd", metadata[aasInstanceID])
	assert.Equal(t, "test-instance", metadata[aasInstanceName])
	assert.Equal(t, os, metadata[aasOperatingSystem])
	assert.Equal(t, "Node.js", metadata[aasRuntime])
	assert.Equal(t, "test-resource-group", metadata[aasResourceGroup])
	assert.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/test-resource-group/providers/microsoft.web/sites/site-name-test", metadata[aasResourceID])
	assert.Equal(t, "app", metadata[aasSiteKind])
	assert.Equal(t, "site-name-test", metadata[aasSiteName])
	assert.Equal(t, "app", metadata[aasSiteType])
	assert.Equal(t, "00000000-0000-0000-0000-000000000000", metadata[aasSubscriptionID])
}

func TestGetEnvOrUnknown(t *testing.T) {
	unknownEnvVar := getEnvOrUnknown("", mockGetEnvVar)
	assert.Equal(t, "unknown", unknownEnvVar)
}

func TestHasEnv(t *testing.T) {
	has := hasEnv("WEBSITE_STACK", mockGetEnvVar)
	notHas := hasEnv("DD_SERVICE", mockGetEnvVar)

	assert.Equal(t, true, has)
	assert.Equal(t, false, notHas)
}

func TestGetWindowsRuntime(t *testing.T) {
	dotnet := getRuntime("windows", func(s string) string {
		if s == "DOTNET_CLI_TELEMETRY_PROFILE" {
			return "AzureKudu"
		}
		return ""
	})
	java := getRuntime("windows", func(s string) string {
		if s == "WEBSITE_STACK" {
			return "JAVA"
		}
		return ""
	})
	node := getRuntime("windows", func(s string) string {
		if s == "WEBSITE_NODE_DEFAULT_VERSION" {
			return "~18"
		}
		return ""
	})
	unknown := getRuntime("windows", func(s string) string { return "" })

	assert.Equal(t, ".NET", dotnet)
	assert.Equal(t, "Java", java)
	assert.Equal(t, "Node.js", node)
	assert.Equal(t, "unknown", unknown)
}

func TestGetLinuxRuntime(t *testing.T) {
	docker := getRuntime("linux", func(s string) string { return "DOCKER" })
	compose := getRuntime("linux", func(s string) string {
		if s == "WEBSITE_STACK" {
			return ""
		}
		if s == "DOCKER_SERVER_VERSION" {
			return "19.03.15+azure"
		}
		return ""
	})
	java := getRuntime("linux", func(s string) string { return "JAVA" })
	tomcat := getRuntime("linux", func(s string) string { return "TOMCAT" })
	node := getRuntime("linux", func(s string) string { return "NODE" })
	python := getRuntime("linux", func(s string) string { return "PYTHON" })
	dotnet := getRuntime("linux", func(s string) string { return "DOTNETCORE" })
	php := getRuntime("linux", func(s string) string { return "PHP" })
	unknown := getRuntime("linux", func(s string) string { return "" })

	assert.Equal(t, "Container", docker)
	assert.Equal(t, "Container", compose)
	assert.Equal(t, "Java", java)
	assert.Equal(t, "Java", tomcat)
	assert.Equal(t, "Node.js", node)
	assert.Equal(t, "Python", python)
	assert.Equal(t, ".NET", dotnet)
	assert.Equal(t, "PHP", php)
	assert.Equal(t, "unknown", unknown)
}

func TestParseAzureSubscriptionID(t *testing.T) {
	metadata := mockAzureAppServiceMetadata()
	parsedSubID := parseAzureSubscriptionID(metadata["WEBSITE_OWNER_NAME"])
	assert.Equal(t, "00000000-0000-0000-0000-000000000000", parsedSubID)
}

func TestCompileAzureResourceID(t *testing.T) {
	subID := "00000000"
	resourceGroup := "resource"
	siteName := "site-name"

	resourceID := compileAzureResourceID(subID, resourceGroup, siteName)
	assert.Equal(t, "/subscriptions/00000000/resourcegroups/resource/providers/microsoft.web/sites/site-name", resourceID)
}

func mockAzureAppServiceMetadata() map[string]string {
	aasMetadata = make(map[string]string)
	aasMetadata["WEBSITE_SITE_NAME"] = "site-name-test"
	aasMetadata["WEBSITE_OWNER_NAME"] = "00000000-0000-0000-0000-000000000000+apm-dotnet-EastUSwebspace-Linux"
	aasMetadata["WEBSITE_RESOURCE_GROUP"] = "test-resource-group"
	aasMetadata["WEBSITE_INSTANCE_ID"] = "1234abcd"
	aasMetadata["COMPUTERNAME"] = "test-instance"
	aasMetadata["WEBSITE_STACK"] = "NODE"

	return aasMetadata
}

func mockGetEnvVar(key string) string {
	aasMetadata := mockAzureAppServiceMetadata()
	return aasMetadata[key]
}
