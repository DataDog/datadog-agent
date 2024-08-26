// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package status

import (
	"context"
	"errors"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
)

type baseRunSuite struct {
	e2e.BaseSuite[environments.Host]
}

func (s *baseRunSuite) runTimeout() time.Duration {
	return 5 * time.Minute
}

func runCommandWithTimeout(host *components.RemoteHost, cmd string, timeout time.Duration) (string, error) {
	var out string
	var err error
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	go func() {
		out, err = host.Execute(cmd)
		cancel()
	}()
	<-ctx.Done()
	if errors.Is(ctx.Err(), context.Canceled) {
		// return the execute error
		return out, err
	}
	// return the timeout error
	return out, ctx.Err()
}
