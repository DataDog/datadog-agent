package traceutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var aasMetadata map[string]string

func TestGetAppServiceTags(t *testing.T) {
	metadata := getAppServicesTags(mockGetEnvVar)

	assert.Equal(t, "1234abcd", metadata[aasInstanceID])
	assert.Equal(t, "test-instance", metadata[aasInstanceName])
	assert.Equal(t, "linux", metadata[aasOperatingSystem])
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

func TestLinuxOrUnknown(t *testing.T) {
	str := "thisisnotlinux"
	notLinux := getLinuxOrUnknown(str)
	assert.Equal(t, "unknown", notLinux)
}
func TestGetOS(t *testing.T) {
	windows95 := "unknown"
	windowsXP := "windows"
	windowsVista := ""
	mint := "linux"
	fedora := "unknown"
	ubuntu := ""

	a := getOS(windows95, ubuntu)
	b := getOS(windowsXP, ubuntu)
	c := getOS(windows95, mint)
	d := getOS(windows95, fedora)
	e := getOS(windowsVista, ubuntu)
	assert.Equal(t, "unknown", a)
	assert.Equal(t, "windows", b)
	assert.Equal(t, "linux", c)
	assert.Equal(t, "unknown", d)
	assert.Equal(t, "unknown", e)

}
func TestGetRuntime(t *testing.T) {
	dotnet := getRuntime(func(s string) string { return "dotnet" })
	node := getRuntime(func(s string) string { return "node" })
	unknown := getRuntime(func(s string) string { return "hahaha" })
	assert.Equal(t, ".NET", dotnet)
	assert.Equal(t, "Node.js", node)
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

// func TestGetLinuxOrUnknown(t *testing.T) {
// 	os := getLinuxOrUnknown()
// }

func mockAzureAppServiceMetadata() map[string]string {
	aasMetadata = make(map[string]string)
	aasMetadata["WEBSITE_SITE_NAME"] = "site-name-test"
	aasMetadata["WEBSITE_OWNER_NAME"] = "00000000-0000-0000-0000-000000000000+apm-dotnet-EastUSwebspace-Linux"
	aasMetadata["WEBSITE_RESOURCE_GROUP"] = "test-resource-group"
	aasMetadata["WEBSITE_INSTANCE_ID"] = "1234abcd"
	aasMetadata["COMPUTERNAME"] = "test-instance"
	aasMetadata["DD_RUNTIME"] = "node"

	return aasMetadata
}

func mockGetEnvVar(key string) string {
	aasMetadata := mockAzureAppServiceMetadata()
	return aasMetadata[key]
}
