// +build !docker

package util

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// GetAgentUTSMode retrieves from Docker the UTS mode of the Agent container
func GetAgentUTSMode(context.Context) (containers.UTSMode, error) {
	return containers.UnknownUTSMode, nil
}
