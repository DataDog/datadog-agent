// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !foldspace

package foldspace

import "errors"

// BuiltWithFoldspace is whether this binary was compiled with the foldspace tag.
const BuiltWithFoldspace = false

// NewNativeCore returns an error: this binary was not built with the foldspace tag.
func NewNativeCore(_ Config) (Core, error) {
	return nil, errors.New("foldspace native core requires building with the foldspace tag")
}
