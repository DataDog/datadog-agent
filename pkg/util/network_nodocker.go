// +build !docker

package util

// GetAgentNetworkMode retrieves from Docker the network mode of the Agent container
func GetAgentNetworkMode() (string, error) {
	return "", nil
}
