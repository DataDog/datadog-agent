// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package nat64 deploys an aojea/nat64 DaemonSet into a Kubernetes cluster so
// that IPv6-only pods can reach IPv4-only services through the well-known
// NAT64 prefix 64:ff9b::/96. kindest/kindnetd, the CNI bundled with kind,
// does not implement NAT64 itself, so an IPv6-only kind cluster needs this
// daemon to make external IPv4 endpoints reachable.
package nat64

import (
	_ "embed"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

//go:embed nat64.yaml
var manifest string

// Deploy applies the pinned aojea/nat64 manifest to the cluster reachable via
// kubeProvider. Returns the resulting ConfigGroup so callers can chain
// dependsOn against it.
func Deploy(ctx *pulumi.Context, name string, kubeProvider *kubernetes.Provider, opts ...pulumi.ResourceOption) (*yaml.ConfigGroup, error) {
	opts = append(opts, pulumi.Provider(kubeProvider))
	return yaml.NewConfigGroup(ctx, name, &yaml.ConfigGroupArgs{
		YAML: []string{manifest},
	}, opts...)
}
