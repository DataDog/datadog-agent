// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build antithesis_demo

// Antithesis SUT harness that DEMONSTRATES (does not fix) the
// `backpressure-no-rotation-loss` bug in the Datadog Agent logs pipeline, using the
// real `pkg/logs` file tailer + decoder. Gated behind the `antithesis_demo` build
// tag. Build with: CGO_ENABLED=1 go build -tags "antithesis_demo test" ./pkg/logs/cmd/antithesis-rotation-demo
//
// The harness repeatedly: writes N numbered lines to a file, starts the real tailer
// with a small undrained output channel (so the pipeline backpressures), triggers
// StopAfterFileRotation (a rotation), and asserts via the Antithesis SDK that every
// written line was delivered. The `Always` assertion FAILS because lines that were
// read but not yet forwarded are silently dropped when the rotation close-timeout
// fires — that failure is the bug demonstration in the Antithesis triage report.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/antithesishq/antithesis-sdk-go/assert"
	"github.com/antithesishq/antithesis-sdk-go/lifecycle"

	filetailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
)

func main() {
	dir, err := os.MkdirTemp("", "antithesis-rotation-demo")
	if err != nil {
		fmt.Fprintln(os.Stderr, "mktemp:", err)
		os.Exit(1)
	}

	// Signal Antithesis that the SUT is ready and test commands may run.
	lifecycle.SetupComplete(map[string]any{
		"message": "logs rotation-loss demo ready",
		"dir":     dir,
	})

	const (
		totalLines   = 50
		outChanSize  = 3 // small => downstream backpressures quickly
		closeTimeout = 1 * time.Second
	)

	for {
		res, err := filetailer.RunRotationLossExperiment(dir, totalLines, outChanSize, closeTimeout)
		if err != nil {
			// Operational error (not the property under test); record reachability and continue.
			assert.Reachable("rotation experiment errored", map[string]any{"error": err.Error()})
			time.Sleep(time.Second)
			continue
		}

		details := map[string]any{
			"written":   res.Written,
			"delivered": res.Delivered,
			"dropped":   res.Written - res.Delivered,
		}

		// Confirms the experiment actually exercised the rotation path.
		assert.Reachable("rotation-under-backpressure experiment completed", details)

		// PROPERTY backpressure-no-rotation-loss (Safety): every written line is
		// delivered at least once across a rotation. EXPECTED TO FAIL.
		assert.Always(
			res.Delivered == res.Written,
			"every log line written before a rotation is delivered at least once (backpressure-no-rotation-loss)",
			details,
		)

		fmt.Printf("[rotation-demo] written=%d delivered=%d dropped=%d\n",
			res.Written, res.Delivered, res.Written-res.Delivered)

		// --- Bug 2: offset-no-regression-on-seek-error ---
		seek, serr := filetailer.RunSeekErrorExperiment(dir)
		if serr != nil {
			assert.Reachable("seek experiment errored", map[string]any{"error": serr.Error()})
		} else {
			sdetails := map[string]any{
				"resume_line":     seek.ResumeLine,
				"delivered_count": seek.DeliveredCount,
				"regressed":       seek.Regressed,
				"first_line":      seek.FirstLine,
			}
			assert.Reachable("seek-error resume experiment completed", sdetails)
			// PROPERTY offset-no-regression-on-seek-error (Safety): a failed seek must
			// not regress the read offset to 0 and re-deliver consumed lines. EXPECTED
			// TO FAIL.
			assert.Always(
				!seek.Regressed,
				"a failed seek does not regress the tailer offset to 0 (offset-no-regression-on-seek-error)",
				sdetails,
			)
			fmt.Printf("[seek-demo] resume_line=%d delivered=%d regressed=%v first=%s\n",
				seek.ResumeLine, seek.DeliveredCount, seek.Regressed, seek.FirstLine)
		}

		time.Sleep(500 * time.Millisecond)
	}
}
