// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import "github.com/go-json-experiment/json/jsontext"

var (
	notCapturedReason jsontext.Token = jsontext.String("notCapturedReason")

	notCapturedReasonLength         jsontext.Token = jsontext.String("length")
	notCapturedReasonDepth          jsontext.Token = jsontext.String("depth")
	notCapturedReasonCollectionSize jsontext.Token = jsontext.String("collectionSize")
	notCapturedReasonPruned         jsontext.Token = jsontext.String("pruned")
	notCapturedReasonUnavailable    jsontext.Token = jsontext.String("unavailable")
	notCapturedReasonUnimplemented  jsontext.Token = jsontext.String("unimplemented")
	// notCapturedReasonFieldCount     jsontext.Token = jsontext.String("fieldCount")

	truncated jsontext.Token = jsontext.String("truncated")
)
