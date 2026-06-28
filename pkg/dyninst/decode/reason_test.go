// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"testing"
	"unsafe"

	"github.com/go-json-experiment/json/jsontext"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
)

// makeDataItem builds an output.DataItem with the given type id, reason
// bits, and failed-read mask state. Used to construct fixtures that
// exercise pickReason without going through a full eBPF capture.
func makeDataItem(
	typeID uint32,
	reason output.DataItemReason,
	failedRead bool,
	data []byte,
) output.DataItem {
	hdr := output.DataItemHeader{
		Type:    typeID & output.DataItemTypeMask,
		Length:  uint32(len(data)),
		Address: 0,
	}
	if failedRead {
		hdr.Type |= output.DataItemFailedReadMask
	}
	if reason != output.DataItemReasonNone {
		hdr.Type |= uint32(reason) << output.DataItemReasonShift
	}
	// DataItem stores an unexported header pointer; construct via the
	// public surface by reinterpreting a small struct with the same
	// layout. The decoder tests already do this trick to build
	// synthetic events; we replicate the pattern here.
	type publicDataItem struct {
		header *output.DataItemHeader
		data   []byte
	}
	priv := publicDataItem{header: &hdr, data: data}
	return *(*output.DataItem)(unsafe.Pointer(&priv))
}

// TestReasonTokenFor covers the static mapping from DataItemReason to
// JSON token. Forward-compat: unknown codes return (zero, false).
func TestReasonTokenFor(t *testing.T) {
	cases := []struct {
		name   string
		reason output.DataItemReason
		want   jsontext.Token
		wantOK bool
	}{
		{"none", output.DataItemReasonNone, jsontext.Token{}, false},
		{
			"tooManyPointersInFlight",
			output.DataItemReasonTooManyPointersInFlight,
			tokenNotCapturedReasonTooManyPointersInFlight, true,
		},
		{
			"tooManyUniquePointers",
			output.DataItemReasonTooManyUniquePointers,
			tokenNotCapturedReasonTooManyUniquePointers, true,
		},
		{
			"tooManySlicesCaptured",
			output.DataItemReasonTooManySlicesCaptured,
			tokenNotCapturedReasonTooManySlicesCaptured, true,
		},
		{
			"captureNestingTooDeep",
			output.DataItemReasonCaptureNestingTooDeep,
			tokenNotCapturedReasonCaptureNestingTooDeep, true,
		},
		{
			"valueTooLarge",
			output.DataItemReasonValueTooLarge,
			tokenNotCapturedReasonValueTooLarge, true,
		},
		{
			"stringSize",
			output.DataItemReasonStringSize,
			tokenNotCapturedReasonStringSize, true,
		},
		{
			"collectionSize",
			output.DataItemReasonCollectionSize,
			tokenNotCapturedReasonCollectionSize, true,
		},
		{
			"extended is reserved; decoder treats as unknown",
			output.DataItemReasonExtended,
			jsontext.Token{}, false,
		},
		{
			"future code 8 is unknown to the decoder",
			output.DataItemReason(8),
			jsontext.Token{}, false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := reasonTokenFor(tc.reason)
			require.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				require.Equal(t, tc.want.String(), got.String())
			}
		})
	}
}

// TestPickReasonPrefersItemBitsOverFallback verifies the highest-
// priority path in pickReason: a present data item carrying reason
// bits wins over the decoder's shape-inferred fallback.
func TestPickReasonPrefersItemBitsOverFallback(t *testing.T) {
	item := makeDataItem(7, output.DataItemReasonValueTooLarge, false, []byte("clamped"))
	got := pickReason(nil, &item, tokenNotCapturedReasonPruned)
	require.Equal(t, tokenNotCapturedReasonValueTooLarge.String(), got.String())
}

// TestPickReasonFallsBackOnAbsentItem verifies that when no data item
// is available and no peer scope cause is recorded, the helper returns
// the caller-supplied fallback (today's shape-inferred default).
func TestPickReasonFallsBackOnAbsentItem(t *testing.T) {
	got := pickReason(nil, nil, tokenNotCapturedReasonDepth)
	require.Equal(t, tokenNotCapturedReasonDepth.String(), got.String())
}

// TestPickReasonInheritsScopeCause verifies the peer-dedup path: a
// scope that has previously observed a placeholder peer with reason
// bits propagates the cause to subsequent absent peers.
func TestPickReasonInheritsScopeCause(t *testing.T) {
	scope := &peerScope{}
	// First peer is a failed-read placeholder carrying the cause.
	first := makeDataItem(7, output.DataItemReasonTooManyUniquePointers, true, nil)
	got := pickReason(scope, &first, tokenNotCapturedReasonDepth)
	require.Equal(t,
		tokenNotCapturedReasonTooManyUniquePointers.String(),
		got.String())

	// Subsequent absent peers in the same scope inherit the cause
	// even though they have no data item of their own.
	got = pickReason(scope, nil, tokenNotCapturedReasonDepth)
	require.Equal(t,
		tokenNotCapturedReasonTooManyUniquePointers.String(),
		got.String())
}

