// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/prometheus"
)

func TestFindMetricFamily(t *testing.T) {
	families := []prometheus.MetricFamily{
		{Name: "apiserver_resource_objects"},
		{Name: "apiserver_storage_objects"},
	}

	assert.Same(t, &families[1], findMetricFamily(families, "apiserver_storage_objects", "apiserver_resource_objects"))
	assert.Same(t, &families[0], findMetricFamily(families, "missing", "apiserver_resource_objects"))
	assert.Nil(t, findMetricFamily(families, "missing"))
}
