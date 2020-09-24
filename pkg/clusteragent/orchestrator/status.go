package orchestrator

import (
	"encoding/json"
	"expvar"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"k8s.io/client-go/kubernetes"
)

// GetStatus returns status info for the secret and webhook controllers.
func GetStatus(apiCl kubernetes.Interface) map[string]interface{} {
	status := make(map[string]interface{})
	if !config.Datadog.GetBool("orchestrator_explorer.enabled") {
		status["Disabled"] = "The orchestrator explorer is not enabled on the Cluster Agent"
		return status
	}

	// get cluster uid
	clusterID, err := common.GetOrCreateClusterID(apiCl.CoreV1())
	if err != nil {
		status["ClusterIDError"] = err.Error()
	} else {
		status["ClusterID"] = clusterID
	}
	// get cluster name
	hostname, err := util.GetHostname()
	if err != nil {
		status["ClusterNameError"] = err.Error()
	} else {
		status["ClusterName"] = clustername.GetClusterName(hostname)
	}

	// get orchestrator endpoints
	orchestratorEndpoints := config.Datadog.GetString("orchestrator_explorer.orchestrator_additional_endpoints")
	if orchestratorEndpoints != "" {
		status["OrchestratorAdditionalEndpoints"] = orchestratorEndpoints
	}
	orchestratorEndpoint := config.Datadog.GetString("orchestrator_explorer.orchestrator_dd_url")
	if orchestratorEndpoint != "" {
		status["OrchestratorEndpoint"] = orchestratorEndpoint
	}

	// get forwarder stats
	forwarderStatsJSON := []byte(expvar.Get("forwarder").String())
	forwarderStats := make(map[string]interface{})
	json.Unmarshal(forwarderStatsJSON, &forwarderStats) //nolint:errcheck
	status["forwarderStatsPods"] = forwarderStats["Pods"]
	status["forwarderStatsDeployment"] = forwarderStats["Deployments"]
	status["forwarderStatsReplicaSets"] = forwarderStats["ReplicaSets"]
	status["forwarderStatsServices"] = forwarderStats["Services"]
	status["forwarderStatsNodes"] = forwarderStats["Nodes"]

	// get informer status

	return status
}
