// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module_test

import (
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	"github.com/DataDog/datadog-agent/pkg/dyninst/decode"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dispatcher"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/module"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
)

// TestHappyPathEndToEnd verifies the basic end-to-end flow where the module
// receives process updates from the scraper, forwards them to the actuator, and
// generates appropriate diagnostic messages for each probe.
func TestHappyPathEndToEnd(t *testing.T) {
	deps := newFakeTestingDependencies(t)
	deps.irGenerator.program = createTestProgram()
	processUpdate := createTestProcessConfig()
	_ = module.NewUnstartedModule(deps.toDeps())
	deps.sendUpdates(processUpdate)

	require.Len(t, deps.actuator.tenant.updates, 1)
	actualUpdate := deps.actuator.tenant.updates[0]
	require.Len(t, actualUpdate.Processes, 1)
	assert.Equal(t, processUpdate.ProcessID, actualUpdate.Processes[0].ProcessID)
	assert.Equal(t, processUpdate.Executable, actualUpdate.Processes[0].Executable)
	assert.Len(t, actualUpdate.Processes[0].Probes, 2)

	require.Len(t, deps.diagUploader.messages, 2)
	for _, msg := range deps.diagUploader.messages {
		assert.Equal(t, uploader.StatusReceived, msg.Debugger.Diagnostic.Status)
		assert.Contains(t, []string{"probe-1", "probe-2"}, msg.Debugger.Diagnostic.ProbeID)
	}
}

func makeFakeEvent(header output.EventHeader, data []byte) dispatcher.Message {
	header.Data_byte_len = uint32(unsafe.Sizeof(header)) + uint32(len(data))
	return dispatcher.MakeTestingMessage(append(
		append(([]byte)(nil), unsafe.Slice((*byte)(unsafe.Pointer(&header)), unsafe.Sizeof(header))...),
		data...,
	))
}

