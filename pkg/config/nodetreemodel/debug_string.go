// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !test

package nodetreemodel

import (
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type treeDebugger struct{}

func panicInTest(format string, params ...interface{}) {
	log.Errorf(format, params...)
}

func (c *ntmConfig) toDebugString(_ model.Source, _ ...model.StringifyOption) (string, error) {
	// don't show any data outside of tests, that way we don't have to worry about scrubbing
	return "nodeTreeModelConfig{...no-test-tag...}", nil
}
