// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package offsetguess

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ebpf/maps"
)

// TestCheckAndUpdateCurrentOffsetRTT drives the offset guessing state machine
// for the RTT stages against synthetic tcp_sock memory, mirroring the eBPF
// side of offset-guess.c (aligned reads at the candidate offsets).
//
// On kernels <= 6.9 (without the tcp_sock cacheline reorg), srtt_us and
// mdev_us are adjacent u32s. RTT-scale false positives (e.g. rtt_min,
// rcv_rtt_est ~20 bytes before srtt_us on aarch64) can match the lossy
// Rtt>>3 == tcpi_rtt comparison, so the guessed offset_rtt can lock onto the
// wrong field. The state machine must reject such a pair (non-adjacent
// rtt/rtt_var) and resume guessing instead of accepting it.
func TestCheckAndUpdateCurrentOffsetRTT(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("skipping: requires root to create eBPF maps")
	}
	// kernels < 5.11 account BPF memory against RLIMIT_MEMLOCK; the map
	// creation below fails with EPERM unless the limit is raised
	require.NoError(t, rlimit.RemoveMemlock())

	const (
		memSize = 8192

		// struct tcp_sock layout of the kernels where offset guessing runs:
		// srtt_us followed by the adjacent mdev_us, and another RTT-scaled
		// field 20 bytes before srtt_us (see the aarch64 false positives
		// behind the TestOffsetGuess flake).
		srttOff uint64 = 1600
		mdevOff uint64 = srttOff + 4
		fpOff   uint64 = srttOff - 20

		// 424>>3 == 53 == expected.rtt, 40>>2 == 10 == expected.rttVar
		srttVal uint32 = 424
		fpVal   uint32 = 424
		mdevVal uint32 = 40
	)

	expected := &fieldValues{rtt: 53, rttVar: 10}

	readU32 := func(mem []byte, off uint64) uint32 {
		// mirror bpf_probe_read_kernel beyond the (synthetic) struct: faults
		// leave the value at 0
		if off+4 > uint64(len(mem)) {
			return 0
		}
		return binary.LittleEndian.Uint32(mem[off:])
	}

	// simKprobe mirrors what the offset guessing eBPF program does on each
	// kprobe event: when the state is STATE_CHECKING, read the candidate
	// offsets (aligned to the field size) from tcp_sock memory and store the
	// values back in the map with STATE_CHECKED.
	simKprobe := func(mp *maps.GenericMap[uint64, TracerStatus], mem []byte) error {
		var zero uint64
		var st TracerStatus
		if err := mp.Lookup(&zero, &st); err != nil {
			return fmt.Errorf("simKprobe: error reading tracer_status: %w", err)
		}
		if State(st.State) != StateChecking {
			return nil
		}
		switch GuessWhat(st.What) {
		case GuessRTT:
			st.Offset_rtt = (st.Offset_rtt + 3) &^ 3 // aligned_offset
			st.Rtt = readU32(mem, st.Offset_rtt)
		case GuessRTTVar:
			st.Offset_rtt_var = (st.Offset_rtt_var + 3) &^ 3
			st.Rtt_var = readU32(mem, st.Offset_rtt_var)
		default:
			return nil
		}
		st.State = uint64(StateChecked)
		return mp.Put(&zero, &st)
	}

	// runRTTGuessing runs the RTT stages of the Guess() driver loop against
	// mem, returning the guessed offsets (or the overflow error the driver
	// loop raises when a scan exceeds thresholdInetSock). It takes the
	// subtest's t so require failures are attributed to the right test.
	runRTTGuessing := func(t *testing.T, mem []byte) (*TracerStatus, error) {
		mp, err := maps.NewGenericMap[uint64, TracerStatus](&ebpf.MapSpec{
			Name:       "tracer_status_test",
			Type:       ebpf.Hash,
			MaxEntries: 1,
		})
		require.NoError(t, err)
		defer mp.Map().Close()

		guesser := &tracerOffsetGuesser{status: &TracerStatus{
			State:      uint64(StateChecking),
			What:       uint64(GuessRTT),
			Offset_rtt: rttDefaultOffsetBytes,
		}}
		var zero uint64
		require.NoError(t, mp.Put(&zero, guesser.status))

		maxRetries := 100
		for i := 0; i < 5000; i++ {
			switch GuessWhat(guesser.status.What) {
			case GuessRTT, GuessRTTVar:
			default:
				return guesser.status, nil
			}
			require.NoError(t, simKprobe(mp, mem))
			if err := guesser.checkAndUpdateCurrentOffset(mp, expected, &maxRetries, 3000); err != nil {
				return guesser.status, err
			}
			// same overflow guard as the driver loop in Guess()
			if guesser.status.Offset_rtt >= thresholdInetSock {
				return guesser.status, fmt.Errorf("overflow while guessing %v, bailing out", whatString[GuessWhat(guesser.status.What)])
			}
		}
		return guesser.status, errors.New("rtt guessing did not terminate")
	}

	t.Run("rejects false positive rtt offset", func(t *testing.T) {
		// another RTT-scaled field 20 bytes before srtt_us matches the lossy
		// Rtt>>3 comparison; the true mdev_us sits adjacent to srtt_us
		mem := make([]byte, memSize)
		binary.LittleEndian.PutUint32(mem[fpOff:], fpVal)
		binary.LittleEndian.PutUint32(mem[srttOff:], srttVal)
		binary.LittleEndian.PutUint32(mem[mdevOff:], mdevVal)

		status, err := runRTTGuessing(t, mem)
		require.NoError(t, err)

		// the false positive at fpOff must be rejected: the scan must resume
		// before the mdev_us match and lock onto the true srtt_us
		assert.Equal(t, GuessSocketSK, GuessWhat(status.What))
		assert.Equal(t, srttOff, status.Offset_rtt, "unexpected offset_rtt")
		assert.Equal(t, mdevOff, status.Offset_rtt_var, "unexpected offset_rtt_var")
	})

	t.Run("no false positive", func(t *testing.T) {
		mem := make([]byte, memSize)
		binary.LittleEndian.PutUint32(mem[srttOff:], srttVal)
		binary.LittleEndian.PutUint32(mem[mdevOff:], mdevVal)

		status, err := runRTTGuessing(t, mem)
		require.NoError(t, err)

		assert.Equal(t, GuessSocketSK, GuessWhat(status.What))
		assert.Equal(t, srttOff, status.Offset_rtt, "unexpected offset_rtt")
		assert.Equal(t, mdevOff, status.Offset_rtt_var, "unexpected offset_rtt_var")
	})

	t.Run("terminates when no adjacent pair exists", func(t *testing.T) {
		// false positive rtt match, and a rtt_var match that is not adjacent
		// to it and not preceded by any valid srtt_us: the backtracking must
		// keep advancing the scan and hit the driver loop overflow guard
		// instead of looping forever.
		mem := make([]byte, memSize)
		binary.LittleEndian.PutUint32(mem[fpOff:], fpVal)
		binary.LittleEndian.PutUint32(mem[fpOff+8:], mdevVal) // rtt_var match at fp+8, non-adjacent

		_, err := runRTTGuessing(t, mem)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "overflow while guessing")
	})
}
