// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package file

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs-library/routing"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	flareController "github.com/DataDog/datadog-agent/comp/logs/agent/flare"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	auditorMock "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	filetailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
	"github.com/DataDog/datadog-agent/pkg/logs/util/opener"
)

// Keep the standalone library's registry contract compatible in both directions
// without requiring it to import the Agent root module.
var (
	_ auditor.Registry = routing.Registry(nil)
	_ routing.Registry = auditor.Registry(nil)
)

type replaySink struct{ ch chan *message.Payload }

func (s *replaySink) Channel() chan *message.Payload { return s.ch }

func TestRoutedProgressReplaysFileFromPersistedPrefix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "replay.log")
	require.NoError(t, os.WriteFile(path, []byte("first\nsecond\n"), 0o600))
	identifier := "file:" + path
	registry := auditorMock.NewMockRegistry()
	makeTailer := func(output chan *message.Message) *filetailer.Tailer {
		source := sources.NewReplaceableSource(sources.NewLogSource("replay", &config.LogsConfig{Type: config.FileType, Path: path}))
		info := status.NewInfoRegistry()
		return filetailer.NewTailer(&filetailer.TailerOptions{
			OutputChan: output, File: filetailer.NewFile(path, source.UnderlyingSource(), false),
			SleepDuration: 10 * time.Millisecond, Decoder: decoder.NewDecoderFromSource(source, info),
			Info: info, Registry: registry, FileOpener: opener.NewFileOpener(),
			CapacityMonitor: metrics.NewNoopPipelineMonitor("").GetCapacityMonitor("", ""),
		})
	}
	read := func(ch chan *message.Message) *message.Message {
		t.Helper()
		select {
		case msg := <-ch:
			return msg
		case <-time.After(5 * time.Second):
			t.Fatal("file record did not arrive")
			return nil
		}
	}
	sink := &replaySink{ch: make(chan *message.Payload, 2)}
	progress, err := routing.NewProgress(4, sink)
	require.NoError(t, err)
	output := make(chan *message.Message, 2)
	tailer := makeTailer(output)
	require.NoError(t, tailer.StartFromBeginning())
	first, second := read(output), read(output)
	require.Equal(t, "first", string(first.GetContent()))
	require.Equal(t, "second", string(second.GetContent()))
	selection := routing.Selection{Groups: []string{"a", "b"}, Generation: 1}
	for _, m := range []*message.Message{first, second} {
		_, err := progress.Register(context.Background(), m, selection, selection.Groups)
		require.NoError(t, err)
	}
	complete := func(p *routing.Progress, group string, m *message.Message) {
		meta := m.MessageMetadata
		p.CompleteGroup(group, message.NewPayload([]*message.MessageMetadata{&meta}, nil, "", 0))
	}
	complete(progress, "a", first)
	complete(progress, "b", first)
	select {
	case checkpoint := <-sink.ch:
		registry.SetOffset(identifier, checkpoint.MessageMetas[0].Origin.Offset) // Periodically persisted auditor output.
	case <-time.After(5 * time.Second):
		t.Fatal("first checkpoint did not arrive")
	}
	require.Equal(t, strconv.Itoa(len("first\n")), registry.GetOffset(identifier))
	complete(progress, "b", second)
	select {
	case checkpoint := <-sink.ch:
		t.Fatalf("B crossed A's pending checkpoint: %s", checkpoint.MessageMetas[0].Origin.Offset)
	default:
	}
	// Capture the persisted state before any cleanup. This is the abrupt-restart
	// boundary: second reached B but never advanced the saved file position.
	saved := registry.GetOffset(identifier)
	tailer.Stop() // Test cleanup after the snapshot; no provider Flush is used.
	progress.Close()
	require.Equal(t, saved, registry.GetOffset(identifier))
	offset, whence, err := Position(registry, identifier, config.End, filetailer.NewFingerprinterMock(), opener.NewFileOpener(), nil)
	require.NoError(t, err)
	require.Equal(t, int64(len("first\n")), offset)
	require.Equal(t, io.SeekStart, whence)
	noCheckpoint := auditorMock.NewMockRegistry()
	missingOffset, missingWhence, err := Position(noCheckpoint, identifier, config.End, filetailer.NewFingerprinterMock(), opener.NewFileOpener(), nil)
	require.NoError(t, err)
	require.Zero(t, missingOffset)
	require.Equal(t, io.SeekEnd, missingWhence, "end mode cannot replay without an initial saved checkpoint")
	replayOutput := make(chan *message.Message, 2)
	replayTailer := makeTailer(replayOutput)
	require.NoError(t, replayTailer.Start(offset, whence))
	defer replayTailer.Stop()
	replayed := read(replayOutput)
	require.Equal(t, "second", string(replayed.GetContent()))
	replaySink := &replaySink{ch: make(chan *message.Payload, 1)}
	replayProgress, err := routing.NewProgress(2, replaySink)
	require.NoError(t, err)
	defer replayProgress.Close()
	_, err = replayProgress.Register(context.Background(), replayed, routing.Selection{Groups: []string{"a", "b"}, Generation: 2}, []string{"a", "b"})
	require.NoError(t, err)
	complete(replayProgress, "b", replayed) // B receives its duplicate after restart.
	complete(replayProgress, "a", replayed)
	select {
	case checkpoint := <-replaySink.ch:
		require.Equal(t, strconv.Itoa(len("first\nsecond\n")), checkpoint.MessageMetas[0].Origin.Offset)
	case <-time.After(5 * time.Second):
		t.Fatal("replayed file checkpoint did not advance")
	}
}

