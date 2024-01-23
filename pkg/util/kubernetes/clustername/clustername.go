// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clustername provides helpers to get a Kubernetes cluster name.
package clustername

import (
	"context"
	"regexp"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/azure"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/gce"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
)

const (
	clusterIDEnv = "DD_ORCHESTRATOR_CLUSTER_ID"
)

// validClusterName matches exactly the same naming rule as the one enforced by GKE:
// https://cloud.google.com/kubernetes-engine/docs/reference/rest/v1beta1/projects.locations.clusters#Cluster.FIELDS.name
// The cluster name can be up to 40 characters with the following restrictions:
// * Lowercase letters, numbers, dots and hyphens only.
// * Must start with a letter.
// * Must end with a number or a letter.
// * Must be a valid FQDN (without trailing period)
var validClusterName = regexp.MustCompile(`^([a-z]([a-z0-9\-]*[a-z0-9])?\.)*([a-z]([a-z0-9\-]*[a-z0-9])?)$`)

type clusterNameData struct {
	clusterName string
	initDone    bool
	mutex       sync.Mutex
}

// Provider is a generic function to grab the clustername and return it
type Provider func(context.Context) (string, error)

// ProviderCatalog holds all the various kinds of clustername providers
var ProviderCatalog map[string]Provider

func newClusterNameData() *clusterNameData {
	return &clusterNameData{}
}

var defaultClusterNameData *clusterNameData

func init() {
	defaultClusterNameData = newClusterNameData()
	ProviderCatalog = map[string]Provider{
		"gce":   gce.GetClusterName,
		"azure": azure.GetClusterName,
		"ec2":   ec2.GetClusterName,
	}
}

func getClusterName(ctx context.Context, data *clusterNameData, hostname string) string {
	panic("not called")
}

// GetClusterName returns a k8s cluster name if it exists, either directly specified or autodiscovered
func GetClusterName(ctx context.Context, hostname string) string {
	panic("not called")
}

// GetClusterNameTagValue  a k8s cluster name if it exists, either directly specified or autodiscovered
//
// This function also "normalize" the k8s cluster name if the configuration option
// "enabled_rfc1123_compliant_cluster_name_tag" is set to "true"
// this allow to limit the risk of breaking user that currently rely on previous `kube_cluster_name` tag value.
func GetClusterNameTagValue(ctx context.Context, hostname string) string {
	panic("not called")
}

// IsRFC1123CompliantClusterName check if the clusterName is RFC1123 compliant
// return false if not compliant
func IsRFC1123CompliantClusterName(clusterName string) bool {
	panic("not called")
}

// GetRFC1123CompliantClusterName returns an RFC-1123 compliant k8s cluster
// name if it exists, either directly specified or autodiscovered
func GetRFC1123CompliantClusterName(ctx context.Context, hostname string) string {
	panic("not called")
}

func resetClusterName(data *clusterNameData) {
	panic("not called")
}

// ResetClusterName resets the clustername, which allows it to be detected again. Used for tests
func ResetClusterName() {
	panic("not called")
}

// GetClusterID looks for an env variable which should contain the cluster ID.
// This variable should come from a configmap, created by the cluster-agent.
// This function is meant for the node-agent to call (cluster-agent should call GetOrCreateClusterID)
func GetClusterID() (string, error) {
	panic("not called")
}

// MakeClusterNameRFC1123Compliant returns the RFC-1123 compliant cluster name.
func MakeClusterNameRFC1123Compliant(clusterName string) string {
	panic("not called")
}

// setProviderCatalog should only be used for testing.
func setProviderCatalog(catalog map[string]Provider) {
	panic("not called")
}
