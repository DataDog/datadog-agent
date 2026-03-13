// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package utils

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func PulumiDependsOn(resources ...pulumi.Resource) pulumi.ResourceOption {
	return pulumi.DependsOn(resources)
}

func MergeOptions[T any](current []T, opts ...T) []T {
	if len(opts) == 0 {
		return current
	}

	addedOptions := make([]T, 0, len(current)+len(opts))
	addedOptions = append(addedOptions, current...)
	addedOptions = append(addedOptions, opts...)

	return addedOptions
}

func StringPtr(s string) pulumi.StringPtrInput {
	if len(s) > 0 {
		return pulumi.StringPtr(s)
	}

	return nil
}
