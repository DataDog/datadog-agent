// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package util

import (
	"errors"
	"fmt"
	"os"
	"testing"

	cebpf "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/features"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

const testTPBTFProg = "tp_btf__net__net_dev_queue"

func verifierError(prog string, cause error) error {
	return fmt.Errorf("program %s: load program: %w", prog, &cebpf.VerifierError{Cause: cause})
}

func TestRetryWithoutTPBTF(t *testing.T) {
	t.Cleanup(func() { NetDevQueueTPBTFRejected.Store(false) })

	for _, tc := range []struct {
		name      string
		firstErr  error
		rejected  bool
		wantCalls int
	}{
		{"EACCES", verifierError(testTPBTFProg, unix.EACCES), false, 2},
		{"EINVAL", verifierError(testTPBTFProg, unix.EINVAL), false, 2},
		{"ENOMEM", verifierError(testTPBTFProg, unix.ENOMEM), false, 1},
		{"EPERM", verifierError(testTPBTFProg, unix.EPERM), false, 1},
		{"other program", verifierError("raw_tracepoint__net__net_dev_queue", unix.EACCES), false, 1},
		{"not a verifier error", fmt.Errorf("program %s: %w", testTPBTFProg, unix.EACCES), false, 1},
		{"already rejected", verifierError(testTPBTFProg, unix.EACCES), true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			NetDevQueueTPBTFRejected.Store(tc.rejected)
			var calls int
			err := RetryWithoutTPBTF(testTPBTFProg, func() error {
				calls++
				if calls == 1 {
					return tc.firstErr
				}
				return nil
			})
			assert.Equal(t, tc.wantCalls, calls)
			if tc.wantCalls == 1 {
				assert.ErrorIs(t, err, tc.firstErr)
				assert.Equal(t, tc.rejected, NetDevQueueTPBTFRejected.Load())
				return
			}
			assert.NoError(t, err)
			assert.True(t, NetDevQueueTPBTFRejected.Load())
		})
	}
}

// TestRetryWithoutTPBTFRealRejection loads a program the verifier rejects, so a change to how
// cilium/ebpf reports collection load errors breaks this test rather than the fallback.
func TestRetryWithoutTPBTFRealRejection(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	if err := features.HaveProgramType(cebpf.Tracing); err != nil {
		t.Skipf("tracing programs not supported: %s", err)
	}
	t.Cleanup(func() { NetDevQueueTPBTFRejected.Store(false) })
	NetDevQueueTPBTFRejected.Store(false)

	spec := &cebpf.CollectionSpec{Programs: map[string]*cebpf.ProgramSpec{
		testTPBTFProg: {
			Type:       cebpf.Tracing,
			AttachType: cebpf.AttachTraceRawTp,
			AttachTo:   "net_dev_queue",
			// exit without setting R0
			Instructions: asm.Instructions{asm.Return()},
			License:      "GPL",
		},
	}}

	var calls int
	var firstErr error
	err := RetryWithoutTPBTF(testTPBTFProg, func() error {
		calls++
		if calls > 1 {
			return nil
		}
		coll, err := cebpf.NewCollection(spec)
		if err == nil {
			coll.Close()
			return errors.New("verifier accepted a program without a return value")
		}
		firstErr = err
		return err
	})
	require.NoError(t, err, "first load: %v", firstErr)
	assert.Equal(t, 2, calls)
	assert.True(t, NetDevQueueTPBTFRejected.Load())
}
