// +build kubeapiserver

package topology_collectors

import (
	"errors"
)

const (
	Deployment  = "Deployment"
	DaemonSet   = "DaemonSet"
	StatefulSet = "StatefulSet"
	ReplicaSet  = "ReplicaSet"
)

// ClusterTopologyCollector collects cluster components and relations.
type ClusterTopologyCollector interface {
	CollectorFunction() error
	ClusterTopologyCommon
}

type clusterTopologyCollector struct {
	ClusterTopologyCommon
}

// NewClusterTopologyCollector
func NewClusterTopologyCollector(clusterTopologyCommon ClusterTopologyCommon) ClusterTopologyCollector {
	return &clusterTopologyCollector{clusterTopologyCommon}
}


// CollectorFunction
func (c *clusterTopologyCollector) CollectorFunction() error {
	return errors.New("CollectorFunction NotImplemented")
}
