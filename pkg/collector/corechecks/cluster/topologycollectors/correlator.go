// +build kubeapiserver

package topologycollectors

import (
	"errors"
)

// ClusterTopologyCorrelator collects cluster components and relations.
type ClusterTopologyCorrelator interface {
	CorrelateFunction() error
	ClusterTopologyCommon
}

type clusterTopologyCorrelator struct {
	ClusterTopologyCommon
}

// NewClusterTopologyCorrelator
func NewClusterTopologyCorrelator(clusterTopologyCommon ClusterTopologyCommon) ClusterTopologyCorrelator {
	return &clusterTopologyCorrelator{clusterTopologyCommon}
}

// CollectorFunction
func (c *clusterTopologyCorrelator) CorrelateFunction() error {
	return errors.New("CollectorFunction NotImplemented")
}
