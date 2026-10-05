// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/client"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/client/fake"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const password = "hunter2-Sup3rS3cret"

var testConfig = client.Config{Host: "myacct.file.core.windows.net", Share: "logs", Username: "myacct", Password: password}

func errRefused() error {
	return &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}
}

// newClient returns a reconnecting client on a fresh fake share, with a mock
// clock.
func newClient(t *testing.T) (client.Client, *fake.Share, *clock.Mock) {
	t.Helper()
	share := fake.New()
	clk := clock.NewMock()
	c := client.NewReconnecting(testConfig, client.WithDialer(share.Dial), client.WithClock(clk))
	t.Cleanup(func() { c.Close() })
	return c, share, clk
}

// captureLogs redirects the Agent logger to a buffer while fn runs and
// returns everything logged, at every level.
func captureLogs(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	logger, err := log.LoggerFromWriterWithMinLevelAndLvlMsgFormat(w, log.TraceLvl)
	require.NoError(t, err)
	previous := log.Default()
	t.Cleanup(func() { log.SetupLogger(previous, "debug") })
	log.SetupLogger(logger, "trace")
	fn()
	require.NoError(t, w.Flush())
	return buf.String()
}

func TestReconnectingDialsLazilyAndOnce(t *testing.T) {
	c, share, _ := newClient(t)
	share.Write("app/a.log", []byte("hello\n"))
	assert.Zero(t, share.Calls(fake.OpDial), "no dial before the first call")

	entries, err := c.ListDir(context.Background(), "app")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	res, err := c.ReadAt(context.Background(), "app/a.log", 0, 100)
	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(res.Data))

	assert.Equal(t, 1, share.Calls(fake.OpDial))
	assert.Equal(t, 1, share.LiveSessions())
}

func TestReconnectingRedialsAfterTransientErrorAndResumes(t *testing.T) {
	c, share, _ := newClient(t)
	ctx := context.Background()
	share.Write("a.log", []byte("line 1\n"))

	res, err := c.ReadAt(ctx, "a.log", 0, 100)
	require.NoError(t, err)
	offset := int64(len(res.Data))

	share.Append("a.log", []byte("line 2\n"))
	share.DropSessions()
	_, err = c.ReadAt(ctx, "a.log", offset, 100)
	require.Error(t, err)
	assert.Equal(t, client.ErrTransient, client.Classify(err))
	assert.Zero(t, share.LiveSessions(), "the broken session is closed")

	// The caller retries from the offset it kept: no bytes are read twice.
	res, err = c.ReadAt(ctx, "a.log", offset, 100)
	require.NoError(t, err, "the first call after a drop redials without waiting")
	assert.Equal(t, "line 2\n", string(res.Data))
	assert.Equal(t, 2, share.Calls(fake.OpDial))
	assert.Equal(t, 1, share.LiveSessions())
}

func TestReconnectingBacksOffExponentially(t *testing.T) {
	c, share, clk := newClient(t)
	ctx := context.Background()
	share.Write("a.log", []byte("x"))

	// Every dial fails until the queue is empty.
	share.FailNext(fake.OpDial, errRefused(), errRefused(), errRefused(), errRefused(), errRefused(), errRefused(), errRefused())

	dials := 0
	for _, wait := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second} {
		_, err := c.ListDir(ctx, "")
		dials++
		require.Error(t, err)
		assert.Equal(t, client.ErrTransient, client.Classify(err))
		assert.Equal(t, dials, share.Calls(fake.OpDial))

		// Until the delay has passed, calls fail fast without dialing, with
		// an error that classifies like the dial failure.
		clk.Add(wait - time.Millisecond)
		_, err = c.ListDir(ctx, "")
		require.Error(t, err)
		assert.Equal(t, client.ErrTransient, client.Classify(err))
		assert.Contains(t, err.Error(), "next attempt in 1s")
		assert.Equal(t, dials, share.Calls(fake.OpDial), "no dial during the backoff (%s)", wait)
		clk.Add(time.Millisecond)
	}

	// The dial succeeds: the backoff resets.
	_, err := c.ListDir(ctx, "")
	require.NoError(t, err)

	share.DropSessions()
	_, err = c.ListDir(ctx, "")
	require.Error(t, err)
	share.FailNext(fake.OpDial, errRefused())
	_, err = c.ListDir(ctx, "")
	require.Error(t, err)
	clk.Add(time.Second)
	_, err = c.ListDir(ctx, "")
	require.NoError(t, err, "the backoff started over at 1s")
}

func TestReconnectingAuthFailureWaitsTheMaximum(t *testing.T) {
	c, share, clk := newClient(t)
	ctx := context.Background()
	share.FailNext(fake.OpDial, fake.ErrAuth)

	_, err := c.ListDir(ctx, "")
	require.Error(t, err)
	assert.Equal(t, client.ErrAuth, client.Classify(err))

	clk.Add(29 * time.Second)
	_, err = c.ListDir(ctx, "")
	require.Error(t, err)
	assert.Equal(t, client.ErrAuth, client.Classify(err), "a call during the backoff reports the auth failure")
	assert.Equal(t, 1, share.Calls(fake.OpDial))

	clk.Add(time.Second)
	_, err = c.ListDir(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, 2, share.Calls(fake.OpDial))
}

