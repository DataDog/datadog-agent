// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGoValidRegisterLine_RealDumps covers register-line shapes actually
// emitted by upstream Go's per-architecture dumpregs() across the GOARCHes
// listed in runtime/signal_*.go. Each shape must validate.
func TestGoValidRegisterLine_RealDumps(t *testing.T) {
	cases := []struct {
		name string
		line string
	}{
		// arm64 / arm: upstream Go runtime emits "<name> 0x<hex>" with a
		// single space (e.g. `print("r0 ", hex(c.r0()), "\n")`).
		{"arm64 single-space r0", "r0 0x332a97571dd8"},
		{"arm64 single-space r29", "r29 0xfffffff0"},
		{"arm64 single-space lr", "lr 0x1028f0c2c"},
		{"arm64 single-space sp", "sp 0x16d586380"},
		{"arm64 single-space pc", "pc 0x10291c6a8"},
		{"arm64 single-space fault", "fault 0x104e64000"},
		{"arm trap", "trap 0x6"},
		{"arm oldmask (7-char name)", "oldmask 0x0"},
		{"arm cpsr", "cpsr 0x60000000"},
		{"arm fp", "fp 0xbeefcafe"},
		{"arm ip", "ip 0x10000000"},

		// Real-world corpus: many darwin/arm64 dumps observed in our
		// crash_patterns corpus are post-formatted to column-aligned output
		// with multiple spaces between name and value.
		{"corpus arm64 padded r0", "r0      0x104"},
		{"corpus arm64 padded r29", "r29     0xfffffff0"},
		{"corpus arm64 padded fault", "fault   0x104e64000"},
		{"corpus tab-separated", "r0\t0x332a97571dd8"},

		// amd64 linux: single space separator.
		{"amd64 rax", "rax 0x7fffabcd1234"},
		{"amd64 rsp", "rsp 0x7fffabcd1234"},
		{"amd64 rip", "rip 0x7fffabcd1234"},
		{"amd64 rflags", "rflags 0x246"},
		{"amd64 r8", "r8 0x0"},
		{"amd64 r15", "r15 0xffffffff"},
		{"amd64 cs", "cs 0x33"},
		{"amd64 fs", "fs 0x0"},

		// 386 linux (analogous to amd64).
		{"386 eax", "eax 0xdeadbeef"},
		{"386 eflags", "eflags 0x246"},

		// ppc64x: packed-pair layout "r0   0xaaaa\tr1   0xbbbb".
		{"ppc64 packed pair r0/r1", "r0   0xaaaa\tr1   0xbbbb"},
		{"ppc64 packed pair r30/r31", "r30  0xcafe\tr31  0xbabe"},
		{"ppc64 packed pair pc/ctr", "pc   0x100abc\tctr  0xdeadbeef"},
		{"ppc64 packed pair link/xer", "link 0x100\txer  0x200"},
		{"ppc64 packed pair ccr/trap", "ccr  0x0\ttrap 0x400"},

		// mips64x: packed-pair layout, includes lo/hi.
		{"mips64 packed pair pc/link", "pc   0xabc\tlink 0xdef"},
		{"mips64 packed pair lo/hi", "lo   0x0\thi   0x1"},

		// loong64: packed-pair layout (r0..r31 plus pc/link).
		{"loong64 packed pair r0/r1", "r0 0xaaaa\tr1 0xbbbb"},
		{"loong64 packed pair pc/link", "pc 0x100\tlink 0x200"},

		// riscv64: mixed name lengths (ra, sp, gp, tp, t0..t6, s0..s11, a0..a7).
		{"riscv64 ra", "ra 0xdeadbeef"},
		{"riscv64 packed sp/gp", "sp 0xff\tgp 0xee"},
		{"riscv64 packed s10/s11", "s10 0xa\ts11 0xb"},
		{"riscv64 packed a0/a7", "a0 0x1\ta7 0x8"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assert.True(t, goValidRegisterLine([]byte(tc.line)),
				"expected to accept register line: %q", tc.line)
		})
	}
}

