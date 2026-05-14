// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"github.com/go-json-experiment/json/jsontext"

	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
)

// reasonTokenFor maps a DataItemReason carried on a data-item header to
// the corresponding snapshot JSON token, plus an ok flag set to false
// for DataItemReasonNone (the eBPF side did not classify this item)
// and for codes the decoder does not understand (forward-compat).
func reasonTokenFor(r output.DataItemReason) (jsontext.Token, bool) {
	switch r {
	case output.DataItemReasonTooManyPointersInFlight:
		return tokenNotCapturedReasonTooManyPointersInFlight, true
	case output.DataItemReasonTooManyUniquePointers:
		return tokenNotCapturedReasonTooManyUniquePointers, true
	case output.DataItemReasonTooManySlicesCaptured:
		return tokenNotCapturedReasonTooManySlicesCaptured, true
	case output.DataItemReasonCaptureNestingTooDeep:
		return tokenNotCapturedReasonCaptureNestingTooDeep, true
	case output.DataItemReasonValueTooLarge:
		return tokenNotCapturedReasonValueTooLarge, true
	case output.DataItemReasonStringSize:
		return tokenNotCapturedReasonStringSize, true
	case output.DataItemReasonCollectionSize:
		return tokenNotCapturedReasonCollectionSize, true
	}
	return jsontext.Token{}, false
}

// peerScope carries the "presumed cause" through the decoder's
// iteration of a slice, array, map, or struct. The first peer in the
// scope that exposes reason bits seeds the cause; subsequent absent
// peers fall back to it. Nested scopes are independent — pushing a new
// scope on enter (e.g. into a struct field's pointee) and popping on
// exit ensures peer-dedup does not leak across unrelated parents.
type peerScope struct {
	// cause is the data-item reason that has been observed in this scope.
	// Zero (DataItemReasonNone) means "no peer in this scope has carried
	// a reason yet" — i.e. the decoder should fall back to its
	// shape-inferred default.
	cause output.DataItemReason
}

// recordCause sets the scope's presumed cause if one has not yet been
// observed. First-miss-wins: subsequent peers with different reasons
// do not overwrite. Returns the current cause after recording.
func (s *peerScope) recordCause(r output.DataItemReason) output.DataItemReason {
	if s == nil {
		return output.DataItemReasonNone
	}
	if s.cause == output.DataItemReasonNone {
		s.cause = r
	}
	return s.cause
}

// pickReason returns the best available notCapturedReason token for a
// peer at the current iteration scope:
//
//  1. If the peer's data item exists and carries reason bits, use the
//     corresponding token and record the cause in the scope so absent
//     siblings can inherit it.
//  2. Else, if the scope has a remembered cause (a prior peer in this
//     scope exposed reason bits), use that.
//  3. Else, return the shape-inferred fallback.
//
// item may be nil when called on an absent peer (no data item in the
// capture map).
func pickReason(scope *peerScope, item *output.DataItem, fallback jsontext.Token) jsontext.Token {
	if item != nil {
		if tok, ok := reasonTokenFor(item.Reason()); ok {
			if scope != nil {
				scope.recordCause(item.Reason())
			}
			return tok
		}
	}
	if scope != nil && scope.cause != output.DataItemReasonNone {
		if tok, ok := reasonTokenFor(scope.cause); ok {
			return tok
		}
	}
	return fallback
}

// pickReasonForFailedReadItem is the common path used by decoder types
// that look up a chased pointee in the data-items map, find it, but
// then observe DataItemFailedReadMask is set (either a kernel-read
// failure or a placeholder for an abandoned chase). It prefers any
// reason bits the data-item header carries over the "unavailable"
// fallback.
func pickReasonForFailedReadItem(scope *peerScope, item output.DataItem) jsontext.Token {
	return pickReason(scope, &item, tokenNotCapturedReasonUnavailable)
}

// pickReasonForMissingItem is the path used when the data-items map
// has no entry at all for a chased pointee. Falls back to the
// shape-inferred default; only the scope's remembered cause (set by a
// prior peer that did expose reason bits) can override it.
func pickReasonForMissingItem(scope *peerScope, fallback jsontext.Token) jsontext.Token {
	return pickReason(scope, nil, fallback)
}
