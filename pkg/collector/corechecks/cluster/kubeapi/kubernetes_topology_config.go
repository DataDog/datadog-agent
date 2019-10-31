package kubeapi

import (
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	"github.com/StackVista/stackstate-agent/pkg/config"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"gopkg.in/yaml.v2"
)

// ClusterType represents the type of the cluster being monitored - Kubernetes / OpenShift
type ClusterType string
const (
	// Kubernetes cluster type
	Kubernetes ClusterType = "kubernetes"
	// OpenShift cluster type
	OpenShift              = "openshift"
)

// TopologyConfig is the config of the API server.
type TopologyConfig struct {
	ClusterName     string `yaml:"cluster_name"`
	CollectTopology bool   `yaml:"collect_topology"`
	CollectTimeout  int   `yaml:"collect_timeout"`
	CheckID         check.ID
	Instance        topology.Instance
}

func (c *TopologyConfig) parse(data []byte) error {
	// default values
	c.ClusterName = config.Datadog.GetString("cluster_name")
	c.CollectTopology = config.Datadog.GetBool("collect_kubernetes_topology")
	c.CollectTimeout = config.Datadog.GetInt("collect_kubernetes_timeout")

	return yaml.Unmarshal(data, c)
}