// TestGoValidRegisterLine_Injections covers lines that the old
// single-byte validator would have wrongly accepted, and which the
// hardened structural check must now reject.
func TestGoValidRegisterLine_Injections(t *testing.T) {
	rejects := []struct {
		name string
		line string
	}{
		// Lowercase-led log injections (the original validator accepted all of these).
		{"key=value log line", "error=denied"},
		{"structured log header", "level=info ts=2026-01-01"},
		{"short token", "ok"},
		{"two-char no value", "rx"},
		{"app msg", "uptime 12s"},
		{"app msg with number", "users 42"},

		// Looks register-ish but value isn't hex-prefixed.
		{"missing 0x prefix", "rax 7fffabcd"},
		{"decimal value", "r0 1234"},
		{"hex without 0x", "rax deadbeef"},
		{"value is text", "rax abcdef"},

		// Name is wrong shape.
		{"name too long (9 chars)", "registers 0xff"},
		{"name with underscore (rejected even within 8 chars)", "rax_ext 0xff"},
		{"name with hyphen", "r-0 0x1"},
		{"name with dot", "r.0 0x1"},
		{"name with underscore", "r_0 0x1"},
		{"name leading digit", "0r 0xff"},
		{"name uppercase", "RAX 0x1"},
		{"name with symbol", "r0! 0x1"},

		// Missing whitespace separator.
		{"no whitespace before value", "rax0xff"},
		{"no whitespace before value (long name)", "rflags0xff"},

		// Empty hex.
		{"empty hex value", "rax 0x"},
		{"value is 0x then space", "rax 0x "},

		// Lines that are NOT register dumps and start with non-lowercase.
		{"chunk boundary goroutine", "goroutine 17 [chan receive]:"},
		{"signal header", "[signal SIGSEGV: segmentation violation]"},
		{"runtime stack header", "runtime stack:"},
		{"tab-indented file line", "\tmain.foo()"},
		{"frames elided", "...33554244 frames elided..."},
		{"empty line", ""},

		// Packed-pair attacks: first half OK, second half malformed.
		{"packed first ok, second bad name", "rax 0xff\t0bad 0x0"},
		{"packed first ok, second missing 0x", "rax 0xff\trcx ff"},
		{"packed first ok, trailing garbage", "rax 0xff\textra"},
		{"packed first ok, trailing tab", "rax 0xff\t"},

		// Trailing garbage after a valid first pair (no embedded tab to
		// indicate a packed pair) — must reject.
		{"valid pair with trailing text", "rax 0xff garbage"},
		{"valid pair with trailing space", "rax 0xff "},
	}
	for _, tc := range rejects {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assert.False(t, goValidRegisterLine([]byte(tc.line)),
				"expected to reject line: %q", tc.line)
		})
	}
}

// TestGoConsumeRegisterPair_Boundaries verifies the helper's remainder
// behavior, which the packed-pair logic depends on.
func TestGoConsumeRegisterPair_Boundaries(t *testing.T) {
	t.Run("returns empty remainder when fully consumed", func(t *testing.T) {
		rest, ok := goConsumeRegisterPair([]byte("rax 0xff"))
		require.True(t, ok)
		assert.Equal(t, 0, len(rest))
	})

	t.Run("returns tab-led remainder for packed pair", func(t *testing.T) {
		rest, ok := goConsumeRegisterPair([]byte("rax 0xff\tr1 0x1"))
		require.True(t, ok)
		require.Greater(t, len(rest), 0)
		assert.Equal(t, byte('\t'), rest[0])
		assert.Equal(t, "\tr1 0x1", string(rest))
	})

	t.Run("returns original slice on failure", func(t *testing.T) {
		orig := []byte("nope")
		rest, ok := goConsumeRegisterPair(orig)
		assert.False(t, ok)
		assert.Equal(t, "nope", string(rest))
	})

	t.Run("accepts maximum-length name", func(t *testing.T) {
		// 8 bytes ("registr8") is the upper bound, accommodating arm/linux's
		// 7-byte "oldmask" with one byte of headroom.
		_, ok := goConsumeRegisterPair([]byte("registr8 0xff"))
		assert.True(t, ok, "8-char name must validate")
	})

	t.Run("rejects 9-char name", func(t *testing.T) {
		_, ok := goConsumeRegisterPair([]byte("registers 0xff"))
		assert.False(t, ok, "9-char name must not validate")
	})

	t.Run("accepts uppercase hex digits", func(t *testing.T) {
		_, ok := goConsumeRegisterPair([]byte("rax 0xDEADBEEF"))
		assert.True(t, ok)
	})

	t.Run("accepts long hex value", func(t *testing.T) {
		_, ok := goConsumeRegisterPair([]byte("r0 0xffffffffffffffff"))
		assert.True(t, ok)
	})
}

