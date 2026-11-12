// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package selfident resolves the agent's own Kubernetes DaemonSet identity,
// so issues caused by a cluster-distributed template (a bad cluster check,
// a cluster-distributed config file) share one discriminator across every
// node agent, letting the backend collapse them into a single issue.
//
// The resolver lives in selfident.go behind the kubeapiserver build tag;
// selfident_noop.go provides the same API returning empty values for flavors
// built without it, none of which can run as a DaemonSet. This file is
// deliberately untagged so the package keeps a doc comment under either tag
// set — see pkg/util/cloudproviders/kubernetes/docs.go for the same pattern.
package selfident

import "context"

// Resolver is the identity dependency consumed by healthplatform. Keeping the
// dependency as an interface lets callers test behavior when resolution stalls.
type Resolver interface {
	DeploymentID() string
	DeploymentIDWithContext(context.Context) string
	ClusterID() string
	ClusterIDWithContext(context.Context) string
	IssueDiscriminator() string
}
