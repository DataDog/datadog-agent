// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package status

import (
	"bytes"
	"context"
	"errors"
	"io"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
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

func (s *baseRunSuite) readUntil(stdout io.Reader, str string) {
	s.Assert().EventuallyWithT(func(c *assert.CollectT) {
		out := make([]byte, 0x4000)
		_, e := stdout.Read(out)
		if e != nil && !errors.Is(e, io.EOF) {
			c.Errorf("error reading stdout %s", e)
			c.FailNow()
		}
		if !assert.True(c, bytes.Contains(out, []byte(str)), "Did not fine %s", str) {
			s.T().Logf("Waiting for %s", str)
		}

	}, 3*time.Minute, 1*time.Second, "Did Not find %s", str)
}
