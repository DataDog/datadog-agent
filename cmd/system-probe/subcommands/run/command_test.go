// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	"context"
	_ "net/http/pprof"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestRunCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"run"},
		run,
		func() {})
}

func TestStartSystemProbe(t *testing.T) {
	fxutil.TestOneShot(t, func() {
		ctxChan := make(<-chan context.Context)
		errChan := make(chan error)
		runSystemProbe(ctxChan, errChan)
	})
}
