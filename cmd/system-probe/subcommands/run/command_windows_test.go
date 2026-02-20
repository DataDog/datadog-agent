// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package run

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestStartSystemProbe(t *testing.T) {
	fxutil.TestOneShot(t, func() {
		ctxChan := make(<-chan context.Context)
		errChan := make(chan error)
		_ = runSystemProbe(ctxChan, errChan)
	})
}
