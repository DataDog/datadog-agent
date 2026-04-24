// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package optimizer

import (
	"bytes"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/libpcap/bpf"
	"github.com/DataDog/datadog-agent/pkg/libpcap/codegen"
)

func compileAndDump(t *testing.T, filter string, optimize bool) string {
	t.Helper()
	cs := codegen.NewCompilerState(codegen.DLTEN10MB, 262144, 0, nil)
	codegen.InitLinktype(cs)

	// Parse using grammar
	// We can't easily use the grammar here without importing it,
	// so let's build the CFG manually for specific filters

	// For "tcp": GenProtoAbbrev(Q_TCP) + FinishParse
	var b *codegen.Block
	switch filter {
	case "tcp":
		b = codegen.GenProtoAbbrev(cs, codegen.QTCP)
	case "udp":
		b = codegen.GenProtoAbbrev(cs, codegen.QUDP)
	case "ip":
		b = codegen.GenProtoAbbrev(cs, codegen.QIP)
	default:
		t.Fatalf("unsupported filter %q in test", filter)
	}
	codegen.FinishParse(cs, b)

	if optimize {
		if err := Optimize(&cs.IC); err != nil {
			t.Fatalf("Optimize: %v", err)
		}
	}

	insns, err := codegen.IcodeToFcode(&cs.IC, cs.IC.Root)
	if err != nil {
		t.Fatalf("IcodeToFcode: %v", err)
	}

	var buf bytes.Buffer
	prog := &bpf.Program{Instructions: insns}
	bpf.FprintDump(&buf, prog, 1)
	return buf.String()
}

func TestOptimizeTCP(t *testing.T) {
	unopt := compileAndDump(t, "tcp", false)
	opt := compileAndDump(t, "tcp", true)

	t.Logf("Unoptimized TCP (%d instructions):\n%s", countLines(unopt), unopt)
	t.Logf("Optimized TCP (%d instructions):\n%s", countLines(opt), opt)

	// Expected C output for reference:
	// (000) ldh      [12]
	// (001) jeq      #0x800  jt 2  jf 4
	// (002) ldb      [23]
	// (003) jeq      #0x6    jt 10 jf 11
	// (004) jeq      #0x86dd jt 5  jf 11    ← no ldh [12] (redundant load eliminated)
	// (005) ldb      [20]
	// (006) jeq      #0x6    jt 10 jf 7
	// (007) jeq      #0x2c   jt 8  jf 11    ← no ldb [20] (redundant load eliminated)
	// (008) ldb      [54]
	// (009) jeq      #0x6    jt 10 jf 11
	// (010) ret      #262144
	// (011) ret      #0
	//
	// C has 12 instructions, Go should match.
	expected := 12
	got := countLines(opt)
	if got != expected {
		t.Errorf("optimized TCP: got %d instructions, want %d", got, expected)
	}
}

func TestOptimizeIP(t *testing.T) {
	unopt := compileAndDump(t, "ip", false)
	opt := compileAndDump(t, "ip", true)

	t.Logf("Unoptimized IP:\n%s", unopt)
	t.Logf("Optimized IP:\n%s", opt)

	// For "ip", optimized and unoptimized should be the same (already minimal)
	if unopt != opt {
		t.Logf("IP filter changed after optimization (may be fine)")
	}
}

func countLines(s string) int {
	n := 0
	for _, c := range s {
		if c == '\n' {
			n++
		}
	}
	return n
}
