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
	aasFunctionRuntime  = "aas.environment.function_runtime"
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

// GetAppServicesTags returns the env vars pulled from the Azure App Service instance.
// In some cases we will need to add extra tags for function apps.
func GetAppServicesTags() map[string]string {
	siteName := os.Getenv("WEBSITE_SITE_NAME")
	ownerName := os.Getenv("WEBSITE_OWNER_NAME")
	resourceGroup := os.Getenv("WEBSITE_RESOURCE_GROUP")
	instanceID := getEnvOrUnknown("WEBSITE_INSTANCE_ID")
	computerName := getEnvOrUnknown("COMPUTERNAME")
	extensionVersion := os.Getenv("DD_AAS_EXTENSION_VERSION")

	// Windows and linux environments provide the OS differently
	// We should grab it from GO's builtin runtime pkg
	websiteOS := runtime.GOOS

	currentRuntime := getRuntime(websiteOS)
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
		} else if val := os.Getenv("DD_AAS_JAVA_EXTENSION_VERSION"); val != "" {
			tags[aasExtensionVersion] = val
		} else if val := os.Getenv("DD_AAS_DOTNET_EXTENSION_VERSION"); val != "" {
			tags[aasExtensionVersion] = val
		}
	}

	// Function Apps require a different runtime and kind
	if rt, ok := os.LookupEnv("FUNCTIONS_WORKER_RUNTIME"); ok {
		tags[aasRuntime] = rt
		tags[aasFunctionRuntime] = os.Getenv("FUNCTIONS_EXTENSION_VERSION")
		tags[aasSiteKind] = "functionapp"
	}

	return tags
}

func getEnvOrUnknown(env string) string {
	if val, ok := os.LookupEnv(env); ok {
		return val
	}
	return unknown
}

func getRuntime(websiteOS string) (rt string) {
	switch websiteOS {
	case "windows":
		rt = getWindowsRuntime()
	case "linux", "darwin":
		rt = getLinuxRuntime()
	default:
		rt = unknown
	}

	return rt
}

func getWindowsRuntime() (rt string) {
	if os.Getenv("WEBSITE_STACK") == "JAVA" {
		rt = javaFramework
	} else if val := os.Getenv("WEBSITE_NODE_DEFAULT_VERSION"); val != "" {
		rt = nodeFramework
	} else {
		// FIXME: Windows AAS only supports Java, Node, and .NET so we can infer this
		// Needs to be inferred because no other env vars give us context on the runtime
		rt = dotnetFramework
	}

	return rt
}

func getLinuxRuntime() (rt string) {
	rt = unknown

	switch os.Getenv("WEBSITE_STACK") {
	case "DOCKER":
		rt = containerFramework
	case "":
		if val := os.Getenv("DOCKER_SERVER_VERSION"); val != "" {
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