// TestGoIsRegisterDumpStart_PerArchLeaders verifies that the first line of a
// dumpregs() block is recognized as a register-dump start on every GOARCH.
// The leading register per arch is taken directly from runtime/signal_*.go.
func TestGoIsRegisterDumpStart_PerArchLeaders(t *testing.T) {
	accepts := []struct {
		arch string
		line string
	}{
		{"amd64 (rax)", "rax 0x7fffabcd1234"},
		{"386 (eax)", "eax 0xdeadbeef"},
		{"arm64 (r0, single space)", "r0 0x332a97571dd8"},
		{"arm64 (r0, tab)", "r0\t0x332a97571dd8"},
		{"arm64 (r0, padded)", "r0      0x104"},
		{"arm (trap)", "trap 0x6"},
		{"ppc64x (r0 packed)", "r0   0xaaaa\tr1   0xbbbb"},
		{"mips64x (r0 packed)", "r0   0xabc\tr1   0xdef"},
		{"mipsx (r0 packed)", "r0   0xabc\tr1   0xdef"},
		{"s390x (r0 packed)", "r0 0x1\tr1 0x2"},
		{"loong64 (r0 packed)", "r0 0xaaaa\tr1 0xbbbb"},
		{"riscv64 (ra packed)", "ra 0xdeadbeef\tsp 0xff"},
	}
	for _, tc := range accepts {
		tc := tc
		t.Run("accept_"+tc.arch, func(t *testing.T) {
			assert.True(t, goIsRegisterDumpStart([]byte(tc.line)),
				"expected register-dump start for %s: %q", tc.arch, tc.line)
		})
	}

	rejects := []struct {
		name string
		line string
	}{
		// Word-like leaders that are NOT register dumps (no hex value).
		{"trap word log", "trap handler installed"},
		{"ra word log", "ra debug enabled"},
		{"eax-ish word", "eaxample 0xff"}, // name token is "eaxample", not "eax"
		{"r0 prefix but longer name", "r0x 0xff"},
		// Mid-dump register names must NOT trigger a *start* (only leaders do).
		{"rbx is not a leader", "rbx 0x0"},
		{"r1 is not a leader", "r1 0xff"},
		{"pc is not a leader", "pc 0x10291c6a8"},
		{"sp is not a leader", "sp 0x16d586380"},
		{"error (arm 2nd line) not a leader", "error 0x0"},
		{"oldmask (arm 3rd line) not a leader", "oldmask 0x0"},
		// Leader name but malformed value.
		{"rax no hex", "rax nothex"},
		{"trap decimal", "trap 6"},
		// Non-register lines.
		{"goroutine header", "goroutine 1 [running]:"},
		{"empty", ""},
	}
	for _, tc := range rejects {
		tc := tc
		t.Run("reject_"+tc.name, func(t *testing.T) {
			assert.False(t, goIsRegisterDumpStart([]byte(tc.line)),
				"did not expect register-dump start: %q", tc.line)
		})
	}
}

// TestGoStackTraceParser_PerArchRegDumpAggregated drives the parser end-to-end
// for the previously-unsupported arches (386, arm, riscv64), confirming their
// register dumps are now folded into the combined trace.
func TestGoStackTraceParser_PerArchRegDumpAggregated(t *testing.T) {
	cases := []struct {
		arch     string
		regLines []string
	}{
		{"386", []string{"eax 0xdeadbeef", "ebx 0x0", "eip 0x8049abc", "eflags 0x246"}},
		{"arm", []string{"trap 0x6", "error 0x0", "oldmask 0x0", "r0 0x104", "pc 0x1028f0c2c"}},
		{"riscv64", []string{"ra 0xdeadbeef\tsp 0xff", "gp 0x1\ttp 0x2", "pc 0x100\tt0 0x3"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.arch, func(t *testing.T) {
			p := NewGoStackTraceParser()
			require.True(t, p.IsStart([]byte("SIGSEGV: segmentation violation")))
			p.Reset()
			require.True(t, p.AcceptLine([]byte("PC=0x192bf82f4 m=0 sigcode=2 addr=0x0")))
			require.True(t, p.AcceptLine([]byte("")))
			for i, rl := range tc.regLines {
				assert.True(t, p.AcceptLine([]byte(rl)),
					"%s reg line %d should be accepted: %q", tc.arch, i, rl)
			}
			assert.True(t, p.ShouldCombine(), "%s dump should combine", tc.arch)
		})
	}
}

// TestGoStackTraceParser_RegDumpInjectionRejected exercises the parser
// end-to-end: once in the register-dump state, an injected non-register log
// line must cause the line to be rejected rather than silently absorbed.
func TestGoStackTraceParser_RegDumpInjectionRejected(t *testing.T) {
	p := NewGoStackTraceParser()

	// Drive the state machine into goStateInRegDump: header → blank →
	// register-dump start.
	require.True(t, p.IsStart([]byte("SIGSEGV: segmentation violation")))
	p.Reset()
	require.True(t, p.AcceptLine([]byte("PC=0x192bf82f4 m=0 sigcode=2 addr=0x0")))
	require.True(t, p.AcceptLine([]byte("")))           // blank separator
	require.True(t, p.AcceptLine([]byte("r0\t0xff")))   // chunk start (regdump)
	require.True(t, p.AcceptLine([]byte("r1\t0xee")))   // valid continuation
	require.True(t, p.AcceptLine([]byte("lr\t0xdead"))) // valid continuation

	// Injection: a structured log line that the old validator accepted because
	// its first byte is lowercase. The hardened validator must reject it.
	assert.False(t, p.AcceptLine([]byte("error=denied ts=2026-01-01")),
		"injection line must not be absorbed into the register dump")

	// And legitimate continuations after the rejection (in a fresh parser
	// state) must still work.
	p2 := NewGoStackTraceParser()
	require.True(t, p2.IsStart([]byte("SIGSEGV: segmentation violation")))
	p2.Reset()
	require.True(t, p2.AcceptLine([]byte("PC=0x192bf82f4 m=0 sigcode=2 addr=0x0")))
	require.True(t, p2.AcceptLine([]byte("")))
	require.True(t, p2.AcceptLine([]byte("rax 0xff")))
	require.True(t, p2.AcceptLine([]byte("rbx 0xee")))
	require.True(t, p2.AcceptLine([]byte("rip 0xdead")))
}
