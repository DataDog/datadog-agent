// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

// InstrumentAPMInjector instruments the APM injector for IIS on Windows
func InstrumentAPMInjector(ctx context.Context, method string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "instrument_injector")
	defer func() { span.Finish(err) }()

	switch method {
	case env.APMInstrumentationEnabledIIS:
		err = instrumentDotnetLibrary(ctx, "stable")
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("Unsupported method: %s", method)

	}

	return nil
}

// UninstrumentAPMInjector un-instruments the APM injector for IIS on Windows
func UninstrumentAPMInjector(ctx context.Context, method string) (err error) {
	span, ctx := telemetry.StartSpanFromContext(ctx, "uninstrument_injector")
	defer func() { span.Finish(err) }()

	switch method {
	case env.APMInstrumentationEnabledIIS:
		err = uninstrumentDotnetLibrary(ctx, "stable")
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("Unsupported method: %s", method)

	}

	return nil
}
