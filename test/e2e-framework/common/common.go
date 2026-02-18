// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package common

func ApplyOption[T any](instance *T, options []func(*T) error) (*T, error) {
	for _, o := range options {
		if err := o(instance); err != nil {
			return nil, err
		}
	}
	return instance, nil
}
