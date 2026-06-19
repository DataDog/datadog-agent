// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package loclist

import "testing"

func TestParseInstructionsAddrTruncated(t *testing.T) {
	_, err := ParseInstructions([]byte{0x03}, 4, 12)
	if err == nil {
		t.Fatal("expected error for truncated DW_OP_addr payload")
	}
}
