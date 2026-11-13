// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && cgo

package mallocarena

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/mallocarena/internal/arenaprobe"
)

// The cap lands in a constructor, so it is already in force by the time any test runs here.
// Measuring it therefore means re-executing this binary: with probeEnv set, TestMain runs the
// probe instead of the tests. The uncapped baseline uses the documented override to stand the
// constructor down, which keeps the test from needing a back door into production code.
const (
	probeEnv     = "DD_MALLOCARENA_PROBE"
	probeThreads = 32
	probeAllocs  = 20000
	probeSize    = 96 << 10
	// uncappedLimit is high enough to leave glibc's own 8*nproc ceiling in charge.
	uncappedLimit = "1024"
	// notGlibcExit tells the parent to skip: this C library has no malloc_info to read.
	notGlibcExit = 99
)

func TestMain(m *testing.M) {
	mode, isProbe := os.LookupEnv(probeEnv)
	if !isProbe {
		os.Exit(m.Run())
	}
	os.Exit(runProbe(mode == "cap"))
}

// runProbe forces arena contention and prints the resulting arena count, followed by SetMax's
// summary when the cap is meant to be in force.
func runProbe(capped bool) int {
	var summary string
	if capped {
		var err error
		if summary, err = SetMax(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}

	arenaprobe.Churn(probeThreads, probeAllocs, probeSize)
	arenas, err := arenaprobe.Count()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return notGlibcExit
	}

	fmt.Println(arenas)
	if capped {
		fmt.Println(summary)
	}
	return 0
}

type probeResult struct {
	arenas  int
	summary string
}

func spawnProbe(t *testing.T, capped bool) probeResult {
	t.Helper()

	mode, arenaMax := "nocap", uncappedLimit
	if capped {
		mode, arenaMax = "cap", ""
	}
	exe, err := os.Executable()
	require.NoError(t, err)

	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), probeEnv+"="+mode, arenaMaxEnv+"="+arenaMax, tunablesEnv+"=")
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == notGlibcExit {
		t.Skip("no malloc_info: this C library has no arenas to count")
	}
	require.NoError(t, err)

	lines := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)
	arenas, err := strconv.Atoi(lines[0])
	require.NoErrorf(t, err, "probe printed %q", out)

	res := probeResult{arenas: arenas}
	if len(lines) > 1 {
		res.summary = lines[1]
	}
	return res
}

// The cap is only observable where contention actually produces secondary arenas, so the
// uncapped run doubles as this test's precondition.
func TestArenasAreCapped(t *testing.T) {
	uncapped := spawnProbe(t, false)
	if uncapped.arenas <= DefaultMaxArenas {
		t.Skipf("no arena contention here: %d arenas without a cap", uncapped.arenas)
	}

	capped := spawnProbe(t, true)
	assert.LessOrEqualf(t, capped.arenas, DefaultMaxArenas,
		"%d arenas with the cap, %d without", capped.arenas, uncapped.arenas)
	// Reaching the limit rather than freezing above it is the constructor's doing; if this
	// regresses to "after startup" the count above will have drifted up with it.
	assert.Contains(t, capped.summary, "before startup")
}

// The constructor cannot read Go constants, so the limit is spelled twice.
func TestConstructorLimitMatchesDefault(t *testing.T) {
	assert.Equal(t, DefaultMaxArenas, constructorLimit)
}

// Comparing the two constants above says nothing about what SetMax tells the operator. Once
// glibc has fixed its ceiling mallopt keeps returning success, so a summary naming any other
// number would read as a cap in force that is not. Check the reported limit against the one
// the constructor compiled in, and against the arena count the probe actually reached.
func TestSummaryReportsLimitInForce(t *testing.T) {
	capped := spawnProbe(t, true)

	match := regexp.MustCompile(`arenas at (\d+)`).FindStringSubmatch(capped.summary)
	require.Lenf(t, match, 2, "no limit in summary %q", capped.summary)
	reported, err := strconv.Atoi(match[1])
	require.NoError(t, err)

	assert.Equal(t, constructorLimit, reported)
	assert.LessOrEqualf(t, capped.arenas, reported,
		"summary reports %d arenas, probe reached %d", reported, capped.arenas)
}
