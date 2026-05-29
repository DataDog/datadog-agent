// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build antithesis_demo

// This file is an Antithesis bug *demonstration*, not a fix. It is gated behind
// the `antithesis_demo` build tag so it never runs in normal CI. Run it with:
//
//	go test -tags antithesis_demo -run TestAntithesisRotationUnderBackpressureLoss \
//	    ./pkg/logs/tailers/file/ -v
//
// It demonstrates property `backpressure-no-rotation-loss` from the Antithesis
// research catalog: when a file rotates while the pipeline is backpressured, log
// lines that were read but not yet forwarded are silently discarded when the
// rotation close-timeout fires (`Tailer.StopAfterFileRotation` cancels
// `forwardContext` before the in-flight messages drain). No data-loss metric beyond
// the coarse per-rotation `BytesMissed` is emitted, and the lines never reach the
// output. The test ASSERTS at-least-once delivery and is EXPECTED TO FAIL, proving
// the loss.

package file

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditormock "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	"github.com/DataDog/datadog-agent/pkg/logs/util/opener"
)

func TestAntithesisRotationUnderBackpressureLoss(t *testing.T) {
	const (
		totalLines  = 50
		outChanSize = 3 // small => downstream backpressures quickly
	)

	dir := t.TempDir()
	path := filepath.Join(dir, "tailer.log")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()

	// Write more lines than the output channel can hold so the pipeline backs up.
	for i := 0; i < totalLines; i++ {
		if _, err := f.WriteString(fmt.Sprintf("L%05d\n", i)); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	outputChan := make(chan *message.Message, outChanSize)
	src := sources.NewReplaceableSource(sources.NewLogSource("", &config.LogsConfig{
		Type: config.FileType,
		Path: path,
	}))
	info := status.NewInfoRegistry()

	tailer := NewTailer(&TailerOptions{
		OutputChan:      outputChan,
		File:            NewFile(path, src.UnderlyingSource(), false),
		SleepDuration:   10 * time.Millisecond,
		Decoder:         decoder.NewDecoderFromSource(src, info),
		Info:            info,
		CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
		Registry:        auditormock.NewMockRegistry(),
		FileOpener:      opener.NewFileOpener(),
	})
	// Short rotation close-timeout so the demo runs quickly (default is 60s).
	tailer.closeTimeout = 1 * time.Second

	if err := tailer.StartFromBeginning(); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Let the tailer read the file and fill/​block on the output channel
	// (downstream is intentionally NOT draining => backpressure).
	time.Sleep(300 * time.Millisecond)

	// The file rotates while the tailer is backpressured.
	tailer.StopAfterFileRotation()

	// Wait for the tailer to fully stop (closeTimeout governs the drop).
	select {
	case <-tailer.done:
	case <-time.After(tailer.closeTimeout + 10*time.Second):
		t.Fatal("tailer did not stop")
	}

	// Drain whatever actually made it to the output.
	received := map[string]bool{}
	for {
		select {
		case msg := <-outputChan:
			received[string(msg.GetContent())] = true
		default:
			goto done
		}
	}
done:

	t.Logf("delivered %d / %d lines after rotation-under-backpressure", len(received), totalLines)

	// Property backpressure-no-rotation-loss: every written line is delivered at
	// least once. EXPECTED TO FAIL — demonstrating silent loss on rotation.
	if len(received) != totalLines {
		t.Fatalf("BUG DEMONSTRATED (backpressure-no-rotation-loss): %d of %d lines "+
			"silently dropped on file rotation under backpressure; only %d delivered",
			totalLines-len(received), totalLines, len(received))
	}
}
