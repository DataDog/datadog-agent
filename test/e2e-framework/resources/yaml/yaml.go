// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package yaml registers `kubernetes:yaml/v2:ConfigGroup` components without the SDK's `yaml` packages, which import
// most of the pulumi-kubernetes SDK.
package yaml

import (
	"context"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/utilities"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/internals"
)

type ConfigGroup struct {
	pulumi.ResourceState
}

// NewConfigGroup mirrors `yaml/v2.NewConfigGroup`, with `args` keyed by `ConfigGroupArgs`' Pulumi names.
func NewConfigGroup(ctx *pulumi.Context, name string, args pulumi.Map, opts ...pulumi.ResourceOption) (*ConfigGroup, error) {
	var group ConfigGroup
	if err := ctx.RegisterRemoteComponentResource("kubernetes:yaml/v2:ConfigGroup", name, args, &group, utilities.PkgResourceDefaultOpts(opts)...); err != nil {
		return nil, err
	}
	return &group, nil
}

// Transformation mutates a Kubernetes object, as `yaml.Transformation` did before `yaml/v2`.
type Transformation func(state map[string]interface{}, opts ...pulumi.ResourceOption)

// Transforms applies `transformations` to the Kubernetes objects of a `ConfigGroup`, once known.
func Transforms(transformations ...Transformation) pulumi.ResourceOption {
	return pulumi.Transforms([]pulumi.ResourceTransform{func(ctx context.Context, args *pulumi.ResourceTransformArgs) *pulumi.ResourceTransformResult {
		if !args.Custom {
			return nil
		}
		props, err := internals.UnsafeAwaitOutput(ctx, args.Props.ToMapOutput())
		if err != nil || !props.Known {
			return nil
		}
		state := props.Value.(map[string]interface{})
		for _, transformation := range transformations {
			transformation(state)
		}
		result := pulumi.ToMap(state)
		if props.Secret {
			for key, value := range result {
				result[key] = pulumi.ToSecret(value)
			}
		}
		return &pulumi.ResourceTransformResult{Props: result, Opts: args.Opts}
	}})
}
