// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && smb && !goexperiment.systemcrypto && !goexperiment.boringcrypto && !requirefips

package smb

// Tests of the logons of the SMB sources: a server that refuses an account's
// password is asked once per password, whatever the number of sources and of
// servers, and the status of each source says so.

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline/mock"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditorMock "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/client"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/client/fake"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// logonLauncher returns a launcher whose dials are SESSION_SETUPs on the servers
// of reply, counted, and a function that starts a scanner of a source of the
// domain account CORP\svc-logs, with the password, on the given host and share.
func logonLauncher(t *testing.T, reply func(cfg client.Config) error) (l *Launcher, clk *clock.Mock, setups *atomic.Int32, add func(host, share, password string, opts ...func(*config.LogsConfig)) (*scanner, *sources.LogSource)) {
	t.Helper()
	configmock.New(t)
	share := fake.New()
	share.Mkdir("app")
	share.Write("app/app.log", []byte(lines(1, 1)))
	clk = clock.NewMock()
	l = newTestLauncher(share, clk)
	provider := mock.NewMockProvider()
	l.pipelineProvider = provider
	l.registry = auditorMock.NewMockRegistry()
	newCollector(t, provider.NextPipelineChan(), nil) // receives what the tailers forward, so that they can stop
	setups = new(atomic.Int32)
	l.dial = func(ctx context.Context, cfg client.Config) (client.Client, error) {
		setups.Add(1)
		if err := reply(cfg); err != nil {
			return nil, err
		}
		return share.Dial(ctx, cfg)
	}
	add = func(host, share, password string, opts ...func(*config.LogsConfig)) (*scanner, *sources.LogSource) {
		source := newSMBSource("smb-"+host+"-"+share, append([]func(*config.LogsConfig){func(c *config.LogsConfig) {
			c.SMB.Host, c.SMB.Share, c.SMB.Username, c.SMB.Password, c.SMB.Domain = host, share, "svc-logs", password, "CORP"
		}}, opts...)...)
		require.NoError(t, source.Config.Validate())
		key, c := l.acquireClient(source.Config.SMB)
		s, err := newScanner(l, source, c, key)
		require.NoError(t, err)
		t.Cleanup(func() {
			s.stopTailers()
			l.releaseClient(key)
		})
		return s, source
	}
	return l, clk, setups, add
}

func refusedLogon(cfg client.Config) error {
	// The server's message names the account, never the password; the error
	// carries it anyway to check that nothing prints it.
	return fmt.Errorf("session setup for %s with %s: %w", cfg.Username, cfg.Password, fake.ErrAuth)
}

// TestOneRefusedLogonForManySourcesAndServers: the server refuses the password,
// and the Agent sends exactly one logon for the account, whatever the number of
// sources and of servers that share it, and however they race.
func TestOneRefusedLogonForManySourcesAndServers(t *testing.T) {
	_, clk, setups, add := logonLauncher(t, refusedLogon)
	type entry struct {
		s      *scanner
		source *sources.LogSource
		target string
	}
	var entries []entry
	for _, host := range []string{"dc1.corp.example.com", "dc2.corp.example.com", "dc3.corp.example.com"} {
		for _, share := range []string{"logs", "audit", "app", "web"} {
			s, source := add(host, share, testPassword)
			entries = append(entries, entry{s, source, "smb://" + host + "/" + share})
		}
	}

	var wg sync.WaitGroup
	for _, e := range entries {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.s.scan(context.Background())
		}()
	}
	wg.Wait()
	assert.EqualValues(t, 1, setups.Load(), "one SESSION_SETUP for %d sources on 3 servers", len(entries))

	for range 3 {
		clk.Add(31 * time.Second)
		for _, e := range entries {
			e.s.scan(context.Background())
		}
	}
	clk.Add(48 * time.Hour)
	for _, e := range entries {
		e.s.scan(context.Background())
	}
	assert.EqualValues(t, 1, setups.Load(), "and no other, for as long as the password stays the same")

	for _, e := range entries {
		require.True(t, e.source.Status().IsError())
		msg := e.source.Status().GetError()
		assert.Contains(t, msg, e.target, "each status names its own share")
		assert.Contains(t, msg, "rejected the credentials")
		assert.Contains(t, msg, "stopped sending logons")
		assert.Contains(t, msg, "Fix the password, then refresh the secret or restart the Agent")
		assert.NotContains(t, msg, testPassword)
		for _, other := range entries {
			if other.target != e.target {
				assert.NotContains(t, msg, other.target, "the status of %s names no other share", e.target)
			}
		}
	}
}

