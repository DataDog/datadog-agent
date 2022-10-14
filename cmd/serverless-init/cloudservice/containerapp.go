package cloudservice

import "os"

// ContainerApp has helper functions for getting specific Azure Container App data
type ContainerApp struct{}

// ContainerAppNameEnvVar is the environment variable that is present when we're
// running in Azure App Container.
const ContainerAppNameEnvVar = "CONTAINER_APP_NAME"

// GetTags returns a map of Azure-related tags
func (c *ContainerApp) GetTags() map[string]string {
	// Not implemented
	return nil
}

// GetOrigin returns the `origin` attribute type for the given
// cloud service.
func (c *ContainerApp) GetOrigin() string {
	return "containerapp"
}

func isContainerAppService() bool {
	_, exists := os.LookupEnv(ContainerAppNameEnvVar)
	return exists
}