// TestProgramLifecycleFlow tests the complete program lifecycle including
// attachment, loading with metadata (git info, container info), and proper sink
// creation with the correct uploader metadata.
func TestProgramLifecycleFlow(t *testing.T) {
	decoder := &fakeDecoder{}
	program := createTestProgram()
	deps := newFakeTestingDependencies(t)
	deps.decoderFactory.decoder = decoder
	deps.irGenerator.program = program
	processUpdate := createTestProcessConfig()
	processUpdate.Container = process.ContainerInfo{ContainerID: "container-123", EntityID: "entity-123"}
	processUpdate.GitInfo = process.GitInfo{CommitSha: "commit-123", RepositoryURL: "https://github.com/test/test"}
	procID := processUpdate.ProcessID

	_ = module.NewUnstartedModule(deps.toDeps())

	collectVersions := func(status uploader.Status) map[string]int {
		return collectDiagnosticVersions(deps.diagUploader, status)
	}
	collectReceived := func() map[string]int { return collectVersions(uploader.StatusReceived) }
	collectInstalled := func() map[string]int { return collectVersions(uploader.StatusInstalled) }
	collectEmitting := func() map[string]int { return collectVersions(uploader.StatusEmitting) }

	deps.sendUpdates(processUpdate)

	initialProbeVersions := map[string]int{"probe-1": 1, "probe-2": 1}
	require.Equal(t, initialProbeVersions, collectReceived())

	loaded, err := deps.actuator.tenant.rt.Load(
		program.ID, processUpdate.Executable, procID, processUpdate.Probes,
	)
	require.NoError(t, err)

	sink := deps.dispatcher.sinks[program.ID]
	require.NotNil(t, sink)

	_, err = loaded.Attach(procID, processUpdate.Executable)
	require.NoError(t, err)
	require.Equal(t, initialProbeVersions, collectInstalled())

	decoder.probe = processUpdate.Probes[0]
	decoder.output = `{"test": "data"}`
	header := output.EventHeader{
		Goid:             1,
		Stack_byte_depth: 2,
		Probe_id:         3,
	}
	event := makeFakeEvent(header, []byte("event"))
	require.NoError(t, sink.HandleEvent(event))
	require.Len(t, decoder.decodeCalls, 1)

	require.Equal(t, map[string]int{"probe-1": 1}, collectEmitting())

	require.Len(t, deps.logsFactory.uploaders, 1)
	metadata := slices.Collect(maps.Keys(deps.logsFactory.uploaders))
	require.Equal(t, []uploader.LogsUploaderMetadata{{
		Tags:        "git.commit.sha:commit-123,git.repository_url:https://github.com/test/test",
		EntityID:    "entity-123",
		ContainerID: "container-123",
	}}, metadata)

	// Update first probe version and ensure diagnostics/log metadata follow.
	processUpdate.Probes[0].(*rcjson.LogProbe).Version++
	deps.sendUpdates(processUpdate)
	updatedProbeVersions := map[string]int{"probe-1": 2, "probe-2": 1}
	require.Equal(t, updatedProbeVersions, collectReceived())
	require.Equal(t, initialProbeVersions, collectInstalled())

	require.NoError(t, loaded.Close())

	program.Probes[0].ProbeDefinition = processUpdate.Probes[0]
	program.ID++
	update := deps.actuator.tenant.updates[len(deps.actuator.tenant.updates)-1]
	require.Len(t, update.Processes, 1)
	process := update.Processes[0]
	loaded2, err := deps.actuator.tenant.rt.Load(
		program.ID, process.Executable, process.ProcessID, process.Probes,
	)
	require.NoError(t, err)

	sink2 := deps.dispatcher.sinks[program.ID]
	require.NotNil(t, sink2)

	_, err = loaded2.Attach(procID, processUpdate.Executable)
	require.NoError(t, err)
	require.Equal(t, updatedProbeVersions, collectInstalled())
	require.Equal(t, map[string]int{"probe-1": 1}, collectEmitting())

	decoder.probe = processUpdate.Probes[0]
	require.NoError(t, sink2.HandleEvent(makeFakeEvent(header, []byte("event"))))
	require.Equal(t, map[string]int{"probe-1": 2}, collectEmitting())

	require.NoError(t, loaded2.Close())
}

// TestIRGenerationFailure verifies that IR generation failures are properly
// reported as error diagnostics for all affected probes.
func TestIRGenerationFailure(t *testing.T) {
	irErr := errors.New("IR generation failed")
	deps := newFakeTestingDependencies(t)
	deps.irGenerator.err = irErr
	processUpdate := createTestProcessConfig()
	_ = module.NewUnstartedModule(deps.toDeps())
	deps.sendUpdates(processUpdate)

	_, err := deps.actuator.tenant.rt.Load(
		ir.ProgramID(42),
		processUpdate.Executable,
		processUpdate.ProcessID,
		processUpdate.Probes,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), irErr.Error())

	errorCount := 0
	for _, msg := range deps.diagUploader.messages {
		if msg.Debugger.Diagnostic.Status == uploader.StatusError &&
			msg.Debugger.Diagnostic.DiagnosticException.Type == "IRGenFailed" {
			errorCount++
		}
	}
	require.Equal(t, len(processUpdate.Probes), errorCount)
}

// TestAttachmentFailure verifies that program attachment failures are properly
// reported as error diagnostics for all probes in the program.
func TestAttachmentFailure(t *testing.T) {
	deps := newFakeTestingDependencies(t)
	processUpdate := createTestProcessConfig()
	deps.irGenerator.program = createTestProgram()
	deps.attacher.err = errors.New("attachment failed")
	_ = module.NewUnstartedModule(deps.toDeps())

	deps.sendUpdates(processUpdate)

	loaded, err := deps.actuator.tenant.rt.Load(
		ir.ProgramID(42),
		processUpdate.Executable,
		processUpdate.ProcessID,
		processUpdate.Probes,
	)
	require.NoError(t, err)

	_, err = loaded.Attach(processUpdate.ProcessID, processUpdate.Executable)
	require.Error(t, err)

	errorCount := 0
	for _, msg := range deps.diagUploader.messages {
		if msg.Debugger.Diagnostic.Status == uploader.StatusError &&
			msg.Debugger.Diagnostic.DiagnosticException.Type == "AttachmentFailed" {
			errorCount++
		}
	}
	require.Equal(t, len(processUpdate.Probes), errorCount)
}

