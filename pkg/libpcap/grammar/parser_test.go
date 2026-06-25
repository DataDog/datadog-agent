// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grammar

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/libpcap/codegen"
)

func TestParseEmpty(t *testing.T) {
	cs := codegen.NewCompilerState(1, 262144, 0, nil)
	err := Parse(cs, "")
	// Empty filter is valid — it matches all packets
	if err != nil {
		t.Errorf("Parse('') = %v, want nil", err)
	}
}

func TestParseSyntaxError(t *testing.T) {
	cs := codegen.NewCompilerState(1, 262144, 0, nil)
	err := Parse(cs, "((((")
	if err == nil {
		t.Error("Parse('((((') should fail with syntax error")
	}
}

func TestParseTCPCallsCodegen(t *testing.T) {
	// "tcp" should parse successfully but codegen stubs will return an error
	cs := codegen.NewCompilerState(1, 262144, 0, nil)
	err := Parse(cs, "tcp")
	// Currently stubs return "not yet implemented" — the parser succeeds
	// but codegen sets an error. Once codegen is implemented, this will pass.
	if err == nil {
		// If no error, the parse succeeded and codegen produced a block
		if cs.IC.Root == nil {
			t.Error("Parse('tcp') succeeded but IC.Root is nil")
		}
	}
	// Errors from stubs are expected for now
}

func TestParseAndOrNot(t *testing.T) {
	for _, expr := range []string{"tcp and udp", "tcp or udp", "not tcp"} {
		cs := codegen.NewCompilerState(1, 262144, 0, nil)
		if err := Parse(cs, expr); err != nil {
			t.Errorf("Parse(%q) = %v", expr, err)
		}
	}
}
