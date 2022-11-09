package traceutil

import (
	"fmt"
	"os"
	"strings"
)

const (
	aasInstanceID      = "aas.environment.instance_id"
	aasInstanceName    = "aas.environment.instance_name"
	aasOperatingSystem = "aas.environment.os"
	aasRuntime         = "aas.environment.runtime"
	aasResourceGroup   = "aas.resource.group"
	aasResourceID      = "aas.resource.id"
	aasSiteKind        = "aas.site.kind"
	aasSiteName        = "aas.site.name"
	aasSiteType        = "aas.site.type"
	aasSubscriptionID  = "aas.subscription.id"

	// this value matches the runtime value set in the Azure Windows Extension
	dotnetFramework = ".NET"
	dotnetRuntime   = "dotnet"
	nodeFramework   = "Node.js"
	nodeRuntime     = "node"
	unknown         = "unknown"

	appService = "app"
	ddRuntime  = "DD_RUNTIME"
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
	runtime := getRuntime(getenv)

	// Windows and linux environments provide the OS differently
	windowsOS := getEnvOrUnknown("WEBSITE_OS", getenv)
	linuxOS := getLinuxOrUnknown(ownerName)
	websiteOS := getOS(windowsOS, linuxOS)

	subscriptionID := parseAzureSubscriptionID(ownerName)
	resourceID := compileAzureResourceID(subscriptionID, resourceGroup, siteName)

	return map[string]string{
		aasInstanceID:      instanceID,
		aasInstanceName:    computerName,
		aasOperatingSystem: websiteOS,
		aasRuntime:         runtime,
		aasResourceGroup:   resourceGroup,
		aasResourceID:      resourceID,
		aasSiteKind:        appService,
		aasSiteName:        siteName,
		aasSiteType:        appService,
		aasSubscriptionID:  subscriptionID,
	}
}

func getEnvOrUnknown(env string, getenv func(string) string) string {
	val := getenv(env)
	if len(env) == 0 {
		val = unknown
	}
	return val
}

func getRuntime(getenv func(string) string) (rt string) {
	env := getenv(ddRuntime)
	switch env {
	case dotnetRuntime:
		rt = dotnetFramework
	case nodeRuntime:
		rt = nodeFramework
	default:
		rt = unknown
	}
	return
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

func getLinuxOrUnknown(subID string) string {
	strippedOS := strings.Split(subID, "-")
	if len(strippedOS) > 1 {
		return strings.ToLower(strippedOS[len(strippedOS)-1])
	}
	return unknown
}

func getOS(windows string, linux string) string {

	if strings.ToLower(linux) == "linux" {
		return linux
	}

	if windows != unknown && windows != "" {
		return windows
	}

	return unknown
}
