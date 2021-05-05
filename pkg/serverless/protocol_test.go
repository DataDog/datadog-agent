// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverless

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWaitForDaemonBlocking(t *testing.T) {
	assert := assert.New(t)
	d := StartDaemon()
	ctx := context.Background()
	defer d.Shutdown(ctx)

	// WaitForDaemon doesn't block if the client library hasn't
	// registered with the extension's /hello route
	d.clientLibReady = false
	d.WaitForDaemon()

	// WaitForDaemon blocks if the client library has registered with the extension's /hello route
	d.clientLibReady = true

	d.StartInvocation()

	complete := false
	go func() {
		<-time.After(100 * time.Millisecond)
		complete = true
		d.FinishInvocation()
	}()
	d.WaitForDaemon()
	assert.Equal(complete, true, "daemon didn't block until FinishInvocation")
}

func TestWaitUntilReady(t *testing.T) {
	assert := assert.New(t)
	d := StartDaemon()
	ctx := context.Background()
	defer d.Shutdown(ctx)

	ready := d.WaitUntilClientReady(50 * time.Millisecond)
	assert.Equal(ready, false, "client was ready")
}