type fakeRoutedFileProvider struct {
	mu              sync.Mutex
	guarded         bool
	failNextInput   int
	failedInputs    int
	maxActiveInputs int // If >0, SourceInput returns ErrSourceCapacity whenever len(inputs) >= maxActiveInputs.
	registry        auditor.Registry
	delayReset      bool
	nextResetSeq    uint64
	forwardedSeq    uint64
	pendingResets   []string
	inputs          map[sourceGeneration]chan *message.Message
	retired         map[sourceGeneration]bool
	received        chan *message.Message
	retireEvent     chan sourceGeneration
}

func newFakeRoutedFileProvider() *fakeRoutedFileProvider {
	return &fakeRoutedFileProvider{
		inputs:      make(map[sourceGeneration]chan *message.Message),
		retired:     make(map[sourceGeneration]bool),
		received:    make(chan *message.Message, 64),
		retireEvent: make(chan sourceGeneration, 16),
	}
}

func (f *fakeRoutedFileProvider) Start()                                  {}
func (f *fakeRoutedFileProvider) Stop()                                   {}
func (f *fakeRoutedFileProvider) NextPipelineChan() chan *message.Message { return f.received }
func (f *fakeRoutedFileProvider) GetOutputChan() chan *message.Message    { return nil }
func (f *fakeRoutedFileProvider) NextPipelineChanWithMonitor() (chan *message.Message, *metrics.CapacityMonitor) {
	return f.received, nil
}
func (f *fakeRoutedFileProvider) GetPipelineMonitor() metrics.PipelineMonitor {
	return metrics.NewNoopPipelineMonitor("")
}
func (f *fakeRoutedFileProvider) Flush(context.Context) {}

func (f *fakeRoutedFileProvider) SourceInput(identifier string, generation uint64) (chan *message.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNextInput > 0 {
		f.failNextInput--
		f.failedInputs++
		return nil, routing.ErrSourceCapacity
	}
	if f.maxActiveInputs > 0 && len(f.inputs) >= f.maxActiveInputs {
		f.failedInputs++
		return nil, routing.ErrSourceCapacity
	}
	ch := make(chan *message.Message, 16)
	sg := sourceGeneration{identifier: identifier, generation: generation}
	f.inputs[sg] = ch
	go func() {
		for m := range ch {
			f.received <- m
		}
	}()
	return ch, nil
}

func (f *fakeRoutedFileProvider) RetireSource(_ context.Context, identifier string, generation uint64) error {
	f.mu.Lock()
	sg := sourceGeneration{identifier: identifier, generation: generation}
	if ch, ok := f.inputs[sg]; ok {
		delete(f.inputs, sg)
		close(ch)
	}
	f.retired[sg] = true
	f.mu.Unlock()
	f.retireEvent <- sg
	return nil
}

func (f *fakeRoutedFileProvider) GuardRegistry(reg routing.Registry) routing.Registry {
	f.mu.Lock()
	f.guarded = true
	f.mu.Unlock()
	return reg
}

func (f *fakeRoutedFileProvider) ResetSourceOffset(identifier string, _ *sources.LogSource) (uint64, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextResetSeq++
	seq := f.nextResetSeq
	if f.delayReset {
		f.pendingResets = append(f.pendingResets, identifier)
		return seq, true
	}
	if f.registry != nil && identifier != "" {
		f.registry.SetOffset(identifier, "0")
	}
	f.forwardedSeq = seq
	return seq, true
}

func (f *fakeRoutedFileProvider) IsOffsetResetForwarded(seq uint64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.forwardedSeq >= seq
}

func (f *fakeRoutedFileProvider) flushDelayedResets() {
	f.mu.Lock()
	resets := f.pendingResets
	f.pendingResets = nil
	reg := f.registry
	f.forwardedSeq = f.nextResetSeq
	f.mu.Unlock()
	if reg != nil {
		for _, id := range resets {
			reg.SetOffset(id, "0")
		}
	}
}