// TestNewPasswordDialsAgain: a source whose secret was refreshed has a new
// password, and its account is asked once more; the old password stays refused.
func TestNewPasswordDialsAgain(t *testing.T) {
	_, clk, setups, add := logonLauncher(t, func(cfg client.Config) error {
		if cfg.Password == "new-Passw0rd-1" {
			return nil
		}
		return refusedLogon(cfg)
	})
	oldS, oldSource := add("dc1.corp.example.com", "logs", testPassword)
	oldS.scan(context.Background())
	require.True(t, oldSource.Status().IsError())
	assert.EqualValues(t, 1, setups.Load())

	fixedS, fixedSource := add("dc1.corp.example.com", "logs", "new-Passw0rd-1")
	fixedS.scan(context.Background())
	assert.EqualValues(t, 2, setups.Load(), "the new password is tried")
	assert.True(t, fixedSource.Status().IsSuccess())

	clk.Add(time.Hour)
	oldS.scan(context.Background())
	assert.EqualValues(t, 2, setups.Load(), "the old password is not tried again")
	assert.True(t, oldSource.Status().IsError())
}

// TestStoppedLogonStatus: the status of a source whose account the server refused
// says why the Agent stopped, how to recover, and shows no credential.
func TestStoppedLogonStatus(t *testing.T) {
	_, clk, setups, add := logonLauncher(t, refusedLogon)
	s, source := add("files.example.com", "logs", testPassword)
	s.scan(context.Background())
	require.True(t, source.Status().IsError())
	msg := source.Status().GetError()
	assert.Contains(t, msg, "smb://files.example.com/logs")
	assert.Contains(t, msg, "the server rejected the credentials")
	assert.Contains(t, msg, "Fix the password")
	assert.NotContains(t, msg, testPassword)
	assert.NotContains(t, msg, "svc-logs", "no user name either")

	clk.Add(31 * time.Second)
	s.scan(context.Background())
	assert.EqualValues(t, 1, setups.Load())
	assert.Equal(t, msg, source.Status().GetError())
}

// TestLockedOutStatus: a locked-out account says that the Agent tries again, once
// an hour.
func TestLockedOutStatus(t *testing.T) {
	_, clk, setups, add := logonLauncher(t, func(client.Config) error {
		return fmt.Errorf("session setup: %w", fake.ErrLockedOut)
	})
	s, source := add("files.example.com", "logs", testPassword)
	s.scan(context.Background())
	msg := source.Status().GetError()
	assert.Contains(t, msg, "locked out")
	assert.Contains(t, msg, "once an hour")
	assert.NotContains(t, msg, testPassword)
	for range 5 {
		clk.Add(10 * time.Minute)
		s.scan(context.Background())
	}
	assert.EqualValues(t, 1, setups.Load(), "no probe before the hour")
	clk.Add(30 * time.Minute) // 80 minutes: past the hour plus 10%
	s.scan(context.Background())
	assert.EqualValues(t, 2, setups.Load(), "one probe")
}

// TestOneRefusedLogonForManyServersWithNoDomain: a source with no domain lets the
// server's challenge choose it, which is the same domain whatever the server is
// called, so the servers share one account: one logon for all of them.
func TestOneRefusedLogonForManyServersWithNoDomain(t *testing.T) {
	_, _, setups, add := logonLauncher(t, refusedLogon)
	noDomain := func(c *config.LogsConfig) { c.SMB.Domain = "" }
	var scanners []*scanner
	for _, host := range []string{"fs1.example.com", "fs2.example.com", "fs1", "10.0.0.7"} {
		s, _ := add(host, "logs", testPassword, noDomain)
		scanners = append(scanners, s)
	}
	for _, s := range scanners {
		s.scan(context.Background())
	}
	assert.EqualValues(t, 1, setups.Load(), "one SESSION_SETUP for 4 servers")
}

// TestAccountStateStatus: a disabled account is not fixed by a new password, and
// its status says what is: enable the account, then restart the Agent.
func TestAccountStateStatus(t *testing.T) {
	_, _, _, add := logonLauncher(t, func(client.Config) error {
		return fmt.Errorf("session setup: %w", fake.ErrDisabled)
	})
	s, source := add("files.example.com", "logs", testPassword)
	s.scan(context.Background())
	msg := source.Status().GetError()
	assert.Contains(t, msg, "the account is disabled")
	assert.Contains(t, msg, "Enable the account, then restart the Agent")
	assert.NotContains(t, msg, "Fix the password")
	assert.NotContains(t, msg, testPassword)
}