func TestReconnectingKeepsSessionOnFileErrors(t *testing.T) {
	c, share, _ := newClient(t)
	ctx := context.Background()
	share.Write("a.log", []byte("x"))
	share.FailNextPath(fake.OpReadAt, "a.log", fake.ErrNotFound, fake.ErrSharing, fake.ErrAccessDenied, errors.New("other"))

	for _, want := range []client.ErrorKind{client.ErrNotFound, client.ErrSharing, client.ErrAuth, client.ErrOther} {
		_, err := c.ReadAt(ctx, "a.log", 0, 1)
		require.Error(t, err)
		assert.Equal(t, want, client.Classify(err))
	}
	_, err := c.ReadAt(ctx, "missing.log", 0, 1)
	assert.Equal(t, client.ErrNotFound, client.Classify(err))

	_, err = c.ReadAt(ctx, "a.log", 0, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, share.Calls(fake.OpDial))
	assert.Equal(t, 1, share.LiveSessions())
}

func TestReconnectingKeepsSessionWhenCallerGivesUp(t *testing.T) {
	c, share, _ := newClient(t)
	_, err := c.ListDir(context.Background(), "")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.ListDir(ctx, "")
	require.ErrorIs(t, err, context.Canceled)

	_, err = c.ListDir(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, 1, share.Calls(fake.OpDial), "a canceled call says nothing about the session")
}

func TestReconnectingCanceledDialDoesNotBackOff(t *testing.T) {
	c, share, _ := newClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.ListDir(ctx, "")
	require.ErrorIs(t, err, context.Canceled)

	_, err = c.ListDir(context.Background(), "")
	require.NoError(t, err, "no backoff after a dial canceled by the caller")
	assert.Equal(t, 2, share.Calls(fake.OpDial))
}

func TestReconnectingClose(t *testing.T) {
	c, share, _ := newClient(t)
	_, err := c.ListDir(context.Background(), "")
	require.NoError(t, err)

	require.NoError(t, c.Close())
	assert.Zero(t, share.LiveSessions())
	require.NoError(t, c.Close(), "Close is idempotent")

	_, err = c.ListDir(context.Background(), "")
	assert.ErrorIs(t, err, client.ErrClosed)
	_, err = c.ReadAt(context.Background(), "a.log", 0, 1)
	assert.ErrorIs(t, err, client.ErrClosed)
	assert.Equal(t, 1, share.Calls(fake.OpDial), "no dial after Close")
}

func TestReconnectingSharesOneDialAcrossCallers(t *testing.T) {
	c, share, _ := newClient(t)
	release := make(chan struct{})
	dialing := make(chan struct{}, 1)
	share.SetHook(func(op fake.Op, _ string) {
		if op == fake.OpDial {
			select {
			case dialing <- struct{}{}:
			default:
			}
			<-release
		}
	})

	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.ListDir(context.Background(), "")
			errs <- err
		}()
	}
	<-dialing
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, share.Calls(fake.OpDial))
	assert.Equal(t, 1, share.LiveSessions())
}

func TestReconnectingCloseDuringDial(t *testing.T) {
	c, share, _ := newClient(t)
	release := make(chan struct{})
	dialing := make(chan struct{})
	var once sync.Once
	share.SetHook(func(op fake.Op, _ string) {
		if op == fake.OpDial {
			once.Do(func() { close(dialing) })
			<-release
		}
	})

	errc := make(chan error, 1)
	go func() {
		_, err := c.ListDir(context.Background(), "")
		errc <- err
	}()
	<-dialing
	require.NoError(t, c.Close(), "Close does not wait for the dial")
	close(release)
	assert.ErrorIs(t, <-errc, client.ErrClosed)
	assert.Zero(t, share.LiveSessions(), "the session dialed after Close is closed")
}

func TestReconnectingNeverLeaksPassword(t *testing.T) {
	// A dialer whose errors carry the password, as a buggy library could.
	leakyDial := func(_ context.Context, cfg client.Config) (client.Client, error) {
		return nil, fmt.Errorf("NTLM negotiation failed for %s/%s: %w", cfg.Username, cfg.Password, fake.ErrAuth)
	}
	clk := clock.NewMock()
	c := client.NewReconnecting(testConfig, client.WithDialer(leakyDial), client.WithClock(clk))
	defer c.Close()

	var errs []error
	logs := captureLogs(t, func() {
		_, err := c.ListDir(context.Background(), "")
		errs = append(errs, err)
		_, err = c.ReadAt(context.Background(), "a.log", 0, 1) // during the backoff
		errs = append(errs, err)
	})

	assert.Contains(t, logs, "cannot connect to smb://myacct.file.core.windows.net/logs")
	assert.NotContains(t, logs, password)
	for _, err := range errs {
		require.Error(t, err)
		assert.Equal(t, client.ErrAuth, client.Classify(err))
		for _, verb := range []string{"%v", "%+v", "%#v"} {
			assert.NotContains(t, fmt.Sprintf(verb, err), password, verb)
		}
	}
	assert.Contains(t, errs[0].Error(), "myacct/********")

	for _, verb := range []string{"%v", "%+v", "%s", "%#v"} {
		assert.NotContains(t, fmt.Sprintf(verb, c), password, verb)
	}
}

func TestReconnectingLogsWithRealDialerNeverLeakPassword(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())

	cfg := client.Config{Host: "127.0.0.1", Port: port, Share: "logs", Username: "myacct", Password: password, DialTimeout: time.Second}
	c := client.NewReconnecting(cfg)
	defer c.Close()

	var callErr error
	logs := captureLogs(t, func() {
		_, callErr = c.ListDir(context.Background(), "")
	})
	require.Error(t, callErr)
	assert.Equal(t, client.ErrTransient, client.Classify(callErr))
	assert.NotContains(t, callErr.Error(), password)
	assert.Contains(t, logs, "cannot connect to smb://127.0.0.1:")
	assert.NotContains(t, logs, password)
}
