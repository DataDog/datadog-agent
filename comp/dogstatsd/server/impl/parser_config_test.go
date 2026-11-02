// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/server/internal/wire"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/stretchr/testify/require"
)

func TestParserOptions(t *testing.T) {
	cfg := configmock.New(t)
	// The real configuration supplies canonical defaults; parsing fixtures do not.
	require.Equal(t, wire.Options{InternerSize: 4096, ReadTimestamps: true, OriginDetection: false}, parserOptions(cfg))
	for _, capacity := range []int{-1, 0, 1, 4096} {
		for _, timestamps := range []bool{false, true} {
			for _, origin := range []bool{false, true} {
				cfg.SetInTest("dogstatsd_string_interner_size", capacity)
				cfg.SetInTest("dogstatsd_no_aggregation_pipeline", timestamps)
				cfg.SetInTest("dogstatsd_origin_detection_client", origin)
				require.Equal(t, wire.Options{InternerSize: capacity, ReadTimestamps: timestamps, OriginDetection: origin}, parserOptions(cfg))
			}
		}
	}
}
