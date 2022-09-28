// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stopper

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/fx"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/internal"
	"github.com/DataDog/datadog-agent/comp/core/log"
)

// Note that we do not test catching signals, since that affects global state
// (process signals)

func TestStopper(t *testing.T) {
	var stopper Component
	var gotErr error
	app := fx.New(
		fx.Supply(fx.Annotate(t, fx.As(new(testing.TB)))),
		fx.Supply(internal.BundleParams{StopErrorP: &gotErr}),
		log.MockModule,
		Module,
		fx.Populate(&stopper),
	)

	startCtx, cancel := context.WithTimeout(context.Background(), app.StartTimeout())
	defer cancel()

	require.NoError(t, app.Start(startCtx))

	select {
	case <-app.Done():
		t.Fatal("app should not have stopped yet")
	default:
	}

	stopper.Stop(errors.New("uhoh"))

	<-app.Done()

	require.ErrorContains(t, gotErr, "uhoh")
}