func TestFileLauncherAdoptsRoutedSourceInputAndRetiresGenerations(t *testing.T) {
	mockCfg := configmock.New(t)
	mockCfg.SetInTest("logs_config.close_timeout", 1)
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	require.NoError(t, os.WriteFile(path, []byte("gen1-line\n"), 0o600))
	identifier := "file:" + path

	prov := newFakeRoutedFileProvider()
	reg := auditorMock.NewMockRegistry()
	logSources := sources.NewLogSources()
	tracker := tailers.NewTailerTracker()

	l := NewLauncher(10, 10*time.Millisecond, false, 20*time.Millisecond, WildcardModeByName, flareController.NewFlareController(), nil, opener.NewFileOpener(), filetailer.NewFingerprinterMock())
	l.Start(logSources, prov, reg, tracker)

	src := sources.NewLogSource("app", &config.LogsConfig{
		Type:        config.FileType,
		Path:        path,
		TailingMode: "beginning",
	})
	logSources.AddSource(src)

	select {
	case msg := <-prov.received:
		require.Equal(t, "gen1-line", string(msg.GetContent()))
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for gen1 line")
	}

	prov.mu.Lock()
	require.True(t, prov.guarded, "launcher must wrap registry with GuardRegistry")
	require.Len(t, prov.inputs, 1)
	prov.mu.Unlock()

	// Rotate the file while keeping the old file descriptor open so we can write a
	// trailing line after rename (simulating an app still writing to the old FD).
	oldFD, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	require.NoError(t, err)
	defer oldFD.Close()
	require.NoError(t, os.Rename(path, path+".1"))
	_, err = oldFD.WriteString("gen1-trailing-after-rotate\n")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte("gen2-line\n"), 0o600))

	got := make(map[string]bool)
	deadline := time.After(5 * time.Second)
	for !(got["gen1-trailing-after-rotate"] && got["gen2-line"]) {
		select {
		case msg := <-prov.received:
			got[string(msg.GetContent())] = true
		case <-deadline:
			t.Fatalf("timed out waiting for rotated trailing + gen2 lines; got=%v", got)
		}
	}

	// Old generation (1) should retire while new generation (2) remains active until Stop.
	select {
	case sg := <-prov.retireEvent:
		require.Equal(t, identifier, sg.identifier)
		require.Equal(t, uint64(1), sg.generation)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for gen1 retirement after rotation")
	}

	// Remove source right before Stop to verify async stopTailer completes before
	// Launcher.Stop() returns (so closing provider input channels afterward cannot panic).
	logSources.RemoveSource(src)
	l.Stop()
	// Simulate RoutedProvider.Stop() closing all input channels immediately after Launcher.Stop().
	prov.mu.Lock()
	for sg, ch := range prov.inputs {
		delete(prov.inputs, sg)
		close(ch)
	}
	prov.mu.Unlock()

	// New generation (2) retired cleanly without panic.
	select {
	case sg := <-prov.retireEvent:
		require.Equal(t, identifier, sg.identifier)
		require.Equal(t, uint64(2), sg.generation)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for gen2 retirement on Stop")
	}
}

func TestFileLauncherFailedRotationHandoffPreservesReplacementPrefix(t *testing.T) {
	mockCfg := configmock.New(t)
	mockCfg.SetInTest("logs_config.close_timeout", 1)
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	initialContent := "old-prefix-line-1\n"
	require.NoError(t, os.WriteFile(path, []byte(initialContent), 0o600))
	identifier := "file:" + path

	prov := newFakeRoutedFileProvider()
	reg := auditorMock.NewMockRegistry()
	logSources := sources.NewLogSources()
	tracker := tailers.NewTailerTracker()

	l := NewLauncher(10, 10*time.Millisecond, false, 20*time.Millisecond, WildcardModeByName, flareController.NewFlareController(), nil, opener.NewFileOpener(), filetailer.NewFingerprinterMock())
	l.Start(logSources, prov, reg, tracker)
	defer l.Stop()

	src := sources.NewLogSource("app", &config.LogsConfig{
		Type:        config.FileType,
		Path:        path,
		TailingMode: "beginning",
	})
	logSources.AddSource(src)

	select {
	case msg := <-prov.received:
		require.Equal(t, "old-prefix-line-1", string(msg.GetContent()))
		// Simulate persisted checkpoint at the end of the old file.
		reg.SetOffset(identifier, strconv.Itoa(len(initialContent)))
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for initial line")
	}

	// Force the next SourceInput call (during rotation) to fail with ErrSourceCapacity.
	prov.mu.Lock()
	prov.failNextInput = 1
	prov.mu.Unlock()

	// Rotate the file and write a replacement file LARGER than the old saved offset.
	require.NoError(t, os.Rename(path, path+".1"))
	require.NoError(t, os.WriteFile(path, []byte("new-prefix-line-1\nnew-suffix-line-2\n"), 0o600))

	// Both the replacement prefix (before old offset 18) and suffix MUST be delivered
	// once SourceInput succeeds on the retry scan.
	got := make(map[string]bool)
	deadline := time.After(5 * time.Second)
	for !(got["new-prefix-line-1"] && got["new-suffix-line-2"]) {
		select {
		case msg := <-prov.received:
			got[string(msg.GetContent())] = true
		case <-deadline:
			t.Fatalf("timed out waiting for full replacement file prefix+suffix; got=%v", got)
		}
	}
	prov.mu.Lock()
	require.Equal(t, 1, prov.failedInputs, "rotation must have experienced the forced SourceInput failure before retrying")
	prov.mu.Unlock()
}

