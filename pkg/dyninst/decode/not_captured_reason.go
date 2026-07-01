// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import "github.com/go-json-experiment/json/jsontext"

var (
	tokenNotCapturedReason = jsontext.String("notCapturedReason")

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
	// tokenNotCapturedReasonFieldCount      = jsontext.String("fieldCount")

	// redactedIdent is used when a value is dropped because the name of the
	// variable, field, or string map key holding it matches a redacted
	// identifier. redactedType is used when a value is dropped because its type
	// matches a redacted type. These mirror the Java, .NET, and Python tracers.
	tokenNotCapturedReasonRedactedIdent = jsontext.String("redactedIdent")
	tokenNotCapturedReasonRedactedType  = jsontext.String("redactedType")

	tokenTruncated = jsontext.String("truncated")
)