// TestLoadingFailure verifies that program loading failures are properly
// reported as error diagnostics for all probes in the program.
func TestLoadingFailure(t *testing.T) {
	deps := newFakeTestingDependencies(t)
	processUpdate := createTestProcessConfig()
	deps.irGenerator.program = createTestProgram()
	deps.kernelLoader.err = errors.New("loading failed")
	_ = module.NewUnstartedModule(deps.toDeps())

	deps.sendUpdates(processUpdate)

	_, err := deps.actuator.tenant.rt.Load(
		ir.ProgramID(42),
		processUpdate.Executable,
		processUpdate.ProcessID,
		processUpdate.Probes,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "loading failed")

	errorCount := 0
	for _, msg := range deps.diagUploader.messages {
		if msg.Debugger.Diagnostic.Status == uploader.StatusError &&
			msg.Debugger.Diagnostic.DiagnosticException.Type == "LoadingFailed" {
			errorCount++
		}
	}
	require.Equal(t, len(processUpdate.Probes), errorCount)
}

// TestDecoderCreationFailure verifies that decoder creation
// failures are properly handled and reported during program loading.
func TestDecoderCreationFailure(t *testing.T) {
	deps := newFakeTestingDependencies(t)
	processUpdate := createTestProcessConfig()
	deps.decoderFactory.err = errors.New("decoder creation failed")
	deps.irGenerator.program = createTestProgram()
	_ = module.NewUnstartedModule(deps.toDeps())

	deps.sendUpdates(processUpdate)

	_, err := deps.actuator.tenant.rt.Load(
		ir.ProgramID(42), processUpdate.Executable, processUpdate.ProcessID, processUpdate.Probes,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "decoder creation failed")

	errorCount := 0
	for _, msg := range deps.diagUploader.messages {
		if msg.Debugger.Diagnostic.Status == uploader.StatusError &&
			msg.Debugger.Diagnostic.DiagnosticException.Type == "LoadingFailed" {
			errorCount++
		}
	}
	require.Equal(t, len(processUpdate.Probes), errorCount)
}

// TestEventDecodingSuccess verifies successful event decoding, log uploading,
// and probe emitting diagnostic generation.
func TestEventDecodingSuccess(t *testing.T) {
	deps := newFakeTestingDependencies(t)
	decoder := &fakeDecoder{output: `{"test":"data"}`}
	processUpdate := createTestProcessConfig()
	deps.decoderFactory.decoder = decoder
	deps.irGenerator.program = createTestProgram()
	_ = module.NewUnstartedModule(deps.toDeps())

	deps.sendUpdates(processUpdate)

	loaded, err := deps.actuator.tenant.rt.Load(
		ir.ProgramID(42), processUpdate.Executable, processUpdate.ProcessID, processUpdate.Probes,
	)
	require.NoError(t, err)
	sink := deps.dispatcher.sinks[ir.ProgramID(42)]
	require.NotNil(t, sink)

	_, err = loaded.Attach(processUpdate.ProcessID, processUpdate.Executable)
	require.NoError(t, err)

	decoder.probe = processUpdate.Probes[0]
	event := makeFakeEvent(output.EventHeader{}, []byte("event"))
	require.NoError(t, sink.HandleEvent(event))
	require.Len(t, decoder.decodeCalls, 1)

	logsUploader, ok := deps.logsFactory.uploaders[uploader.LogsUploaderMetadata{}]
	require.True(t, ok)
	require.Len(t, logsUploader.messages, 1)
	assert.Equal(t, json.RawMessage(decoder.output), logsUploader.messages[0])

	emitting := collectDiagnosticVersions(deps.diagUploader, uploader.StatusEmitting)
	require.Equal(t, map[string]int{"probe-1": 1}, emitting)
}

