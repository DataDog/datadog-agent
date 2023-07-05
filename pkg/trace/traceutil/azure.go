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

	// this value matches the runtime value set in the Azure Windows Extension
	dotnetFramework = ".NET"
	nodeFramework   = "Node.js"
	javaFramework   = "Java"
	unknown         = "unknown"

	appService = "app"
)

var appServicesTags map[string]string

func GetAppServicesTags() map[string]string {
	if appServicesTags != nil {
		return appServicesTags
	}
	return getAppServicesTags(os.Getenv)
}

func getAppServicesTags(getenv func(string) string) map[string]string {
	siteName := getenv("WEBSITE_SITE_NAME")
	ownerName := getenv("WEBSITE_OWNER_NAME")
	resourceGroup := getenv("WEBSITE_RESOURCE_GROUP")
	instanceID := getEnvOrUnknown("WEBSITE_INSTANCE_ID", getenv)
	computerName := getEnvOrUnknown("COMPUTERNAME", getenv)
	currentRuntime := getRuntime(getenv)
	extensionVersion := getenv("DD_AAS_EXTENSION_VERSION")

	// Windows and linux environments provide the OS differently
	// We should grab it from GO's builtin runtime pkg
	websiteOS := runtime.GOOS

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
		if len(extensionVersion) > 0 {
			tags[aasExtensionVersion] = extensionVersion
		} else if hasEnv("DD_AAS_JAVA_EXTENSION_VERSION", getenv) {
			tags[aasExtensionVersion] = getenv("DD_AAS_JAVA_EXTENSION_VERSION")
		} else if hasEnv("DD_AAS_DOTNET_EXTENSION_VERSION", getenv) {
			tags[aasExtensionVersion] = getenv("DD_AAS_DOTNET_EXTENSION_VERSION")
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

func getRuntime(getenv func(string) string) (rt string) {
	rt = unknown

	env := getenv("WEBSITE_STACK")
	if env == "JAVA" {
		rt = javaFramework
	} else if env == "NODE" || hasEnv("WEBSITE_NODE_DEFAULT_VERSION", getenv) {
		rt = nodeFramework
	} else if hasEnv("DOTNET_CLI_TELEMETRY_PROFILE", getenv) {
		rt = dotnetFramework
	}

	return rt
}

func hasEnv(env string, getenv func(string) string) bool {
	return len(getenv(env)) != 0
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
