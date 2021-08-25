// +build !docker

package util

import "context"

// GetAgentNetworkMode retrieves from Docker the network mode of the Agent container
func GetAgentNetworkMode(context.Context) (string, error) {
	return "", nil
}