// TestEventDecodingFailure verifies that event decoding failures are handled
// gracefully and reported as probe error diagnostics.
func TestEventDecodingFailure(t *testing.T) {
	deps := newFakeTestingDependencies(t)
	decoder := &fakeDecoder{probe: createTestProbe("probe-1"), err: errors.New("decode failed")}
	processUpdate := createTestProcessConfig()
	deps.decoderFactory.decoder = decoder
	deps.irGenerator.program = createTestProgram()
	_ = module.NewUnstartedModule(deps.toDeps())

	deps.sendUpdates(processUpdate)

	loaded, err := deps.actuator.tenant.rt.Load(
		ir.ProgramID(42), processUpdate.Executable, processUpdate.ProcessID, processUpdate.Probes,
	)
	require.NoError(t, err)
	sink := deps.dispatcher.sinks[ir.ProgramID(42)]
	require.NotNil(t, sink)

	_, err = loaded.Attach(processUpdate.ProcessID, processUpdate.Executable)
	require.NoError(t, err)

	require.NoError(t, sink.HandleEvent(makeFakeEvent(output.EventHeader{}, []byte("event"))))

	errorCount := 0
	for _, msg := range deps.diagUploader.messages {
		if msg.Debugger.Diagnostic.Status == uploader.StatusError &&
			msg.Debugger.Diagnostic.DiagnosticException.Type == "DecodeFailed" {
			errorCount++
		}
	}
	require.Equal(t, 1, errorCount)
}

func TestDecoderErrorHandling(t *testing.T) {
	deps := newFakeTestingDependencies(t)
	processUpdate := createTestProcessConfig()

	decoder := &fakeDecoder{output: `{"test":"data"}`}
	factory := &failOnceDecoderFactory{inner: &fakeDecoderFactory{decoder: decoder}}
	deps.irGenerator.program = createTestProgram()
	td := deps.toDeps()
	td.DecoderFactory = factory
	_ = module.NewUnstartedModule(td)

	deps.sendUpdates(processUpdate)
	received := collectDiagnosticVersions(deps.diagUploader, uploader.StatusReceived)
	require.Equal(t, map[string]int{"probe-1": 1, "probe-2": 1}, received)

	program := createTestProgram()
	loaded, err := deps.actuator.tenant.rt.Load(
		program.ID, processUpdate.Executable, processUpdate.ProcessID, processUpdate.Probes,
	)
	require.NoError(t, err)
	sink := deps.dispatcher.sinks[program.ID]
	require.NotNil(t, sink)

	_, err = loaded.Attach(processUpdate.ProcessID, processUpdate.Executable)
	require.NoError(t, err)

	decoder.probe = processUpdate.Probes[0]
	require.NoError(t, sink.HandleEvent(makeFakeEvent(output.EventHeader{}, nil)))

	errors := collectDiagnosticVersions(deps.diagUploader, uploader.StatusError)
	require.Equal(t, map[string]int{"probe-1": 1}, errors)

	decoder.probe = processUpdate.Probes[1]
	require.NoError(t, sink.HandleEvent(makeFakeEvent(output.EventHeader{}, nil)))

	emitting := collectDiagnosticVersions(deps.diagUploader, uploader.StatusEmitting)
	require.Equal(t, map[string]int{"probe-2": 1}, emitting)

	logsUploader, ok := deps.logsFactory.uploaders[uploader.LogsUploaderMetadata{}]
	require.True(t, ok)
	require.Len(t, logsUploader.messages, 1)
	assert.Equal(t, json.RawMessage(decoder.output), logsUploader.messages[0])
}

