// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package helm registers `kubernetes:helm.sh/v3:Release` resources without the SDK's `helm/v3` package, which
// imports most of the pulumi-kubernetes SDK.
package helm

import (
	"reflect"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/utilities"
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
	Devel          pulumi.BoolPtrInput
	TimeoutSeconds int // Optional timeout in seconds (default: 300)
}

// Important: set relevant Kubernetes provider in `opts`
func NewInstallation(e config.Env, args InstallArgs, opts ...pulumi.ResourceOption) (*Release, error) {
	releaseArgs := pulumi.Map{
		"namespace":        pulumi.String(args.Namespace),
		"name":             pulumi.String(args.InstallName),
		"repositoryOpts":   pulumi.Map{"repo": pulumi.String(args.RepoURL)},
		"chart":            pulumi.String(args.ChartName),
		"createNamespace":  pulumi.Bool(true),
		"dependencyUpdate": pulumi.Bool(true),
		"valueYamlFiles":   args.ValuesYAML,
		"values":           args.Values,
		"version":          args.Version,
		"devel":            args.Devel,
	}
	// Set timeout if specified, otherwise use default
	if args.TimeoutSeconds > 0 {
		releaseArgs["timeout"] = pulumi.Int(args.TimeoutSeconds)
	}
	return NewRelease(e.Ctx(), args.InstallName, releaseArgs, opts...)
}

type Release struct {
	pulumi.CustomResourceState

	Name   pulumi.StringPtrOutput `pulumi:"name"`
	Status ReleaseStatusOutput    `pulumi:"status"`
}

// NewRelease mirrors `helm/v3.NewRelease`, with `args` keyed by `ReleaseArgs`' Pulumi names.
func NewRelease(ctx *pulumi.Context, name string, args pulumi.Map, opts ...pulumi.ResourceOption) (*Release, error) {
	args["compat"] = pulumi.String("true")
	var release Release
	if err := ctx.RegisterResource("kubernetes:helm.sh/v3:Release", name, args, &release, utilities.PkgResourceDefaultOpts(opts)...); err != nil {
		return nil, err
	}
	return &release, nil
}

type releaseStatus struct {
	AppVersion *string `pulumi:"appVersion"`
	Version    *string `pulumi:"version"`
}

type ReleaseStatusOutput struct{ *pulumi.OutputState }

func (ReleaseStatusOutput) ElementType() reflect.Type {
	return reflect.TypeOf((*releaseStatus)(nil)).Elem()
}

func (o ReleaseStatusOutput) AppVersion() pulumi.StringPtrOutput {
	return o.ApplyT(func(v releaseStatus) *string { return v.AppVersion }).(pulumi.StringPtrOutput)
}

func (o ReleaseStatusOutput) Version() pulumi.StringPtrOutput {
	return o.ApplyT(func(v releaseStatus) *string { return v.Version }).(pulumi.StringPtrOutput)
}

func init() {
	pulumi.RegisterOutputType(ReleaseStatusOutput{})
}
