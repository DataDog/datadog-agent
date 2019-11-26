// +build !kubelet

package util

func isAgentKubeHostNetwork() (bool, error) {
	return true, nil
}
