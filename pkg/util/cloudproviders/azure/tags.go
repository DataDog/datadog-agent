package azure

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
)

var appServicesTags map[string]string

func GetAppServicesTags() map[string]string {
	if appServicesTags != nil {
		return appServicesTags
	}
	return getAppServicesTags(os.Getenv)
}

func getAppServicesTags(getenv func(string) string) map[string]string {
	// TODO: do not create each time this is called

	siteName := getenv("WEBSITE_SITE_NAME")
	ownerName := getenv("WEBSITE_OWNER_NAME")
	resourceGroup := getenv("WEBSITE_RESOURCE_GROUP")
	instanceID := getEnvOrUnknown("WEBSITE_INSTANCE_ID", getenv)
	computerName := getEnvOrUnknown("COMPUTERNAME", getenv)
	websiteOS := getEnvOrUnknown("WEBSITE_OS", getenv)
	runtime := getRuntime(getenv)

	subscriptionID := parseAzureSubscriptionID(ownerName)
	resourceID := compileAzureResourceID(subscriptionID, resourceGroup, siteName)

	return map[string]string{
		// TODO: app as string const
		aasInstanceID:      instanceID,
		aasInstanceName:    computerName,
		aasOperatingSystem: websiteOS,
		aasRuntime:         runtime,
		aasResourceGroup:   resourceGroup,
		aasResourceID:      resourceID,
		aasSiteKind:        "app",
		aasSiteName:        siteName,
		aasSiteType:        "app",
		aasSubscriptionID:  subscriptionID,
	}
}

func getEnvOrUnknown(env string, getenv func(string) string) string {
	val := getenv(env)
	if len(env) == 0 {
		val = "unknown"
	}
	return val
}

func getRuntime(getenv func(string) string) (rt string) {
	env := getenv("DD_RUNTIME")
	switch env {
	case "dotnet":
		// this value matches the runtime value set in the azure extension for
		// windows
		// TODO const this
		rt = ".NET"
	case "node":
		rt = "node.js"
	default:
		rt = "unknown"
	}
	return
}

func parseAzureSubscriptionID(subID string) (id string) {
	if len(subID) > 0 {
		// TODO what if "+" is not in the subID?
		id = strings.SplitN(subID, "+", 1)[0]
	}
	// TODO: logging
	return
}

func compileAzureResourceID(subID, resourceGroup, siteName string) (id string) {
	if len(subID) > 0 && len(resourceGroup) > 0 && len(siteName) > 0 {
		id = fmt.Sprintf("/subscriptions/%s/resourcegroups/%s/providers/microsoft.web/sites/%s",
			subID, resourceGroup, siteName)
	}
	// TODO: logging
	return
}
