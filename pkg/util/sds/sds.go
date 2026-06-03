// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sds is a small example utility package.
package sds

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// HelloWorld logs a "hello world" message through the agent logger.
func HelloWorld() {
	log.Info("hello world")
}