// TestPickReasonFirstMissWins verifies that once a scope records a
// cause it does not get overwritten by a subsequent peer's own reason
// bits. Different abandonment causes in the same scope are unusual in
// practice (the SM tends to hit one limit and stay at it), but the
// behavior here is "first observation wins" so the snapshot reader
// sees a single explanation per scope.
func TestPickReasonFirstMissWins(t *testing.T) {
	scope := &peerScope{}
	a := makeDataItem(7, output.DataItemReasonTooManyUniquePointers, true, nil)
	b := makeDataItem(7, output.DataItemReasonTooManyPointersInFlight, true, nil)
	_ = pickReason(scope, &a, tokenNotCapturedReasonDepth)
	got := pickReason(scope, nil, tokenNotCapturedReasonDepth)
	require.Equal(t,
		tokenNotCapturedReasonTooManyUniquePointers.String(),
		got.String(),
		"first-observed cause should propagate; second peer's "+
			"different reason should not overwrite it",
	)
	// A peer that has its own present-and-different reason bits still
	// wins for *that* peer (the scope cache only fills absent peers).
	got = pickReason(scope, &b, tokenNotCapturedReasonDepth)
	require.Equal(t,
		tokenNotCapturedReasonTooManyPointersInFlight.String(),
		got.String())
}

// TestPickReasonScopeNilSafe verifies that pickReason is safe to call
// from a non-iterating context (no scope yet established). The
// behavior degrades gracefully to "item bits or fallback".
func TestPickReasonScopeNilSafe(t *testing.T) {
	item := makeDataItem(7, output.DataItemReasonStringSize, false, []byte("xx"))
	require.Equal(t,
		tokenNotCapturedReasonStringSize.String(),
		pickReason(nil, &item, tokenNotCapturedReasonLength).String())
	require.Equal(t,
		tokenNotCapturedReasonDepth.String(),
		pickReason(nil, nil, tokenNotCapturedReasonDepth).String())
}

// TestPickReasonForFailedReadItem verifies the helper specialized for
// the "item exists with failed-read mask" case used at every pointer-
// chase / map-group / slice-data lookup site. Reason bits on the
// item should override the historical "unavailable" fallback.
func TestPickReasonForFailedReadItem(t *testing.T) {
	// No reason bits: stays as today's default.
	plain := makeDataItem(7, output.DataItemReasonNone, true, nil)
	require.Equal(t,
		tokenNotCapturedReasonUnavailable.String(),
		pickReasonForFailedReadItem(nil, plain).String())

	// Reason bits override the fallback.
	withReason := makeDataItem(
		7, output.DataItemReasonCaptureNestingTooDeep, true, nil,
	)
	require.Equal(t,
		tokenNotCapturedReasonCaptureNestingTooDeep.String(),
		pickReasonForFailedReadItem(nil, withReason).String())
}

// TestPickReasonForMissingItem verifies the helper used when no data
// item is in the capture for a chased pointee. Without a scope cache,
// today's shape-inferred fallback is returned unchanged; with a
// scope that has observed a cause, the scope cause wins.
func TestPickReasonForMissingItem(t *testing.T) {
	require.Equal(t,
		tokenNotCapturedReasonDepth.String(),
		pickReasonForMissingItem(nil, tokenNotCapturedReasonDepth).String())

	scope := &peerScope{cause: output.DataItemReasonTooManySlicesCaptured}
	require.Equal(t,
		tokenNotCapturedReasonTooManySlicesCaptured.String(),
		pickReasonForMissingItem(scope, tokenNotCapturedReasonDepth).String())
}

// TestPeerScopeWithPeerScope verifies the encodingContext helper that
// scopes a peerScope to the duration of a callback. Nested scopes are
// independent: a cause recorded in an inner scope does not leak to
// the outer scope after withPeerScope returns.
func TestPeerScopeWithPeerScope(t *testing.T) {
	ec := &encodingContext{}
	require.Nil(t, ec.currentPeerScope)

	err := ec.withPeerScope(func(outer *peerScope) error {
		outer.cause = output.DataItemReasonTooManyUniquePointers
		require.Equal(t, outer, ec.currentPeerScope)
		// Nested scope.
		return ec.withPeerScope(func(inner *peerScope) error {
			require.Equal(t, inner, ec.currentPeerScope)
			inner.cause = output.DataItemReasonValueTooLarge
			return nil
		})
	})
	require.NoError(t, err)
	require.Nil(t, ec.currentPeerScope,
		"withPeerScope should restore (eventually clear) the scope on exit")
}
