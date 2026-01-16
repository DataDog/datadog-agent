// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filterlistimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	telemetrynoop "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/require"
)

func TestHistogramMetricNamesFilter(t *testing.T) {
	cfg := make(map[string]interface{})
	require := require.New(t)

	cfg["histogram_aggregates"] = []string{"avg", "max", "median"}
	cfg["histogram_percentiles"] = []string{"0.73", "0.22"}

	logComponent := logmock.New(t)
	configComponent := config.NewMockWithOverrides(t, cfg)
	telemetryComponent := fxutil.Test[telemetry.Component](t, telemetrynoop.Module())
	filterList := NewFilterList(logComponent, configComponent, telemetryComponent)

	bl := []string{
		"foo",
		"bar",
		"baz",
		"foomax",
		"foo.avg",
		"foo.max",
		"foo.count",
		"baz.73percentile",
		"bar.50percentile",
		"bar.22percentile",
		"count",
	}

	filtered := filterList.createHistogramsFilterList(bl)
	require.ElementsMatch(filtered, []string{"foo.avg", "foo.max", "baz.73percentile", "bar.22percentile"})
}
