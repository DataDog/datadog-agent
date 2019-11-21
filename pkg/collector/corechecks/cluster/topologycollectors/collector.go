// +build kubeapiserver

package topologycollectors

import (
	"errors"
)

const (
	Deployment  = "Deployment"
	DaemonSet   = "DaemonSet"
	StatefulSet = "StatefulSet"
	ReplicaSet  = "ReplicaSet"
	CronJob     = "CronJob"
	Job         = "Job"
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
