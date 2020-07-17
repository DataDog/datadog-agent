// +build kubelet

package util

import (
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

func isAgentKubeHostNetwork() (bool, error) {
	ku, err := kubelet.GetKubeUtil()
	if err != nil {
		return true, err
	}

	return ku.IsAgentHostNetwork()
}
