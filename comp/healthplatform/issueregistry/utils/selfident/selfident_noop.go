// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !kubeapiserver

package selfident

import (
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// SelfIdent is a no-op on flavors built without Kubernetes support. Those
// agents never run as a DaemonSet — the iot and heroku agents ship only as
// system packages, and the cloudfoundry cluster agent has no Kubernetes at all
// — so there is no deployment_id or cluster id to resolve. Both getters return
// empty strings.
//
// Keeping this out of those builds also avoids linking the whole resolver, and
// with it the retry loop in DeploymentID: env.IsFeaturePresent(env.Kubernetes)
// is not build-tag gated, so a package-installed agent on a Kubernetes node
// would otherwise retry a lookup that can never succeed without the kubelet
// workloadmeta collector.
type SelfIdent struct{}

// New returns the no-op SelfIdent. The workloadmeta component is accepted, and
// ignored, to keep the same signature as the Kubernetes implementation.
func New(workloadmeta.Component) *SelfIdent {
	return &SelfIdent{}
}

// DeploymentID always returns "" without Kubernetes support.
func (*SelfIdent) DeploymentID() string { return "" }

// ClusterID always returns "" without Kubernetes support.
func (*SelfIdent) ClusterID() string { return "" }

// IssueDiscriminator always returns "" without Kubernetes support: there is no
// DaemonSet to scope issue ids by.
func (*SelfIdent) IssueDiscriminator() string { return "" }
