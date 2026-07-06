// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	"github.com/stretchr/testify/require"

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// TestNewComponentReturnsDisabledStubWhenOff verifies the off-by-default fast
// path: with anomaly_detection.enabled=false and no recorder, NewComponent must
// return the zero-allocation disabledObserver stub rather than building the
// engine, storage, catalog, 1000-cap channel, and dispatch goroutine.
func TestNewComponentReturnsDisabledStubWhenOff(t *testing.T) {
	// anomaly_detection.enabled defaults to false; the mock carries that default.
	cfg := configmock.New(t)

	provides, err := NewComponent(Requires{
		Config:   cfg,
		Recorder: option.None[recorderdef.Component](),
	})
	require.NoError(t, err)

	_, ok := provides.Comp.(*disabledObserver)
	require.Truef(t, ok, "expected *disabledObserver when anomaly detection is disabled, got %T", provides.Comp)
}
