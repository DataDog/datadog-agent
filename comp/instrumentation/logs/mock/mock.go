// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

// Package mock provides a test double for the logs handler component.
package mock

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/instrumentation/common/handlermock"
	logs "github.com/DataDog/datadog-agent/comp/instrumentation/logs/def"
)

var _ logs.Component = (*handlermock.Handler)(nil)

// Mock creates a configurable logs handler test double.
func Mock(t *testing.T) *handlermock.Handler { return handlermock.New(t, "logs") }
