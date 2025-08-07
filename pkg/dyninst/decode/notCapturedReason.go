// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import "github.com/go-json-experiment/json/jsontext"

var (
	tokenNotCapturedReason = jsontext.String("tokenNotCapturedReason")

	tokenNotCapturedReasonLength         = jsontext.String("length")
	tokenNotCapturedReasonDepth          = jsontext.String("depth")
	tokenNotCapturedReasonCollectionSize = jsontext.String("collectionSize")
	tokenNotCapturedReasonPruned         = jsontext.String("pruned")
	tokenNotCapturedReasonUnavailable    = jsontext.String("unavailable")
	tokenNotCapturedReasonUnimplemented  = jsontext.String("unimplemented")
	tokenNotCapturedReasonCycle          = jsontext.String("circular reference")
	// tokenNotCapturedReasonFieldCount     = jsontext.String("fieldCount")

	tokenTruncated = jsontext.String("truncated")
)