func TestFileLauncherOldInfoSurvivesHitFileLimitAndLaunchTailers(t *testing.T) {
	t.Run("routedRotationPending survives a hitFileLimit scan omission", func(t *testing.T) {
		dir := t.TempDir()
		pathA := filepath.Join(dir, "a.log")
		pathB := filepath.Join(dir, "b.log")
		require.NoError(t, os.WriteFile(pathA, []byte("new-prefix-line-1\nnew-suffix-line-2\n"), 0o600))
		require.NoError(t, os.WriteFile(pathB, []byte("other-file-line\n"), 0o600))

		prov := newFakeRoutedFileProvider()
		reg := auditorMock.NewMockRegistry()
		// Saved offset from old rotated inode of a.log (18 bytes == len("new-prefix-line-1\n")).
		reg.SetOffset("file:"+pathA, "18")

		l := NewLauncher(1, 10*time.Millisecond, false, time.Hour, WildcardModeByName, flareController.NewFlareController(), nil, opener.NewFileOpener(), filetailer.NewFingerprinterMock())
		l.pipelineProvider = prov
		l.routedProvider = prov
		l.registry = reg

		srcA := sources.NewLogSource("a", &config.LogsConfig{Type: config.FileType, Path: pathA, TailingMode: "beginning"})
		srcB := sources.NewLogSource("b", &config.LogsConfig{Type: config.FileType, Path: pathB, TailingMode: "beginning"})
		fileA := filetailer.NewFile(pathA, srcA, false)
		fileB := filetailer.NewFile(pathB, srcB, false)

		// Record pending rotation handoff for fileA.
		l.activeSources = []*sources.LogSource{srcA, srcB}
		l.recordRoutedRotationPending(fileA.GetScanKey(), &oldTailerInfo{InfoRegistry: status.NewInfoRegistry()}, srcA, fileA.Identifier(), 0, pathA)

		// Scan 1 hits open_files_limit (tailingLimit=1) with only fileB selected.
		l.resolveActiveTailers([]*filetailer.File{fileB})
		require.Contains(t, l.routedRotationPending, fileA.GetScanKey(), "hitFileLimit omission must preserve routedRotationPending entry")

		// Scan 2 selects fileA -> must read from beginning (both prefix and suffix).
		l.resolveActiveTailers([]*filetailer.File{fileA})
		got := make(map[string]bool)
		deadline := time.After(5 * time.Second)
		for !(got["new-prefix-line-1"] && got["new-suffix-line-2"]) {
			select {
			case msg := <-prov.received:
				got[string(msg.GetContent())] = true
			case <-deadline:
				t.Fatalf("timed out waiting for replacement prefix+suffix after hitFileLimit; got=%v", got)
			}
		}
		l.cleanup()
	})

	t.Run("routedRotationPending survives an under-limit scan where replacement path is temporarily absent", func(t *testing.T) {
		dir := t.TempDir()
		pathA := filepath.Join(dir, "a.log")
		require.NoError(t, os.WriteFile(pathA, []byte("new-prefix-line-1\nnew-suffix-line-2\n"), 0o600))

		prov := newFakeRoutedFileProvider()
		reg := auditorMock.NewMockRegistry()
		reg.SetOffset("file:"+pathA, "18")

		// tailingLimit=10 so an empty scan is strictly under-limit (hitFileLimit == false).
		l := NewLauncher(10, 10*time.Millisecond, false, time.Hour, WildcardModeByName, flareController.NewFlareController(), nil, opener.NewFileOpener(), filetailer.NewFingerprinterMock())
		l.pipelineProvider = prov
		l.routedProvider = prov
		l.registry = reg

		srcA := sources.NewLogSource("a", &config.LogsConfig{Type: config.FileType, Path: pathA, TailingMode: "beginning"})
		l.activeSources = []*sources.LogSource{srcA}
		fileA := filetailer.NewFile(pathA, srcA, false)
		l.recordRoutedRotationPending(fileA.GetScanKey(), &oldTailerInfo{InfoRegistry: status.NewInfoRegistry()}, srcA, fileA.Identifier(), 0, pathA)

		// Under-limit scan omits fileA (simulating rename/create gap or transient stat error).
		l.resolveActiveTailers(nil)
		require.Contains(t, l.routedRotationPending, fileA.GetScanKey(), "under-limit omission must preserve routedRotationPending entry")

		// Next scan sees replacement fileA larger than saved offset 18 -> must deliver full prefix+suffix.
		l.resolveActiveTailers([]*filetailer.File{fileA})
		got := make(map[string]bool)
		deadline := time.After(5 * time.Second)
		for !(got["new-prefix-line-1"] && got["new-suffix-line-2"]) {
			select {
			case msg := <-prov.received:
				got[string(msg.GetContent())] = true
			case <-deadline:
				t.Fatalf("timed out waiting for replacement prefix+suffix after under-limit absence; got=%v", got)
			}
		}
		require.NotContains(t, l.routedRotationPending, fileA.GetScanKey())
		l.cleanup()
	})

	t.Run("launchTailers honors pending routedRotationPending entry with ForceBeginning", func(t *testing.T) {
		dir := t.TempDir()
		pathA := filepath.Join(dir, "a.log")
		require.NoError(t, os.WriteFile(pathA, []byte("new-prefix-line-1\nnew-suffix-line-2\n"), 0o600))

		prov := newFakeRoutedFileProvider()
		reg := auditorMock.NewMockRegistry()
		reg.SetOffset("file:"+pathA, "18")

		l := NewLauncher(10, 10*time.Millisecond, false, time.Hour, WildcardModeByName, flareController.NewFlareController(), nil, opener.NewFileOpener(), filetailer.NewFingerprinterMock())
		l.pipelineProvider = prov
		l.routedProvider = prov
		l.registry = reg

		srcA := sources.NewLogSource("a", &config.LogsConfig{Type: config.FileType, Path: pathA, TailingMode: "beginning"})
		l.activeSources = []*sources.LogSource{srcA}
		fileA := filetailer.NewFile(pathA, srcA, false)
		l.recordRoutedRotationPending(fileA.GetScanKey(), &oldTailerInfo{InfoRegistry: status.NewInfoRegistry()}, srcA, fileA.Identifier(), 0, pathA)

		l.launchTailers(srcA)
		require.NotContains(t, l.routedRotationPending, fileA.GetScanKey(), "consumed routedRotationPending entry must be removed")

		got := make(map[string]bool)
		deadline := time.After(5 * time.Second)
		for !(got["new-prefix-line-1"] && got["new-suffix-line-2"]) {
			select {
			case msg := <-prov.received:
				got[string(msg.GetContent())] = true
			case <-deadline:
				t.Fatalf("timed out waiting for replacement prefix+suffix in launchTailers; got=%v", got)
			}
		}
		l.cleanup()
	})

	t.Run("conservation bound preserves copytruncate regrowth, persistent capacity retirement, open_files_limit, and unrelated files", func(t *testing.T) {
		mockCfg := configmock.New(t)
		mockCfg.SetInTest("logs_config.close_timeout", 1)
		dir := t.TempDir()
		pathCopyTrunc := filepath.Join(dir, "copytrunc.log")
		pathUnrelated := filepath.Join(dir, "unrelated.log")
		pathForceEnd := filepath.Join(dir, "force_end.log")
		initial := "old-line-1\nold-line-2\n"
		require.NoError(t, os.WriteFile(pathCopyTrunc, []byte(initial), 0o600))
		require.NoError(t, os.WriteFile(pathUnrelated, []byte("already-sent-line\nnew-tail-line\n"), 0o600))
		require.NoError(t, os.WriteFile(pathForceEnd, []byte("pre-existing-should-skip\n"), 0o600))

		prov := newFakeRoutedFileProvider()
		// Persistent capacity limit: only 1 active SourceInput allowed at a time!
		// Recovery after rotation MUST retire the old generation first.
		prov.maxActiveInputs = 1
		reg := auditorMock.NewMockRegistry()
		reg.SetOffset("file:"+pathUnrelated, "18")
		reg.SetOffset("file:"+pathForceEnd, "5")

		// tailingLimit=1 so open_files_limit is strictly 1 active tailer.
		l := NewLauncher(1, 10*time.Millisecond, false, time.Hour, WildcardModeByName, flareController.NewFlareController(), nil, opener.NewFileOpener(), filetailer.NewFingerprinterMock())
		l.pipelineProvider = prov
		l.routedProvider = prov
		l.registry = reg

		srcCT := sources.NewLogSource("ct", &config.LogsConfig{Type: config.FileType, Path: pathCopyTrunc, TailingMode: "beginning"})
		l.activeSources = append(l.activeSources, srcCT)
		fileCT := filetailer.NewFile(pathCopyTrunc, srcCT, false)
		l.resolveActiveTailers([]*filetailer.File{fileCT})

		for range 2 {
			select {
			case <-prov.received:
			case <-time.After(5 * time.Second):
				t.Fatal("timed out waiting for initial copytrunc lines")
			}
		}
		reg.SetOffset("file:"+pathCopyTrunc, strconv.Itoa(len(initial)))

		// Fill routedRotationPending to max-1 (255 + 1 active tailer == 256 conservation cap).
		for i := 0; i < 255; i++ {
			l.recordRoutedRotationPending("dummy-"+strconv.Itoa(i), &oldTailerInfo{}, srcCT, "", 0, "")
		}
		require.Equal(t, 256, len(l.routedRotationPending)+l.tailers.Count())

		// Perform same-inode copytruncate (shrink file below old offset 22).
		require.NoError(t, os.Truncate(pathCopyTrunc, 0))
		f, err := os.OpenFile(pathCopyTrunc, os.O_WRONLY|os.O_TRUNC, 0o600)
		require.NoError(t, err)
		_, err = f.WriteString("short\n")
		require.NoError(t, err)
		require.NoError(t, f.Close())

		// Scan detects copytruncate; SourceInput fails due to persistent maxActiveInputs=1,
		// oldTailer is IMMEDIATELY stopped & removed, and copytrunc.log is recorded in
		// routedRotationPending (256 + 0 == 256).
		l.resolveActiveTailers([]*filetailer.File{fileCT})
		require.Contains(t, l.routedRotationPending, fileCT.GetScanKey())
		require.Equal(t, 0, l.tailers.Count())
		require.Equal(t, 256, len(l.routedRotationPending)+l.tailers.Count())

		// Before next scan, regrow the SAME inode PAST its old offset 22!
		f, err = os.OpenFile(pathCopyTrunc, os.O_WRONLY|os.O_TRUNC, 0o600)
		require.NoError(t, err)
		_, err = f.WriteString("regrown-prefix-1\nregrown-suffix-2\n")
		require.NoError(t, err)
		require.NoError(t, f.Close())

		// Wait for oldTailer to finish draining and retire generation 1 (freeing persistent capacity!).
		require.Eventually(t, func() bool {
			l.cleanUpRotatedTailers()
			prov.mu.Lock()
			defer prov.mu.Unlock()
			return len(prov.inputs) == 0
		}, 5*time.Second, 20*time.Millisecond, "old rotated tailer must retire to free persistent generation capacity")

		// Next scan starts copytrunc.log from byte 0 (delivering regrown prefix+suffix despite regrowth past offset 22!).
		l.resolveActiveTailers([]*filetailer.File{fileCT})
		require.LessOrEqual(t, l.tailers.Count(), 1, "open_files_limit must never be exceeded")
		got := make(map[string]bool)
		deadline := time.After(5 * time.Second)
		for !(got["regrown-prefix-1"] && got["regrown-suffix-2"]) {
			select {
			case msg := <-prov.received:
				got[string(msg.GetContent())] = true
			case <-deadline:
				t.Fatalf("timed out waiting for regrown copytruncate prefix+suffix; got=%v", got)
			}
		}

		// Verify unrelated files with valid saved offset (18) and forceEnd are completely unaffected
		// once capacity allows admitting them.
		prov.mu.Lock()
		prov.maxActiveInputs = 10
		prov.mu.Unlock()
		l.tailingLimit = 10
		l.routedRotationPending = nil // clear dummy entries so brand-new unrelated files can be admitted

		srcUnrelated := sources.NewLogSource("unrelated", &config.LogsConfig{Type: config.FileType, Path: pathUnrelated, TailingMode: "beginning"})
		fileUnrelated := filetailer.NewFile(pathUnrelated, srcUnrelated, false)
		srcForceEnd := sources.NewLogSource("force-end", &config.LogsConfig{Type: config.FileType, Path: pathForceEnd, TailingMode: "forceEnd"})
		l.activeSources = append(l.activeSources, srcUnrelated, srcForceEnd)
		l.launchTailers(srcForceEnd)
		fe, err := os.OpenFile(pathForceEnd, os.O_WRONLY|os.O_APPEND, 0o600)
		require.NoError(t, err)
		_, err = fe.WriteString("after-force-end\n")
		require.NoError(t, err)
		require.NoError(t, fe.Close())

		l.resolveActiveTailers([]*filetailer.File{fileCT, fileUnrelated, filetailer.NewFile(pathForceEnd, srcForceEnd, false)})
		got2 := make(map[string]bool)
		deadline2 := time.After(5 * time.Second)
		for !(got2["new-tail-line"] && got2["after-force-end"]) {
			select {
			case msg := <-prov.received:
				text := string(msg.GetContent())
				require.NotEqual(t, "already-sent-line", text, "unrelated file with valid checkpoint must not replay from byte 0")
				require.NotEqual(t, "pre-existing-should-skip", text, "unrelated forceEnd source must not read pre-existing content")
				got2[text] = true
			case <-deadline2:
				t.Fatalf("timed out waiting for unrelated + forceEnd lines; got=%v", got2)
			}
		}
		l.cleanup()
	})

	t.Run("permanently deleted wildcard paths are reclaimed even under repeated hitFileLimit scans", func(t *testing.T) {
		dir := t.TempDir()
		freshPath := filepath.Join(dir, "fresh.log")
		require.NoError(t, os.WriteFile(freshPath, []byte("fresh-after-churn\n"), 0o600))

		prov := newFakeRoutedFileProvider()
		baseReg := auditorMock.NewMockRegistry()
		var flushCalls int
		flushReg := &trackingFlushRegistry{
			Registry: baseReg,
			onFlush:  func() error { flushCalls++; return nil },
		}
		prov.registry = flushReg
		// tailingLimit=1 so every scan with [freshFile] hits hitFileLimit=true!
		l := NewLauncher(1, 10*time.Millisecond, false, time.Hour, WildcardModeByName, flareController.NewFlareController(), nil, opener.NewFileOpener(), filetailer.NewFingerprinterMock())
		l.pipelineProvider = prov
		l.routedProvider = prov
		l.registry = flushReg

		// Active wildcard source remains configured throughout.
		srcWildcard := sources.NewLogSource("wild", &config.LogsConfig{Type: config.FileType, Path: filepath.Join(dir, "*.log"), TailingMode: "beginning"})
		l.activeSources = []*sources.LogSource{srcWildcard}

		// Fill routedRotationPending to cap (256) with permanently deleted paths belonging to the active wildcard source.
		for i := 0; i < 256; i++ {
			deletedPath := filepath.Join(dir, "deleted-"+strconv.Itoa(i)+".log")
			l.recordRoutedRotationPending(deletedPath, &oldTailerInfo{}, srcWildcard, "file:"+deletedPath, 0, deletedPath)
		}
		require.Len(t, l.routedRotationPending, 256)

		freshFile := filetailer.NewFile(freshPath, srcWildcard, false)

		// Scan 1 (hitFileLimit=true): publishes FIFO offset "0" reset for absent deleted paths.
		l.resolveActiveTailers([]*filetailer.File{freshFile})
		require.Equal(t, 0, l.tailers.Count())
		require.Len(t, l.routedRotationPending, 256)
		require.Equal(t, 0, flushCalls)

		// Scan 2 (hitFileLimit=true): performs ONE batched FlushWithError for all 256 entries and reclaims them!
		l.resolveActiveTailers([]*filetailer.File{freshFile})
		require.Empty(t, l.routedRotationPending, "permanently deleted wildcard paths must be reclaimed even under hitFileLimit")
		require.Equal(t, 1, flushCalls, "reclaiming 256 pending entries must trigger exactly one batched registry flush")

		// Scan 3 (hitFileLimit=true): freshFile is now admitted without any manual map clearing!
		l.resolveActiveTailers([]*filetailer.File{freshFile})
		require.Equal(t, 1, l.tailers.Count())
		select {
		case msg := <-prov.received:
			require.Equal(t, "fresh-after-churn", string(msg.GetContent()))
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for fresh wildcard file after deleted entries were reclaimed")
		}
		l.cleanup()
	})

	t.Run("delayed replacement reappearing after reclamation still starts at byte zero (with or without prior checkpoint and on end-mode launchTailers)", func(t *testing.T) {
		dir := t.TempDir()
		pathWithCkpt := filepath.Join(dir, "delayed_ckpt.log")
		pathNoCkpt := filepath.Join(dir, "delayed_nockpt.log")

		prov := newFakeRoutedFileProvider()
		reg := auditorMock.NewMockRegistry()
		prov.registry = reg
		// pathWithCkpt had an old inode offset 18; pathNoCkpt had NO prior checkpoint ("").
		reg.SetOffset("file:"+pathWithCkpt, "18")

		l := NewLauncher(10, 10*time.Millisecond, false, time.Hour, WildcardModeByName, flareController.NewFlareController(), nil, opener.NewFileOpener(), filetailer.NewFingerprinterMock())
		l.pipelineProvider = prov
		l.routedProvider = prov
		l.registry = reg

		srcWithCkpt := sources.NewLogSource("ckpt", &config.LogsConfig{Type: config.FileType, Path: pathWithCkpt, TailingMode: "beginning"})
		srcNoCkpt := sources.NewLogSource("nockpt", &config.LogsConfig{Type: config.FileType, Path: pathNoCkpt}) // default end mode!
		l.activeSources = []*sources.LogSource{srcWithCkpt, srcNoCkpt}

		fileWithCkpt := filetailer.NewFile(pathWithCkpt, srcWithCkpt, false)
		fileNoCkpt := filetailer.NewFile(pathNoCkpt, srcNoCkpt, false)
		l.recordRoutedRotationPending(fileWithCkpt.GetScanKey(), &oldTailerInfo{}, srcWithCkpt, fileWithCkpt.Identifier(), 0, pathWithCkpt)
		l.recordRoutedRotationPending(fileNoCkpt.GetScanKey(), &oldTailerInfo{}, srcNoCkpt, fileNoCkpt.Identifier(), 0, pathNoCkpt)

		// Both files are absent across multiple scans -> reclaimed after verified offset "0" reset!
		l.resolveActiveTailers(nil)
		l.resolveActiveTailers(nil)
		require.Empty(t, l.routedRotationPending)
		require.Equal(t, "0", reg.GetOffset(fileWithCkpt.Identifier()))
		require.Equal(t, "0", reg.GetOffset(fileNoCkpt.Identifier()))

		// Long after reclamation, both replacement files appear larger than old offset 18.
		require.NoError(t, os.WriteFile(pathWithCkpt, []byte("ckpt-prefix-12345\nckpt-suffix-67890\n"), 0o600))
		require.NoError(t, os.WriteFile(pathNoCkpt, []byte("nockpt-prefix-123\nnockpt-suffix-456\n"), 0o600))

		// Re-add no-checkpoint file in default end mode via launchTailers BEFORE scan,
		// and start with-checkpoint file via ordinary scan!
		l.launchTailers(srcNoCkpt)
		l.resolveActiveTailers([]*filetailer.File{fileWithCkpt, fileNoCkpt})

		got := make(map[string]bool)
		deadline := time.After(5 * time.Second)
		for !(got["ckpt-prefix-12345"] && got["ckpt-suffix-67890"] && got["nockpt-prefix-123"] && got["nockpt-suffix-456"]) {
			select {
			case msg := <-prov.received:
				got[string(msg.GetContent())] = true
			case <-deadline:
				t.Fatalf("timed out waiting for delayed replacement prefixes+suffixes; got=%v", got)
			}
		}
		l.cleanup()
	})

	t.Run("fingerprint oldInfoMap entry reserves conservation slot against earlier fresh file in pass 2", func(t *testing.T) {
		dir := t.TempDir()
		freshPath := filepath.Join(dir, "fresh.log")
		fpPath := filepath.Join(dir, "fp_rotated.log")
		require.NoError(t, os.WriteFile(freshPath, []byte("fresh-line\n"), 0o600))
		require.NoError(t, os.WriteFile(fpPath, []byte("fp-replacement-line\n"), 0o600))

		prov := newFakeRoutedFileProvider()
		reg := auditorMock.NewMockRegistry()
		l := NewLauncher(10, 10*time.Millisecond, false, time.Hour, WildcardModeByName, flareController.NewFlareController(), nil, opener.NewFileOpener(), filetailer.NewFingerprinterMock())
		l.pipelineProvider = prov
		l.routedProvider = prov
		l.registry = reg

		src := sources.NewLogSource("wild", &config.LogsConfig{Type: config.FileType, Path: filepath.Join(dir, "*.log"), TailingMode: "beginning"})
		l.activeSources = []*sources.LogSource{src}

		// 255 routedRotationPending entries + 1 fingerprint oldInfoMap entry == 256 conservation cap.
		for i := 0; i < 255; i++ {
			l.recordRoutedRotationPending("dummy-"+strconv.Itoa(i), &oldTailerInfo{}, src, "", 0, "")
		}
		freshFile := filetailer.NewFile(freshPath, src, false)
		fpFile := filetailer.NewFile(fpPath, src, false)
		l.oldInfoMap[fpFile.GetScanKey()] = &oldTailerInfo{InfoRegistry: status.NewInfoRegistry()}

		// Scan presents freshFile FIRST, then fpFile SECOND: freshFile must NOT steal fpFile's slot!
		l.resolveActiveTailers([]*filetailer.File{freshFile, fpFile})
		require.Equal(t, 1, l.tailers.Count(), "only fpFile may start; freshFile must not bypass conservation bound")
		_, freshTailed := l.tailers.Get(freshFile.GetScanKey())
		_, fpTailed := l.tailers.Get(fpFile.GetScanKey())
		require.False(t, freshTailed)
		require.True(t, fpTailed)
		require.LessOrEqual(t, len(l.routedRotationPending)+len(l.oldInfoMap)+l.tailers.Count(), l.maxRoutedRotationPending())
		l.cleanup()
	})

	t.Run("delayed auditor apply retains pending entry until forwarded, flushed, and untailed", func(t *testing.T) {
		dir := t.TempDir()
		pathDelayed := filepath.Join(dir, "delayed_auditor.log")

		prov := newFakeRoutedFileProvider()
		prov.delayReset = true
		baseReg := auditorMock.NewMockRegistry()
		baseReg.SetOffset("file:"+pathDelayed, "18")
		baseReg.SetTailed("file:"+pathDelayed, true)

		var flushedCount int
		var failFlush bool
		var untailed bool
		flushReg := &trackingFlushRegistry{
			Registry: baseReg,
			onFlush: func() error {
				flushedCount++
				if failFlush {
					return os.ErrPermission
				}
				return nil
			},
			onTailed: func(id string, isTailed bool) {
				if id == "file:"+pathDelayed && !isTailed {
					untailed = true
				}
			},
		}
		prov.registry = flushReg

		l := NewLauncher(10, 10*time.Millisecond, false, time.Hour, WildcardModeByName, flareController.NewFlareController(), nil, opener.NewFileOpener(), filetailer.NewFingerprinterMock())
		l.pipelineProvider = prov
		l.routedProvider = prov
		l.registry = flushReg

		src := sources.NewLogSource("delayed", &config.LogsConfig{Type: config.FileType, Path: pathDelayed})
		l.activeSources = []*sources.LogSource{src}
		fileDelayed := filetailer.NewFile(pathDelayed, src, false)
		l.recordRoutedRotationPending(fileDelayed.GetScanKey(), &oldTailerInfo{}, src, fileDelayed.Identifier(), 0, pathDelayed)

		// Scan 1: enqueues reset, but auditor is delayed.
		l.resolveActiveTailers(nil)
		require.Contains(t, l.routedRotationPending, fileDelayed.GetScanKey())

		// Scans 2 & 3 while auditor is STILL delayed: entry MUST NOT be evicted!
		l.resolveActiveTailers(nil)
		l.resolveActiveTailers(nil)
		require.Contains(t, l.routedRotationPending, fileDelayed.GetScanKey(), "pending entry must not be evicted before auditor applies offset 0")
		require.Equal(t, "18", baseReg.GetOffset(fileDelayed.Identifier()))

		// Now auditor applies the reset, but disk persistence FAILS: entry MUST NOT be evicted!
		prov.flushDelayedResets()
		failFlush = true
		l.resolveActiveTailers(nil)
		require.Contains(t, l.routedRotationPending, fileDelayed.GetScanKey(), "pending entry must not be evicted when registry FlushWithError fails")
		require.Equal(t, 1, flushedCount)

		// Disk persistence recovers -> next scan flushes cleanly, clears tailed mark, verifies "0", and evicts!
		failFlush = false
		l.resolveActiveTailers(nil)
		require.Empty(t, l.routedRotationPending)
		require.Equal(t, "0", baseReg.GetOffset(fileDelayed.Identifier()))
		require.Equal(t, 2, flushedCount, "registry FlushWithError must retry and succeed before evicting pending entry")
		require.True(t, untailed, "stale tailed mark must be cleared so auditor_ttl can expire deleted paths")

		// Simulate restart/reappearance larger than old offset 18 in default end mode: starts at byte 0!
		require.NoError(t, os.WriteFile(pathDelayed, []byte("restarted-prefix-1\nrestarted-suffix-2\n"), 0o600))
		offset, whence, err := Position(baseReg, fileDelayed.Identifier(), config.End, filetailer.NewFingerprinterMock(), opener.NewFileOpener(), nil)
		require.NoError(t, err)
		require.Equal(t, int64(0), offset)
		require.Equal(t, io.SeekStart, whence)
		l.cleanup()
	})
}

type trackingFlushRegistry struct {
	auditor.Registry
	onFlush  func() error
	onTailed func(string, bool)
}

func (r *trackingFlushRegistry) FlushWithError() error {
	if r.onFlush != nil {
		return r.onFlush()
	}
	return nil
}

func (r *trackingFlushRegistry) SetTailed(id string, isTailed bool) {
	if r.onTailed != nil {
		r.onTailed(id, isTailed)
	}
	r.Registry.SetTailed(id, isTailed)
}
