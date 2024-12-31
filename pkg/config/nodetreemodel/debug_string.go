// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !test
// +build !test

package nodetreemodel

import "github.com/DataDog/datadog-agent/pkg/config/model"

func (c *ntmConfig) toDebugString(_ model.Source) (string, error) {
	// don't show any data outside of tests, that way we don't have to worry about scrubbing
	return "nodeTreeModelConfig{...}", nil
}