// TestProcessRemoval verifies that process removals are properly handled by
// updating the internal state and notifying the actuator.
func TestProcessRemoval(t *testing.T) {
	deps := newFakeTestingDependencies(t)
	processUpdate := createTestProcessConfig()
	removals := []process.ID{processUpdate.ProcessID}
	td := deps.toDeps()
	td.IRGenerator = irgen.NewGenerator()
	_ = module.NewUnstartedModule(td)

	deps.sendUpdates(processUpdate)
	require.Len(t, deps.actuator.tenant.updates, 1)

	deps.sendRemovals(removals...)

	require.Len(t, deps.actuator.tenant.updates, 2)
	require.Equal(t, deps.actuator.tenant.updates[0], actuator.ProcessesUpdate{
		Processes: []actuator.ProcessUpdate{
			{
				Info:   processUpdate.Info,
				Probes: processUpdate.Probes,
			},
		},
	})
	require.Equal(t, deps.actuator.tenant.updates[1], actuator.ProcessesUpdate{
		Removals: removals,
	})
}

// TestMultipleProcesses verifies that the controller can handle multiple
// processes in a single update, generating diagnostics for all probes across
// all processes.
func TestMultipleProcesses(t *testing.T) {
	deps := newFakeTestingDependencies(t)
	processUpdate1 := createTestProcessConfig()
	processUpdate1.ProcessID = process.ID{PID: 12345}
	processUpdate1.Service = "service-1"
	processUpdate1.RuntimeID = "runtime-1"

	processUpdate2 := createTestProcessConfig()
	processUpdate2.ProcessID = process.ID{PID: 67890}
	processUpdate2.Service = "service-2"
	processUpdate2.RuntimeID = "runtime-2"

	td := deps.toDeps()
	td.IRGenerator = irgen.NewGenerator()
	_ = module.NewUnstartedModule(td)

	deps.sendUpdates(processUpdate1, processUpdate2)

	require.Len(t, deps.actuator.tenant.updates, 1)
	actualUpdate := deps.actuator.tenant.updates[0]
	require.Len(t, actualUpdate.Processes, 2)

	assert.Len(t, deps.diagUploader.messages, 4)
	for _, msg := range deps.diagUploader.messages {
		assert.Equal(t, uploader.StatusReceived, msg.Debugger.Diagnostic.Status)
	}
}

// TestProbeIssueReporting verifies that probe issues in a program are properly
// reported as error diagnostics during the program loading phase.
func TestProbeIssueReporting(t *testing.T) {
	deps := newFakeTestingDependencies(t)
	decoder := &fakeDecoder{}
	processUpdate := createTestProcessConfig()
	program := &ir.Program{
		ID: ir.ProgramID(42),
		Probes: []*ir.Probe{
			{ProbeDefinition: createTestProbe("probe-1")},
		},
		Issues: []ir.ProbeIssue{
			{
				ProbeDefinition: createTestProbe("probe-2"),
				Issue: ir.Issue{
					Kind:    ir.IssueKindTargetNotFoundInBinary,
					Message: "target missing",
				},
			},
		},
	}

	deps.decoderFactory.decoder = decoder
	deps.irGenerator.program = program
	_ = module.NewUnstartedModule(deps.toDeps())

	deps.sendUpdates(processUpdate)

	loaded, err := deps.actuator.tenant.rt.Load(
		program.ID, processUpdate.Executable, processUpdate.ProcessID, processUpdate.Probes,
	)
	require.NoError(t, err)

	_, err = loaded.Attach(processUpdate.ProcessID, processUpdate.Executable)
	require.NoError(t, err)

	received := collectDiagnosticVersions(deps.diagUploader, uploader.StatusReceived)
	require.Equal(t, map[string]int{"probe-1": 1, "probe-2": 1}, received)

	// One probe should be marked installed, the other should surface an issue.
	installed := collectDiagnosticVersions(deps.diagUploader, uploader.StatusInstalled)
	require.Equal(t, map[string]int{"probe-1": 1}, installed)

	issueCount := 0
	for _, msg := range deps.diagUploader.messages {
		if msg.Debugger.Diagnostic.Status == uploader.StatusError &&
			msg.Debugger.Diagnostic.ProbeID == "probe-2" {
			assert.Equal(t, "TargetNotFoundInBinary", msg.Debugger.Diagnostic.DiagnosticException.Type)
			assert.Equal(t, "target missing", msg.Debugger.Diagnostic.DiagnosticException.Message)
			issueCount++
		}
	}
	require.Equal(t, 1, issueCount)
}

