package cloudservice

import (
	"os"
	"testing"

	"gotest.tools/assert"
)

func TestGetCloudServiceType(t *testing.T) {
	os.Setenv(ContainerAppNameEnvVar, "test-name")
	assert.Equal(t, GetCloudServiceType(), &ContainerApp{})

	os.Unsetenv(ContainerAppNameEnvVar)
	assert.Equal(t, GetCloudServiceType(), &CloudRun{})
}
