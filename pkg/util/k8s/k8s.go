package k8s

import "fmt"

// GetNodeInfo returns the IP address and the hostname of the node where
// this pod is running.
// TODO: see https://github.com/kubernetes/client-go for the client
func GetNodeInfo() (ip, name string, err error) {
	err = fmt.Errorf("Not yet implemented")
	return
}
