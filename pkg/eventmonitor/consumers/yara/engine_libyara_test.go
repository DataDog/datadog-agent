// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && yara

package yara

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func compileLibyaraForTest(t *testing.T, sources ...RuleSource) *libyaraScanner {
	t.Helper()
	scanner, err := DefaultCompiler()(sources)
	require.NoError(t, err)
	s := scanner.(*libyaraScanner)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestLibyaraEngineName(t *testing.T) {
	assert.Equal(t, "libyara", EngineName)
}

func TestLibyaraCompileError(t *testing.T) {
	_, err := DefaultCompiler()([]RuleSource{
		{Name: "good.yar", Data: []byte(testRule)},
		{Name: "bad.yar", Data: []byte("rule broken {\n  condition:\n    $undefined\n}\n")},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad.yar")
	assert.Contains(t, err.Error(), "undefined string")
	assert.Regexp(t, `line \d+`, err.Error())
}

func TestLibyaraMarkerMatch(t *testing.T) {
	s := compileLibyaraForTest(t, RuleSource{Name: "marker.yar", Data: []byte(testRule)})

	matches, err := s.Scan(context.Background(), []byte("some bytes EVIL_MARKER more bytes"))
	require.NoError(t, err)
	assert.Equal(t, []Match{{Rule: "evil_marker", Namespace: "marker.yar", Tags: []string{"test"}}}, matches)

	matches, err = s.Scan(context.Background(), []byte("nothing to see here"))
	require.NoError(t, err)
	assert.Empty(t, matches)

	matches, err = s.Scan(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func TestLibyaraHexAndRegex(t *testing.T) {
	s := compileLibyaraForTest(t,
		RuleSource{Name: "hex.yar", Data: []byte(`rule elf_hex { strings: $h = { 7F 45 4C 46 ?? [2-4] 00 } condition: $h at 0 }`)},
		RuleSource{Name: "re.yar", Data: []byte(`rule url_re : net c2 { strings: $r = /https?:\/\/[a-z]{3,10}\.example\/[0-9]+/ condition: $r }`)},
	)

	elfHeader := []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00, 0x00}
	matches, err := s.Scan(context.Background(), elfHeader)
	require.NoError(t, err)
	assert.Equal(t, []Match{{Rule: "elf_hex", Namespace: "hex.yar", Tags: []string{}}}, normalizeTags(matches))

	matches, err = s.Scan(context.Background(), []byte("GET http://evil.example/1234 HTTP/1.1"))
	require.NoError(t, err)
	assert.Equal(t, []Match{{Rule: "url_re", Namespace: "re.yar", Tags: []string{"net", "c2"}}}, matches)

	matches, err = s.Scan(context.Background(), append(append([]byte{}, elfHeader...), []byte(" https://abc.example/9")...))
	require.NoError(t, err)
	assert.Len(t, matches, 2)
}

// normalizeTags turns nil tags into empty slices, for comparisons
func normalizeTags(matches []Match) []Match {
	for i := range matches {
		if matches[i].Tags == nil {
			matches[i].Tags = []string{}
		}
	}
	return matches
}

func TestLibyaraModules(t *testing.T) {
	for _, module := range []string{"elf", "math", "string", "time"} {
		t.Run(module, func(t *testing.T) {
			_, err := DefaultCompiler()([]RuleSource{{
				Name: module + ".yar",
				Data: []byte(fmt.Sprintf("import %q\nrule r { condition: true }\n", module)),
			}})
			assert.NoError(t, err)
		})
	}
	// modules left out of the build, see deps/libyara
	for _, module := range []string{"pe", "hash", "dotnet", "console", "tests", "cuckoo", "magic"} {
		t.Run(module, func(t *testing.T) {
			_, err := DefaultCompiler()([]RuleSource{{
				Name: module + ".yar",
				Data: []byte(fmt.Sprintf("import %q\nrule r { condition: true }\n", module)),
			}})
			assert.Error(t, err)
		})
	}
}

func TestLibyaraElfModule(t *testing.T) {
	s := compileLibyaraForTest(t, RuleSource{Name: "elf.yar", Data: []byte(`import "elf"
rule not_elf { condition: not defined elf.type }
`)})
	matches, err := s.Scan(context.Background(), []byte("#!/bin/sh\necho hello\n"))
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "not_elf", matches[0].Rule)
}

func TestLibyaraIncludesDisabled(t *testing.T) {
	_, err := DefaultCompiler()([]RuleSource{{
		Name: "include.yar",
		Data: []byte("include \"/etc/passwd\"\nrule r { condition: true }\n"),
	}})
	require.Error(t, err)
}

func TestLibyaraConcurrentScans(t *testing.T) {
	s := compileLibyaraForTest(t, RuleSource{Name: "marker.yar", Data: []byte(testRule)})

	const goroutines = 8
	const scans = 50
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < scans; i++ {
				withMarker := (g+i)%2 == 0
				data := []byte(fmt.Sprintf("goroutine %d scan %d", g, i))
				if withMarker {
					data = append(data, "EVIL_MARKER"...)
				}
				matches, err := s.Scan(context.Background(), data)
				if err != nil {
					errs <- err
					return
				}
				if withMarker != (len(matches) == 1) {
					errs <- fmt.Errorf("goroutine %d scan %d: got %d matches", g, i, len(matches))
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	// at most one scanner object per concurrent scan is kept
	assert.LessOrEqual(t, len(s.scanners), goroutines)
}

func TestLibyaraTimeout(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		left     time.Duration
		expected time.Duration
	}{
		{left: time.Millisecond, expected: time.Second},
		{left: time.Second, expected: time.Second},
		{left: time.Second + time.Nanosecond, expected: 2 * time.Second},
		{left: 10 * time.Second, expected: 10 * time.Second},
		{left: 9500 * time.Millisecond, expected: 10 * time.Second},
	} {
		ctx, cancel := context.WithDeadline(context.Background(), now.Add(tc.left))
		timeout, err := libyaraTimeout(ctx, now)
		cancel()
		require.NoError(t, err)
		assert.Equal(t, tc.expected, timeout, "left %s", tc.left)
	}

	timeout, err := libyaraTimeout(context.Background(), now)
	require.NoError(t, err)
	assert.Equal(t, libyaraNoDeadlineTimeout, timeout)

	ctx, cancel := context.WithDeadline(context.Background(), now)
	defer cancel()
	_, err = libyaraTimeout(ctx, now)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestLibyaraDoneContext(t *testing.T) {
	s := compileLibyaraForTest(t, RuleSource{Name: "marker.yar", Data: []byte(testRule)})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.Scan(ctx, []byte("EVIL_MARKER"))
	assert.ErrorIs(t, err, context.Canceled)

	ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, err = s.Scan(ctx, []byte("EVIL_MARKER"))
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestLibyaraEngineTimeout(t *testing.T) {
	// about 1e9 loop iterations: far more than one second of work
	s := compileLibyaraForTest(t, RuleSource{Name: "slow.yar", Data: []byte(`rule slow {
    condition:
        for all i in (0..1000) : (for all j in (0..1000) : (for all k in (0..1000) : (i + j + k >= 0)))
}`)})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	matches, err := s.Scan(ctx, []byte("data"))
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded), "unexpected error: %v", err)
	assert.Nil(t, matches)
	// the engine timeout is rounded up to one second
	assert.GreaterOrEqual(t, elapsed, 900*time.Millisecond)
	assert.Less(t, elapsed, 10*time.Second)

	// the scanner object went back to the free list
	assert.Len(t, s.scanners, 1)
}

func TestLibyaraClose(t *testing.T) {
	scanner, err := DefaultCompiler()([]RuleSource{{Name: "marker.yar", Data: []byte(testRule)}})
	require.NoError(t, err)
	s := scanner.(*libyaraScanner)
	_, err = s.Scan(context.Background(), []byte("EVIL_MARKER"))
	require.NoError(t, err)

	require.NoError(t, s.Close())
	require.NoError(t, s.Close())
	_, err = s.Scan(context.Background(), []byte("EVIL_MARKER"))
	assert.Error(t, err)
}

func TestLibyaraLoadScanner(t *testing.T) {
	// the temp dir is owned by the test user, not root
	trustCurrentUser(t)
	dir := t.TempDir()
	writeRuleFiles(t, dir, map[string]string{
		"marker.yar": testRule,
		"hex.yara":   `rule hex_marker { strings: $h = { DE AD BE EF } condition: $h }`,
		"notes.txt":  "not a rule file",
	})

	scanner, version, err := LoadScanner(dir, DefaultCompiler())
	require.NoError(t, err)
	assert.Equal(t, version, scanner.RulesVersion())
	assert.NotEqual(t, EngineName, version)

	matches, err := scanner.Scan(context.Background(), []byte("xx EVIL_MARKER xx \xde\xad\xbe\xef"))
	require.NoError(t, err)
	assert.ElementsMatch(t, []Match{
		{Rule: "evil_marker", Namespace: "marker.yar", Tags: []string{"test"}},
		{Rule: "hex_marker", Namespace: "hex.yara", Tags: []string{}},
	}, normalizeTags(matches))

	writeRuleFiles(t, dir, map[string]string{"broken.yar": "rule broken { condition: nope }"})
	_, _, err = LoadScanner(dir, DefaultCompiler())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broken.yar")
}
