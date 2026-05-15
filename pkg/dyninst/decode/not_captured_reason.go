// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import "github.com/go-json-experiment/json/jsontext"

var (
	tokenNotCapturedReason = jsontext.String("notCapturedReason")

	// Existing tokens. Kept unchanged.
	tokenNotCapturedReasonLength         = jsontext.String("length")
	tokenNotCapturedReasonDepth          = jsontext.String("depth")
	tokenNotCapturedReasonCollectionSize = jsontext.String("collectionSize")
	tokenNotCapturedReasonPruned         = jsontext.String("pruned")
	tokenNotCapturedReasonUnavailable    = jsontext.String("unavailable")
	tokenNotCapturedReasonUnimplemented  = jsontext.String("unimplemented")
	tokenNotCapturedReasonCycle          = jsontext.String("circular reference")
	// This is used when we're missing the type information for a value
	// underneath an interface.
	tokenNotCapturedReasonMissingTypeInfo = jsontext.String("missing type information")

	// Per-value reasons stamped by the eBPF side on the data-item
	// header (either on a real captured item that got clamped, or on
	// a placeholder item with Length == 0 standing in for an omitted
	// chase).
	tokenNotCapturedReasonTooManyPointersInFlight = jsontext.String("tooManyPointersInFlight")
	tokenNotCapturedReasonTooManyUniquePointers   = jsontext.String("tooManyUniquePointers")
	tokenNotCapturedReasonTooManySlicesCaptured   = jsontext.String("tooManySlicesCaptured")
	tokenNotCapturedReasonCaptureNestingTooDeep   = jsontext.String("captureNestingTooDeep")
	tokenNotCapturedReasonValueTooLarge           = jsontext.String("valueTooLarge")
	tokenNotCapturedReasonStringSize              = jsontext.String("stringSize")

	tokenTruncated = jsontext.String("truncated")
)
