// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traceutil

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

const (
	aasInstanceID       = "aas.environment.instance_id"
	aasInstanceName     = "aas.environment.instance_name"
	aasOperatingSystem  = "aas.environment.os"
	aasRuntime          = "aas.environment.runtime"
	aasExtensionVersion = "aas.environment.extension_version"
	aasResourceGroup    = "aas.resource.group"
	aasResourceID       = "aas.resource.id"
	aasSiteKind         = "aas.site.kind"
	aasSiteName         = "aas.site.name"
	aasSiteType         = "aas.site.type"
	aasSubscriptionID   = "aas.subscription.id"

	dotnetFramework    = ".NET"
	nodeFramework      = "Node.js"
	javaFramework      = "Java"
	pythonFramework    = "Python"
	phpFramework       = "PHP"
	goFramework        = "Go"
	containerFramework = "Container"
	unknown            = "unknown"

	appService = "app"
)

func GetAppServicesTags() map[string]string {
	return getAppServicesTags(os.Getenv) //TODO: make this function cache these values
}

func getAppServicesTags(getenv func(string) string) map[string]string {
	siteName := getenv("WEBSITE_SITE_NAME")
	ownerName := getenv("WEBSITE_OWNER_NAME")
	resourceGroup := getenv("WEBSITE_RESOURCE_GROUP")
	instanceID := getEnvOrUnknown("WEBSITE_INSTANCE_ID", getenv)
	computerName := getEnvOrUnknown("COMPUTERNAME", getenv)
	extensionVersion := getenv("DD_AAS_EXTENSION_VERSION")

	// Windows and linux environments provide the OS differently
	// We should grab it from GO's builtin runtime pkg
	websiteOS := runtime.GOOS

	currentRuntime := getRuntime(websiteOS, getenv)
	subscriptionID := parseAzureSubscriptionID(ownerName)
	resourceID := compileAzureResourceID(subscriptionID, resourceGroup, siteName)

	tags := map[string]string{
		aasInstanceID:      instanceID,
		aasInstanceName:    computerName,
		aasOperatingSystem: websiteOS,
		aasRuntime:         currentRuntime,
		aasResourceGroup:   resourceGroup,
		aasResourceID:      resourceID,
		aasSiteKind:        appService,
		aasSiteName:        siteName,
		aasSiteType:        appService,
		aasSubscriptionID:  subscriptionID,
	}

	// Remove the Java and .NET logic once non-universal extensions are deprecated
	if websiteOS == "windows" {
		if extensionVersion != "" {
			tags[aasExtensionVersion] = extensionVersion
		} else if val := getenv("DD_AAS_JAVA_EXTENSION_VERSION"); val != "" {
			tags[aasExtensionVersion] = val
		} else if val := getenv("DD_AAS_DOTNET_EXTENSION_VERSION"); val != "" {
			tags[aasExtensionVersion] = val
		}
	}

	return tags
}

func getEnvOrUnknown(env string, getenv func(string) string) string {
	val := getenv(env)
	if len(val) == 0 {
		val = unknown
	}
	return val
}

func getRuntime(websiteOS string, getenv func(string) string) (rt string) {
	switch websiteOS {
	case "windows":
		rt = getWindowsRuntime(getenv)
	case "linux", "darwin":
		rt = getLinuxRuntime(getenv)
	default:
		rt = unknown
	}

	return rt
}

func getWindowsRuntime(getenv func(string) string) (rt string) {
	if getenv("WEBSITE_STACK") == "JAVA" {
		rt = javaFramework
	} else if val := getenv("WEBSITE_NODE_DEFAULT_VERSION"); val != "" {
		rt = nodeFramework
	} else {
		// FIXME: Windows AAS only supports Java, Node, and .NET so we can infer this
		// Needs to be inferred because no other env vars give us context on the runtime
		rt = dotnetFramework
	}

	return rt
}

func getLinuxRuntime(getenv func(string) string) (rt string) {
	rt = unknown

	switch getenv("WEBSITE_STACK") {
	case "DOCKER":
		rt = containerFramework
	case "":
		if val := getenv("DOCKER_SERVER_VERSION"); val != "" {
			rt = containerFramework
		}
	case "NODE":
		rt = nodeFramework
	case "PYTHON":
		rt = pythonFramework
	case "JAVA", "TOMCAT":
		rt = javaFramework
	case "DOTNETCORE":
		rt = dotnetFramework
	case "PHP":
		rt = phpFramework
	}

	return rt
}

func parseAzureSubscriptionID(subID string) (id string) {
	parsedSubID := strings.SplitN(subID, "+", 2)
	if len(parsedSubID) > 1 {
		id = parsedSubID[0]
	}
	return
}

func compileAzureResourceID(subID, resourceGroup, siteName string) (id string) {
	if len(subID) > 0 && len(resourceGroup) > 0 && len(siteName) > 0 {
		id = fmt.Sprintf("/subscriptions/%s/resourcegroups/%s/providers/microsoft.web/sites/%s",
			subID, resourceGroup, siteName)
	}
	return
}
