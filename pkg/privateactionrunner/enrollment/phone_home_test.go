// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package enrollment

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/stretchr/testify/require"
)

func TestPhoneHomeMutationClaimAndStaleOutcome(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest(setup.PARIdentityFilePath, filepath.Join(t.TempDir(), "identity.json"))
	cfg.SetInTest("api_key", "credential-not-in-journal")
	now := time.Now()
	a, err := ReservePhoneHomeAttempt(cfg, "host", now, nil)
	require.NoError(t, err)
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if claimPhoneHomePOST(cfg, a) == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	require.EqualValues(t, 1, successes.Load())
	require.Error(t, claimPhoneHomePOST(cfg, a))
	require.NoError(t, RecordPhoneHomeOutcome(cfg, PhoneHomeOutcome{AttemptID: a.ID, EnrollmentFailure: opms.EnrollmentFailure{Category: "quota", RetrySafe: true}}))
	a.NextAttemptAt = now.Add(time.Hour)
	require.NoError(t, SavePhoneHomeSchedule(cfg, a))
	_, err = ReservePhoneHomeAttempt(cfg, "host", now.Add(time.Minute), a)
	require.Error(t, err)
	b, err := ReservePhoneHomeAttempt(cfg, "host", now.Add(time.Hour), a)
	require.NoError(t, err)
	require.NotEqual(t, a.ID, b.ID)
	require.ErrorIs(t, RecordPhoneHomeOutcome(cfg, PhoneHomeOutcome{AttemptID: a.ID}), ErrStalePhoneHomeAttempt)
	require.ErrorIs(t, SavePhoneHomeSchedule(cfg, a), ErrStalePhoneHomeAttempt)
	for _, name := range []string{"current.json", filepath.Join(a.ID, "outcome.json")} {
		data, err := os.ReadFile(filepath.Join(PhoneHomeAttemptPath(cfg), name))
		require.NoError(t, err)
		require.NotContains(t, string(data), "credential-not-in-journal")
		require.NotContains(t, string(data), "private_key")
		require.NotContains(t, string(data), "config_hash")
	}
}

func TestPhoneHomeProtectedPendingAndMemoryRetention(t *testing.T) {
	t.Setenv(app.PhoneHomePOCEnvVar, "true")
	cfg := configmock.New(t)
	cfg.SetInTest(setup.PARIdentityFilePath, filepath.Join(t.TempDir(), "identity.json"))
	a, err := ReservePhoneHomeAttempt(cfg, "host", time.Now(), nil)
	require.NoError(t, err)
	private, _, err := util.GenerateKeys()
	require.NoError(t, err)
	encoded, err := private.MarshalJSON()
	require.NoError(t, err)
	// Use the same protected serialization as the actual enroller.
	p, err := readPendingIdentity(cfg, a)
	require.NoError(t, err)
	p.PrivateKey = base64.RawURLEncoding.EncodeToString(encoded)
	p.URN = util.MakeRunnerURN("us1", 42, "runner")
	held := &PendingPersistence{pending: p}
	path := pendingIdentityPath(cfg, a.ID)
	require.NoError(t, os.Remove(path))
	require.NoError(t, os.Mkdir(path, 0700))
	require.Error(t, held.Retain(cfg))
	require.NotContains(t, held.Error(), p.PrivateKey)
	require.NoError(t, os.Remove(path))
	require.NoError(t, held.Retain(cfg))
	saved, err := readPendingIdentity(cfg, a)
	require.NoError(t, err)
	require.Equal(t, p, saved)
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	outcome, err := ReadPhoneHomeOutcome(cfg, a)
	require.NoError(t, err)
	require.True(t, outcome.PersistencePending)
	require.False(t, outcome.HelperRetained)
	require.NoError(t, RecoverPhoneHomeIdentity(context.Background(), cfg, "host"))
	identity, err := GetIdentityFromPreviousEnrollment(context.Background(), cfg)
	require.NoError(t, err)
	require.Equal(t, p.PrivateKey, identity.PrivateKey)
	require.Equal(t, p.URN, identity.URN)
}
