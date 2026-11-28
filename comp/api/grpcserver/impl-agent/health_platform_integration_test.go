// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package agentimpl

import (
	"context"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	healthplatformstore "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	storefx "github.com/DataDog/datadog-agent/comp/healthplatform/store/fx"
	storeimpl "github.com/DataDog/datadog-agent/comp/healthplatform/store/impl"
	remotenoopfx "github.com/DataDog/datadog-agent/comp/healthplatform/store/remote/fx-noop"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// slowStoreSave is how long every save of the store takes in TestHealthIssueRPCsDoNotWaitForSlowStore: longer than
// the store's 10 s persist interval, so a save is still running when the next one is due
const slowStoreSave = 30 * time.Second

// TestHealthIssueRPCsDoNotWaitForSlowStore checks the entry point of remote reporters, such as system-probe, against
// the real store: while every save takes slowStoreSave, ReportHealthIssue and ResolveHealthIssue return without
// waiting, and the store applies what they sent. It runs in a testing/synctest bubble, where time only moves while
// every goroutine is blocked, so an RPC that waited on the save on a timer or a channel shows as elapsed time. A
// regression making it wait on a mutex held during the save would hang the test instead, as a goroutine blocked on a
// mutex keeps the bubble clock from advancing.
func TestHealthIssueRPCsDoNotWaitForSlowStore(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		saves := storeimpl.SetSaveDelay(t, slowStoreSave)
		store := fxutil.Test[healthplatformstore.Component](t,
			storefx.Module(),
			remotenoopfx.Module(),
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component {
				cfg := config.NewMock(t)
				cfg.SetInTest("health_platform.enabled", true)
				cfg.SetInTest("health_platform.persist_on_kubernetes", true)
				cfg.SetInTest("run_path", t.TempDir())
				return cfg
			}),
			telemetrymock.Module(),
			hostnameinterface.MockModule(),
		)
		// Registered after fxutil.Test, so it runs before the stop, also on a failure: let the stuck save finish and make
		// the final one fast, so the stop meets its deadline.
		t.Cleanup(func() {
			saves.SpeedUp()
			time.Sleep(slowStoreSave)
		})
		srv := serverWithStore(store)

		const reports = 35
		for i := range reports {
			id := fmt.Sprintf("remote:%d", i)
			start := time.Now()
			_, err := srv.ReportHealthIssue(context.Background(), &pb.ReportHealthIssueRequest{
				Issue: &healthplatformpayload.Issue{Id: id, IssueName: "remote-test", Title: "Remote issue " + id},
			})
			require.NoError(t, err)
			require.Zero(t, time.Since(start), "ReportHealthIssue for %s waited for the store", id)
			if i%2 == 1 {
				if i%4 == 1 {
					// the report landed, so a missing issue below means the resolution was applied, not that both
					// were dropped
					synctest.Wait()
					require.NotNil(t, store.GetIssue(id), "the report of %s was not applied", id)
				}
				// otherwise the resolution is queued right behind its report, as a remote reporter can send them
				start = time.Now()
				_, err = srv.ResolveHealthIssue(context.Background(), &pb.ResolveHealthIssueRequest{IssueId: id})
				require.NoError(t, err)
				require.Zero(t, time.Since(start), "ResolveHealthIssue for %s waited for the store", id)
			}
			time.Sleep(time.Second)
		}
		require.Positive(t, saves.Started(), "no slow save started while the RPCs came in")

		synctest.Wait()
		for i := range reports {
			id := fmt.Sprintf("remote:%d", i)
			if i%2 == 1 {
				assert.Nil(t, store.GetIssue(id), "the resolution of %s was not applied", id)
			} else {
				assert.NotNil(t, store.GetIssue(id), "the report of %s was not applied", id)
			}
		}
	})
}
