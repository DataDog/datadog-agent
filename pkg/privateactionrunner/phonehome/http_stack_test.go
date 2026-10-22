// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && !windows

package phonehome

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/procmgr"
	"github.com/stretchr/testify/require"
)

// Uses the real RC protobuf fetch, AAA HTTP client and production gate composition.
// Only supervisor RPCs are fake; this is not live process/enrollment evidence.
func stackController(t *testing.T, f *scopeFixture) (*controller, *supervisor) {
	t.Helper()
	f.cfg.SetInTest(par.IdentityFilePath, filepath.Join(t.TempDir(), "identity.json"))
	pm := &supervisor{
		control:  &pb.ProcessDetail{Uuid: "control", State: pb.ProcessState_SKIPPED, RestartPolicy: "never"},
		executor: &pb.ProcessDetail{Uuid: "executor", State: pb.ProcessState_SKIPPED, RestartPolicy: "never"},
	}
	return &controller{cfg: f.cfg, source: NewScopedSource(f.s.rc, f.s), pm: pm, now: f.clock.Now, emit: func(State) {}}, pm
}

func TestHTTPStackEveryStartRequiresRCAndPARScope(t *testing.T) {
	for _, saved := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			for _, authorized := range []bool{false, true} {
				for _, granted := range []bool{false, true} {
					t.Run(fmt.Sprintf("saved=%t/rc=%t,%t/par=%t", saved, enabled, authorized, granted), func(t *testing.T) {
						f := newScopeFixture(t)
						c, pm := stackController(t, f)
						identity := f.cfg.GetString(par.IdentityFilePath)
						if saved {
							require.NoError(t, os.WriteFile(identity, []byte("existing identity is owned by PAR, not the gate"), 0600))
						}
						if !granted {
							f.mu.Lock()
							f.body = `{"data":{"attributes":{"valid":true,"api_key_scopes":[]}}}`
							f.mu.Unlock()
						}
						c.step(context.Background())
						f.step()
						require.Zero(t, f.aaaGETs.Load(), "unknown RC cannot dispatch AAA")
						f.rc(enabled, authorized)
						c.step(context.Background())
						f.step()
						c.step(context.Background())
						if enabled && authorized {
							require.EqualValues(t, 1, f.aaaGETs.Load())
						} else {
							require.Zero(t, f.aaaGETs.Load())
						}
						if enabled && authorized && granted {
							require.Equal(t, 1, pm.starts)
						} else {
							require.Zero(t, pm.starts)
						}
						if saved {
							b, err := os.ReadFile(identity)
							require.NoError(t, err)
							require.Equal(t, "existing identity is owned by PAR, not the gate", string(b))
						} else {
							require.NoFileExists(t, identity)
						}
					})
				}
			}
		}
	}
}

func TestHTTPStackLaterScopeGrantStartsWithoutCoreRestart(t *testing.T) {
	f := newScopeFixture(t)
	c, pm := stackController(t, f)
	f.rc(true, true)
	f.mu.Lock()
	f.body = `{"data":{"attributes":{"valid":true,"api_key_scopes":[]}}}`
	f.mu.Unlock()
	c.step(context.Background())
	f.step()
	c.step(context.Background())
	require.Zero(t, pm.starts)
	f.mu.Lock()
	f.body = grantedScopeJSON
	f.mu.Unlock()
	f.advance(f.s.GetPARScope().NextCheck.Sub(f.clock.Now()))
	f.step()
	c.step(context.Background())
	require.Equal(t, 1, pm.starts)
	c.step(context.Background())
	require.Equal(t, 1, pm.starts, "controller cannot trigger a second enrollment by relaunching")
}

func TestHTTPStackRejectsGateLossBeforeStartAndExpiredScope(t *testing.T) {
	for _, change := range []string{"rc", "expiry"} {
		t.Run(change, func(t *testing.T) {
			f := newScopeFixture(t)
			c, pm := stackController(t, f)
			f.rc(true, true)
			c.step(context.Background())
			f.step()
			source := c.source
			reads := 0
			c.source = sourceFunc(func(now time.Time) Observation {
				reads++
				if reads == 2 {
					if change == "rc" {
						f.rc(false, true)
					} else {
						f.clock.Add(3 * time.Minute)
						now = f.clock.Now()
					}
				}
				return source.Read(now)
			})
			c.step(context.Background())
			require.Zero(t, pm.starts)
		})
	}
}

func TestHTTPStackOutageRetainsLastSuccessfulRCGate(t *testing.T) {
	f := newScopeFixture(t)
	c, pm := stackController(t, f)
	f.rc(true, true)
	f.mu.Lock()
	f.rcCode = 503
	f.mu.Unlock()
	f.rc(false, false) // Failed fetch must retain the previous successful pair.
	c.step(context.Background())
	f.step()
	c.step(context.Background())
	require.EqualValues(t, 1, f.aaaGETs.Load())
	require.Equal(t, 1, pm.starts)
}

func TestHTTPStackMissingProducerFailsClosed(t *testing.T) {
	f := newScopeFixture(t)
	c, pm := stackController(t, f)
	f.rc(true, true)
	c.source = NewScopedSource(f.s.rc, nil)
	c.step(context.Background())
	require.Zero(t, pm.starts)
	require.Zero(t, f.aaaGETs.Load())
}
