// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package helm

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type InstallArgs struct {
	RepoURL        string
	ChartName      string
	InstallName    string
	Namespace      string
	ValuesYAML     pulumi.AssetOrArchiveArrayInput
	Values         pulumi.MapInput
	Version        pulumi.StringPtrInput
	TimeoutSeconds int // Optional timeout in seconds (default: 300)
}

// Important: set relevant Kubernetes provider in `opts`
func NewInstallation(e config.Env, args InstallArgs, opts ...pulumi.ResourceOption) (*helm.Release, error) {
	releaseArgs := &helm.ReleaseArgs{
		Namespace: pulumi.StringPtr(args.Namespace),
		Name:      pulumi.StringPtr(args.InstallName),
		RepositoryOpts: helm.RepositoryOptsArgs{
			Repo: pulumi.StringPtr(args.RepoURL),
		},
		Chart:            pulumi.String(args.ChartName),
		CreateNamespace:  pulumi.BoolPtr(true),
		DependencyUpdate: pulumi.BoolPtr(true),
		ValueYamlFiles:   args.ValuesYAML,
		Values:           args.Values,
		Version:          args.Version,
	}
	// Set timeout if specified, otherwise use default
	if args.TimeoutSeconds > 0 {
		releaseArgs.Timeout = pulumi.IntPtr(args.TimeoutSeconds)
	}
	return helm.NewRelease(e.Ctx(), args.InstallName, releaseArgs, opts...)
}