// TestNoSuccessfulProbes verifies that probe issues in a program are properly
// reported as error diagnostics during the program loading phase.
func TestNoSuccessfulProbes(t *testing.T) {
	processUpdate := createTestProcessConfig()
	fakeDeps := newFakeTestingDependencies(t)
	a := actuator.NewActuator()
	t.Cleanup(func() { require.NoError(t, a.Shutdown()) })
	deps := fakeDeps.toDeps()
	deps.IRGenerator = irgen.NewGenerator()
	deps.Actuator = module.EraseActuator(a)
	bin := testprogs.MustGetBinary(t, "simple", testprogs.MustGetCommonConfigs(t)[0])
	processUpdate.Executable = process.Executable{Path: bin}

	_ = module.NewUnstartedModule(deps)

	fakeDeps.sendUpdates(processUpdate)

	require.EventuallyWithT(t, func(t *assert.CollectT) {
		errorCount := 0
		for _, msg := range fakeDeps.diagUploader.messages {
			if msg.Debugger.Diagnostic.Status != uploader.StatusError {
				continue
			}
			errorCount++
			assert.Equal(t, "TargetNotFoundInBinary", msg.Debugger.Diagnostic.DiagnosticException.Type)
		}
		assert.Equal(t, 2, errorCount)
	}, 1*time.Second, 10*time.Millisecond)
}

type fakeDispatcher struct {
	sinks map[ir.ProgramID]dispatcher.Sink
}

func (f *fakeDispatcher) RegisterSink(progID ir.ProgramID, sink dispatcher.Sink) {
	f.sinks[progID] = sink
}

func (f *fakeDispatcher) UnregisterSink(progID ir.ProgramID) {
	delete(f.sinks, progID)
}

func (f *fakeDispatcher) Shutdown() error { return nil }

type fakeProcessSubscriber func(process.ProcessesUpdate)

func (f *fakeProcessSubscriber) Subscribe(cb func(process.ProcessesUpdate)) {
	*f = cb
}

type fakeActuatorTenant struct {
	name    string
	rt      actuator.Runtime
	updates []actuator.ProcessesUpdate
}

func (f *fakeActuatorTenant) HandleUpdate(update actuator.ProcessesUpdate) {
	f.updates = append(f.updates, update)
}

type fakeActuator struct {
	t      *testing.T
	tenant *fakeActuatorTenant
}

func (f *fakeActuator) Shutdown() error {
	return nil
}

func (f *fakeActuator) NewTenant(
	name string, rt actuator.Runtime,
) module.ActuatorTenant {
	assert.Nil(f.t, f.tenant)
	f.tenant = &fakeActuatorTenant{name: name, rt: rt}
	return f.tenant
}

type fakeProgramCompiler struct {
	err error
}

func (f *fakeProgramCompiler) GenerateProgram(*ir.Program) (compiler.Program, error) {
	if f.err != nil {
		return compiler.Program{}, f.err
	}
	return compiler.Program{}, nil
}

type fakeKernelLoader struct {
	err error
}

func (f *fakeKernelLoader) Load(compiler.Program) (*loader.Program, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &loader.Program{}, nil
}

type fakeAttacher struct {
	err error
}

func (f *fakeAttacher) Attach(
	_ *loader.Program,
	_ actuator.Executable,
	_ actuator.ProcessID,
) (actuator.AttachedProgram, error) {
	if f.err != nil {
		return nil, f.err
	}
	return fakeAttachedProgram{}, nil
}

type fakeAttachedProgram struct{}

