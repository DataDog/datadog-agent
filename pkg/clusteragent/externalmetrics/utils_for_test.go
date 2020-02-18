// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package externalmetrics

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/externalmetrics/model"
	"github.com/stretchr/testify/assert"
)

// Fake LeaderElector
type fakeLeaderElector struct {
	isLeader bool
}

func (le *fakeLeaderElector) IsLeader() bool {
	return le.isLeader
}

func compareDatadogMetricInternal(t *testing.T, expected, actual *model.DatadogMetricInternal) {
	t.Helper()
	assert.Condition(t, func() bool { return actual.UpdateTime.After(expected.UpdateTime) })
	alignedTime := time.Now().UTC()
	expected.UpdateTime = alignedTime
	actual.UpdateTime = alignedTime

	assert.Equal(t, expected, actual)
}
