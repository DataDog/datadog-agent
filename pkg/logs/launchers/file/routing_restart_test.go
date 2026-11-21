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
	mu            sync.Mutex
	guarded       bool
	failNextInput int
	failedInputs  int
	inputs        map[sourceGeneration]chan *message.Message
	retired       map[sourceGeneration]bool
	received      chan *message.Message
	retireEvent   chan sourceGeneration
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
		require.True(t, l.tryRecordRoutedRotationPending(fileA.GetScanKey(), &oldTailerInfo{InfoRegistry: status.NewInfoRegistry()}))

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
		fileA := filetailer.NewFile(pathA, srcA, false)
		require.True(t, l.tryRecordRoutedRotationPending(fileA.GetScanKey(), &oldTailerInfo{InfoRegistry: status.NewInfoRegistry()}))

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
		fileA := filetailer.NewFile(pathA, srcA, false)
		require.True(t, l.tryRecordRoutedRotationPending(fileA.GetScanKey(), &oldTailerInfo{InfoRegistry: status.NewInfoRegistry()}))

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

	t.Run("cap deferral preserves no-checkpoint rotation on end-mode re-add without affecting unrelated files", func(t *testing.T) {
		mockCfg := configmock.New(t)
		mockCfg.SetInTest("logs_config.close_timeout", 1)
		dir := t.TempDir()
		pathRot := filepath.Join(dir, "rot_no_ckpt.log")
		pathUnrelated := filepath.Join(dir, "unrelated.log")
		pathForceEnd := filepath.Join(dir, "force_end.log")
		require.NoError(t, os.WriteFile(pathRot, []byte("old-before-ckpt\n"), 0o600))
		require.NoError(t, os.WriteFile(pathUnrelated, []byte("already-sent-line\nnew-tail-line\n"), 0o600))
		require.NoError(t, os.WriteFile(pathForceEnd, []byte("pre-existing-should-skip\n"), 0o600))

		prov := newFakeRoutedFileProvider()
		reg := auditorMock.NewMockRegistry()
		// pathRot has NO saved checkpoint in reg!
		// pathUnrelated has a valid saved offset right after "already-sent-line\n" (18 bytes).
		reg.SetOffset("file:"+pathUnrelated, "18")
		reg.SetOffset("file:"+pathForceEnd, "5")

		l := NewLauncher(10, 10*time.Millisecond, false, time.Hour, WildcardModeByName, flareController.NewFlareController(), nil, opener.NewFileOpener(), filetailer.NewFingerprinterMock())
		l.pipelineProvider = prov
		l.routedProvider = prov
		l.registry = reg

		// Start tailing pathRot (default end/beginning before any checkpoint is persisted).
		srcRot := sources.NewLogSource("rot", &config.LogsConfig{Type: config.FileType, Path: pathRot, TailingMode: "beginning"})
		l.activeSources = append(l.activeSources, srcRot)
		fileRot := filetailer.NewFile(pathRot, srcRot, false)
		l.resolveActiveTailers([]*filetailer.File{fileRot})
		select {
		case msg := <-prov.received:
			require.Equal(t, "old-before-ckpt", string(msg.GetContent()))
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for initial line on pathRot")
		}

		// Fill routedRotationPending to its cap (256) with distinct scan keys.
		for i := 0; i < 256; i++ {
			require.True(t, l.tryRecordRoutedRotationPending("dummy-"+strconv.Itoa(i), &oldTailerInfo{}))
		}
		require.Len(t, l.routedRotationPending, 256)

		// Force SourceInput to fail once on rotation, then rotate pathRot (with NO checkpoint in reg!).
		prov.mu.Lock()
		prov.failNextInput = 1
		prov.mu.Unlock()
		require.NoError(t, os.Rename(pathRot, pathRot+".1"))
		require.NoError(t, os.WriteFile(pathRot, []byte("replacement-line-1\nreplacement-line-2\n"), 0o600))

		// Scan detects rotation; SourceInput fails AND routedRotationPending is full -> deferred in-place!
		l.resolveActiveTailers([]*filetailer.File{fileRot})
		require.Contains(t, l.deferredRotations, fileRot.GetScanKey(), "at-cap rotation must defer in-place")
		require.Len(t, l.routedRotationPending, 256, "map size must stay strictly bounded")

		// Even if an empty scan omits pathRot while its source is active, deferred tailer stays alive.
		l.resolveActiveTailers(nil)
		require.Contains(t, l.deferredRotations, fileRot.GetScanKey())

		// Re-adding pathRot in default end mode before the next scan replaces the source on the
		// deferred tailer rather than starting a new tailer at EOF!
		srcRotReadd := sources.NewLogSource("rot", &config.LogsConfig{Type: config.FileType, Path: pathRot})
		l.activeSources = []*sources.LogSource{srcRotReadd}
		l.launchTailers(srcRotReadd)

		// Meanwhile, an unrelated file with a valid saved offset (18) must NOT replay from byte 0,
		// and an unrelated forceEnd source must NOT read pre-existing content!
		srcUnrelated := sources.NewLogSource("unrelated", &config.LogsConfig{Type: config.FileType, Path: pathUnrelated, TailingMode: "beginning"})
		fileUnrelated := filetailer.NewFile(pathUnrelated, srcUnrelated, false)
		srcForceEnd := sources.NewLogSource("force-end", &config.LogsConfig{Type: config.FileType, Path: pathForceEnd, TailingMode: "forceEnd"})
		l.activeSources = append(l.activeSources, srcUnrelated, srcForceEnd)
		l.launchTailers(srcForceEnd)
		f, err := os.OpenFile(pathForceEnd, os.O_WRONLY|os.O_APPEND, 0o600)
		require.NoError(t, err)
		_, err = f.WriteString("after-force-end\n")
		require.NoError(t, err)
		require.NoError(t, f.Close())

		// Next scan with SourceInput capacity restored rotates pathRot from beginning AND starts pathUnrelated at offset 18.
		fileRotReadd := filetailer.NewFile(pathRot, srcRotReadd, false)
		l.resolveActiveTailers([]*filetailer.File{fileRotReadd, fileUnrelated, filetailer.NewFile(pathForceEnd, srcForceEnd, false)})

		got := make(map[string]bool)
		deadline := time.After(5 * time.Second)
		for !(got["replacement-line-1"] && got["replacement-line-2"] && got["new-tail-line"] && got["after-force-end"]) {
			select {
			case msg := <-prov.received:
				text := string(msg.GetContent())
				require.NotEqual(t, "already-sent-line", text, "unrelated file with valid checkpoint must not replay from byte 0")
				require.NotEqual(t, "pre-existing-should-skip", text, "unrelated forceEnd source must not read pre-existing content")
				got[text] = true
			case <-deadline:
				t.Fatalf("timed out waiting for replacement + unrelated lines; got=%v", got)
			}
		}
		require.NotContains(t, l.deferredRotations, fileRot.GetScanKey())
		l.cleanup()
	})
}
