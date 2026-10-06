// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package enrollment

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/stretchr/testify/require"
)

func TestPhoneHomeMutationClaim(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest(setup.PARIdentityFilePath, filepath.Join(t.TempDir(), "identity.json"))
	require.Error(t, ClaimPhoneHomePOST(cfg), "executor cannot enroll without core's launch latch")
	require.NoError(t, os.Mkdir(PhoneHomeAttemptPath(cfg), 0700))
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ClaimPhoneHomePOST(cfg) == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	require.EqualValues(t, 1, successes.Load())
	require.Error(t, ClaimPhoneHomePOST(cfg), "claim survives a new caller")
	data, err := os.ReadFile(filepath.Join(PhoneHomeAttemptPath(cfg), "post"))
	require.NoError(t, err)
	require.Empty(t, data, "no new credential cache")
	cfg.SetInTest(setup.PARIdentityFilePath, "relative.json")
	require.Error(t, ClaimPhoneHomePOST(cfg))
}