func (fakeAttachedProgram) Detach() error { return nil }

type fakeIRGenerator struct {
	program *ir.Program
	err     error
}

func (f *fakeIRGenerator) GenerateIR(
	programID ir.ProgramID, _ string, _ []ir.ProbeDefinition,
) (*ir.Program, error) {
	if f == nil {
		return &ir.Program{ID: programID}, nil
	}
	if f.err != nil {
		return nil, f.err
	}
	if f.program != nil {
		return f.program, nil
	}
	return &ir.Program{ID: programID}, nil
}

type fakeDecoderFactory struct {
	decoder module.Decoder
	err     error
}

func (f *fakeDecoderFactory) NewDecoder(
	_ *ir.Program, _ process.Executable,
) (module.Decoder, error) {
	return f.decoder, f.err
}

type failOnceDecoderFactory struct {
	inner  module.DecoderFactory
	failed atomic.Bool
}

func (f *failOnceDecoderFactory) NewDecoder(
	program *ir.Program,
	executable process.Executable,
) (module.Decoder, error) {
	dec, err := f.inner.NewDecoder(program, executable)
	if err != nil {
		return nil, err
	}
	return &failOnceDecoder{inner: dec, failed: &f.failed}, nil
}

type failOnceDecoder struct {
	inner  module.Decoder
	failed *atomic.Bool
}

func (d *failOnceDecoder) Decode(
	event decode.Event,
	symbolicator symbol.Symbolicator,
	out []byte,
) ([]byte, ir.ProbeDefinition, error) {
	bytes, probe, err := d.inner.Decode(event, symbolicator, out)
	if err != nil {
		return bytes, probe, err
	}
	if d.failed.CompareAndSwap(false, true) {
		return bytes, probe, errors.New("boom")
	}
	return bytes, probe, nil
}

type fakeDecoder struct {
	probe  ir.ProbeDefinition
	err    error
	output string

	decodeCalls []decodeCall
}

type decodeCall struct {
	event        decode.Event
	symbolicator symbol.Symbolicator
	out          []byte
}

func (f *fakeDecoder) Decode(
	event decode.Event, symbolicator symbol.Symbolicator, out []byte,
) ([]byte, ir.ProbeDefinition, error) {
	f.decodeCalls = append(f.decodeCalls, decodeCall{event, symbolicator, out})
	return []byte(f.output), f.probe, f.err
}

type fakeDiagnosticsUploader struct {
	messages []*uploader.DiagnosticMessage
}

func (f *fakeDiagnosticsUploader) Enqueue(diag *uploader.DiagnosticMessage) error {
	f.messages = append(f.messages, diag)
	return nil
}

func (f *fakeDiagnosticsUploader) Stop() {}

type fakeLogsUploaderFactory struct {
	uploaders map[uploader.LogsUploaderMetadata]*fakeLogsUploader
}

func (f *fakeLogsUploaderFactory) Stop() {}

func (f *fakeLogsUploaderFactory) GetUploader(
	metadata uploader.LogsUploaderMetadata,
) module.LogsUploader {
	if len(f.uploaders) > 0 {
		for _, uploader := range f.uploaders {
			return uploader
		}
	}
	if f.uploaders == nil {
		f.uploaders = make(map[uploader.LogsUploaderMetadata]*fakeLogsUploader)
	}
	ul, ok := f.uploaders[metadata]
	if !ok {
		ul = &fakeLogsUploader{}
		f.uploaders[metadata] = ul
	}
	return ul
}

type fakeLogsUploader struct {
	messages []json.RawMessage
	closed   bool
}

func (f *fakeLogsUploader) Enqueue(data json.RawMessage) {
	f.messages = append(f.messages, data)
}

func (f *fakeLogsUploader) Close() {
	f.closed = true
}

type fakeActuatorAdapter struct {
	inner *fakeActuator
}

func (a fakeActuatorAdapter) Shutdown() error { return a.inner.Shutdown() }

