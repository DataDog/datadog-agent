// +build !docker

package util

import (
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// GetAgentUTSMode retrieves from Docker the UTS mode of the Agent container
func GetAgentUTSMode() (containers.UTSMode, error) {
	return containers.UnknownUTSMode, nil
}
