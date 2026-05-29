//go:build antithesis_demo

// Antithesis bug-demonstration support (NOT a fix). Gated behind the
// `antithesis_demo` build tag so it never compiles into the production agent or
// normal CI. It exposes the rotation-under-backpressure experiment so an external
// `package main` harness (which cannot reach package-private fields or the internal
// decoder) can drive the real file tailer and observe silent log loss.
//
// Build with: -tags "antithesis_demo test"  (the auditor mock is behind `test`).

package file

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/afero"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditormock "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	"github.com/DataDog/datadog-agent/pkg/logs/util/opener"
)

// --- seek-error fault injection (offset-no-regression-on-seek-error) ---

// seekFailingFile wraps a real afero.File but fails every Seek (an injected
// filesystem fault).
type seekFailingFile struct {
	afero.File
}

func (s *seekFailingFile) Seek(int64, int) (int64, error) {
	return 0, errors.New("injected seek failure")
}

// seekFailingOpener returns files whose Seek always fails.
type seekFailingOpener struct{ fs afero.Fs }

func (o *seekFailingOpener) OpenLogFile(path string) (afero.File, error) {
	f, err := o.fs.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	return &seekFailingFile{File: f}, nil
}
func (o *seekFailingOpener) OpenShared(path string) (afero.File, error) { return o.OpenLogFile(path) }
func (o *seekFailingOpener) Abs(path string) (string, error)            { return filepath.Abs(path) }

// SeekErrorResult reports the outcome of one seek-error experiment.
type SeekErrorResult struct {
	ResumeLine     int
	DeliveredCount int
	Regressed      bool   // true if a line before the resume offset was re-delivered
	FirstLine      string // first line delivered
}

// RunSeekErrorExperiment writes a file, then resumes a tailer from a mid-file offset
// behind a Seek-failing file (injected filesystem fault). A correct resume reads only
// from the offset forward; the bug discards the seek error, regresses the offset to 0,
// and re-reads the whole file (re-delivering already-consumed lines).
//
// Property: offset-no-regression-on-seek-error.
func RunSeekErrorExperiment(dir string) (SeekErrorResult, error) {
	const totalLines = 20
	const lineWidth = 7 // "L00000\n"
	const resumeLine = 10
	res := SeekErrorResult{ResumeLine: resumeLine}

	path := filepath.Join(dir, fmt.Sprintf("seek-%d.log", time.Now().UnixNano()))
	f, err := os.Create(path)
	if err != nil {
		return res, err
	}
	for i := 0; i < totalLines; i++ {
		if _, err := f.WriteString(fmt.Sprintf("L%05d\n", i)); err != nil {
			_ = f.Close()
			return res, err
		}
	}
	_ = f.Close()
	defer func() { _ = os.Remove(path) }()

	outputChan := make(chan *message.Message, 100)
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
		FileOpener:      &seekFailingOpener{fs: afero.NewOsFs()},
	})

	if err := tailer.Start(int64(resumeLine*lineWidth), io.SeekStart); err != nil {
		return res, err
	}

	var delivered []string
	done := make(chan struct{})
	go func() {
		for {
			select {
			case msg := <-outputChan:
				delivered = append(delivered, string(msg.GetContent()))
			case <-done:
				return
			}
		}
	}()
	time.Sleep(600 * time.Millisecond)
	tailer.Stop()
	time.Sleep(100 * time.Millisecond)
	close(done)

	res.DeliveredCount = len(delivered)
	if len(delivered) > 0 {
		res.FirstLine = delivered[0]
	}
	res.Regressed = strings.Contains(strings.Join(delivered, "\n"), "L00000")
	return res, nil
}

// RotationLossResult reports the outcome of one experiment.
type RotationLossResult struct {
	Written   int
	Delivered int
	ReadBytes int
}

// RunRotationLossExperiment writes `totalLines` numbered lines to a file, starts the
// real Agent file tailer with a deliberately small (undrained) output channel so the
// pipeline backpressures, then triggers StopAfterFileRotation (simulating a log
// rotation). It returns how many distinct lines actually reached the output channel.
//
// Property under demonstration: backpressure-no-rotation-loss — every written line
// should be delivered at least once. In the current code, lines that were read but
// not yet forwarded are silently discarded when the rotation close-timeout fires
// (StopAfterFileRotation cancels forwardContext before in-flight messages drain).
func RunRotationLossExperiment(dir string, totalLines, outChanSize int, closeTimeout time.Duration) (RotationLossResult, error) {
	res := RotationLossResult{Written: totalLines}

	path := filepath.Join(dir, fmt.Sprintf("tailer-%d.log", time.Now().UnixNano()))
	f, err := os.Create(path)
	if err != nil {
		return res, err
	}
	defer func() { _ = f.Close(); _ = os.Remove(path) }()

	for i := 0; i < totalLines; i++ {
		if _, err := f.WriteString(fmt.Sprintf("L%05d\n", i)); err != nil {
			return res, err
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
	tailer.closeTimeout = closeTimeout

	if err := tailer.StartFromBeginning(); err != nil {
		return res, err
	}

	// Let the tailer read the file and block on the undrained output channel.
	time.Sleep(300 * time.Millisecond)

	// Rotation while backpressured.
	tailer.StopAfterFileRotation()

	select {
	case <-tailer.done:
	case <-time.After(closeTimeout + 10*time.Second):
		return res, fmt.Errorf("tailer did not stop within timeout")
	}

	received := map[string]bool{}
drain:
	for {
		select {
		case msg := <-outputChan:
			received[string(msg.GetContent())] = true
		default:
			break drain
		}
	}
	res.Delivered = len(received)
	return res, nil
}