func (a fakeActuatorAdapter) NewTenant(
	name string, rt actuator.Runtime,
) module.ActuatorTenant {
	return a.inner.NewTenant(name, rt)
}

type fakeTestingDependencies struct {
	actuator          *fakeActuator
	dispatcher        *fakeDispatcher
	diagUploader      *fakeDiagnosticsUploader
	logsFactory       *fakeLogsUploaderFactory
	programCompiler   *fakeProgramCompiler
	kernelLoader      *fakeKernelLoader
	attacher          *fakeAttacher
	decoderFactory    *fakeDecoderFactory
	irGenerator       *fakeIRGenerator
	objectLoader      object.Loader
	processesCallback func(process.ProcessesUpdate)
}

func newFakeTestingDependencies(t *testing.T) *fakeTestingDependencies {
	return &fakeTestingDependencies{
		actuator:        &fakeActuator{t: t},
		dispatcher:      &fakeDispatcher{sinks: make(map[ir.ProgramID]dispatcher.Sink)},
		diagUploader:    &fakeDiagnosticsUploader{},
		logsFactory:     &fakeLogsUploaderFactory{},
		programCompiler: &fakeProgramCompiler{},
		kernelLoader:    &fakeKernelLoader{},
		attacher:        &fakeAttacher{},
		decoderFactory:  &fakeDecoderFactory{},
		irGenerator:     &fakeIRGenerator{program: createTestProgram()},
		objectLoader:    object.NewInMemoryLoader(),
	}
}

func (d *fakeTestingDependencies) toDeps() module.Dependencies {
	return module.Dependencies{
		Actuator:            fakeActuatorAdapter{inner: d.actuator},
		Dispatcher:          d.dispatcher,
		DecoderFactory:      d.decoderFactory,
		IRGenerator:         d.irGenerator,
		ProgramCompiler:     d.programCompiler,
		KernelLoader:        d.kernelLoader,
		Attacher:            d.attacher,
		LogsFactory:         d.logsFactory,
		DiagnosticsUploader: d.diagUploader,
		ProcessSubscriber:   (*fakeProcessSubscriber)(&d.processesCallback),
	}
}

func (d *fakeTestingDependencies) sendUpdates(updates ...process.Config) {
	d.processesCallback(process.ProcessesUpdate{Updates: updates})
}

func (d *fakeTestingDependencies) sendRemovals(removals ...process.ID) {
	d.processesCallback(process.ProcessesUpdate{Removals: removals})
}

func collectDiagnosticVersions(
	u *fakeDiagnosticsUploader, status uploader.Status,
) map[string]int {
	versions := make(map[string]int)
	for _, msg := range u.messages {
		if msg.Debugger.Diagnostic.Status == status {
			versions[msg.Debugger.Diagnostic.ProbeID] = msg.Debugger.Diagnostic.ProbeVersion
		}
	}
	return versions
}

// Test data helpers.

func createTestProbe(id string) ir.ProbeDefinition {
	return &rcjson.LogProbe{
		LogProbeCommon: rcjson.LogProbeCommon{
			ProbeCommon: rcjson.ProbeCommon{
				ID:      id,
				Version: 1,
				Where:   &rcjson.Where{MethodName: "main"},
			},
			Template: "test log message",
		},
	}
}

func createTestProcessConfig() process.Config {
	return process.Config{
		Info: process.Info{
			ProcessID:  process.ID{PID: 12345},
			Executable: process.Executable{Path: "/usr/bin/test"},
			Service:    "test-service",
		},
		Probes: []ir.ProbeDefinition{
			createTestProbe("probe-1"), createTestProbe("probe-2"),
		},
		RuntimeID: "runtime-123",
	}
}

func createTestProgram() *ir.Program {
	return &ir.Program{
		ID: ir.ProgramID(42),
		Probes: []*ir.Probe{
			{ProbeDefinition: createTestProbe("probe-1")},
			{ProbeDefinition: createTestProbe("probe-2")},
		},
	}
}
