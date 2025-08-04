// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module_test

import (
	"encoding/json"
	"errors"
	"io"
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/decode"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/module"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcscrape"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
)

// Stub/fake implementations for testing.

type fakeScraper struct {
	updates []rcscrape.ProcessUpdate
}

func (f *fakeScraper) GetUpdates() []rcscrape.ProcessUpdate {
	return f.updates
}

type fakeActuatorTenant struct {
	name        string
	reporter    actuator.Reporter
	irGenerator actuator.IRGenerator
	updates     []actuator.ProcessesUpdate
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

func (f *fakeActuator) NewTenant(name string, reporter actuator.Reporter, irGenerator actuator.IRGenerator) *fakeActuatorTenant {
	assert.Nil(f.t, f.tenant)
	f.tenant = &fakeActuatorTenant{
		name:        name,
		reporter:    reporter,
		irGenerator: irGenerator,
	}
	return f.tenant
}

type fakeDecoderFactory struct {
	decoder module.Decoder
	err     error
}

func (f *fakeDecoderFactory) NewDecoder(_ *ir.Program) (module.Decoder, error) {
	return f.decoder, f.err
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
	out          io.Writer
}

func (f *fakeDecoder) Decode(event decode.Event, symbolicator symbol.Symbolicator, out io.Writer) (ir.ProbeDefinition, error) {
	f.decodeCalls = append(f.decodeCalls, decodeCall{event, symbolicator, out})
	if f.output != "" {
		_, err := io.WriteString(out, f.output)
		return f.probe, err
	}
	return f.probe, f.err
}

type fakeDiagnosticsUploader struct {
	messages []*uploader.DiagnosticMessage
}

func (f *fakeDiagnosticsUploader) Enqueue(diag *uploader.DiagnosticMessage) error {
	f.messages = append(f.messages, diag)
	return nil
}

type fakeLogsUploaderFactory struct {
	uploaders map[uploader.LogsUploaderMetadata]*fakeLogsUploader
}

func (f *fakeLogsUploaderFactory) GetUploader(metadata uploader.LogsUploaderMetadata) module.LogsUploader {
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

func createTestProcessUpdate() rcscrape.ProcessUpdate {
	return rcscrape.ProcessUpdate{
		ProcessUpdate: procmon.ProcessUpdate{
			ProcessID:  procmon.ProcessID{PID: 12345},
			Executable: procmon.Executable{Path: "/usr/bin/test"},
			Service:    "test-service",
		},
		Probes:    []ir.ProbeDefinition{createTestProbe("probe-1"), createTestProbe("probe-2")},
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

// TestController_HappyPathEndToEnd verifies the basic end-to-end flow where
// the controller receives process updates from the scraper, forwards them to
// the actuator, and generates appropriate diagnostic messages for each probe.
func TestController_HappyPathEndToEnd(t *testing.T) {
	scraper := &fakeScraper{}
	a := &fakeActuator{t: t}
	decoderFactory := &fakeDecoderFactory{}
	diagUploader := &fakeDiagnosticsUploader{}
	logUploaderFactory := &fakeLogsUploaderFactory{}

	processUpdate := createTestProcessUpdate()
	scraper.updates = []rcscrape.ProcessUpdate{processUpdate}

	controller := module.NewController(
		a, logUploaderFactory, diagUploader, scraper, decoderFactory,
	)
	require.NotNil(t, controller)

	controller.CheckForUpdates()

	require.Len(t, a.tenant.updates, 1)
	actualUpdate := a.tenant.updates[0]
	require.Len(t, actualUpdate.Processes, 1)
	assert.Equal(t, processUpdate.ProcessID, actualUpdate.Processes[0].ProcessID)
	assert.Equal(t, processUpdate.Executable, actualUpdate.Processes[0].Executable)
	assert.Len(t, actualUpdate.Processes[0].Probes, 2)

	require.Len(t, diagUploader.messages, 2)
	for _, msg := range diagUploader.messages {
		assert.Equal(t, uploader.StatusReceived, msg.Debugger.Diagnostic.Status)
		assert.Contains(t, []string{"probe-1", "probe-2"}, msg.Debugger.Diagnostic.ProbeID)
	}
}

// TestController_ProgramLifecycleFlow tests the complete program lifecycle
// including attachment, loading with metadata (git info, container info), and
// proper sink creation with the correct uploader metadata.
func TestController_ProgramLifecycleFlow(t *testing.T) {
	scraper := &fakeScraper{}
	a := &fakeActuator{t: t}
	decoder := &fakeDecoder{}
	decoderFactory := &fakeDecoderFactory{decoder: decoder}
	diagUploader := &fakeDiagnosticsUploader{}
	logUploaderFactory := &fakeLogsUploaderFactory{}

	processUpdate := createTestProcessUpdate()
	processUpdate.Container = procmon.ContainerInfo{
		ContainerID: "container-123",
		EntityID:    "entity-123",
	}
	processUpdate.GitInfo = procmon.GitInfo{
		CommitSha:     "commit-123",
		RepositoryURL: "https://github.com/test/test",
	}
	program := createTestProgram()
	procID := processUpdate.ProcessID

	scraper.updates = []rcscrape.ProcessUpdate{processUpdate}

	controller := module.NewController(
		a, logUploaderFactory, diagUploader, scraper, decoderFactory,
	)
	require.NotNil(t, controller)
	require.NotNil(t, a.tenant)
	require.NotNil(t, a.tenant.reporter)

	controller.CheckForUpdates()

	a.tenant.reporter.ReportAttached(procID, program)

	installedCount := 0
	for _, msg := range diagUploader.messages {
		if msg.Debugger.Diagnostic.Status == uploader.StatusInstalled {
			installedCount++
		}
	}
	assert.Equal(t, 2, installedCount)

	sink, err := a.tenant.reporter.ReportLoaded(procID, processUpdate.Executable, program)
	require.NoError(t, err)
	require.NotNil(t, sink)
	require.Len(t, logUploaderFactory.uploaders, 1)
	require.Equal(t,
		slices.Collect(maps.Keys(logUploaderFactory.uploaders)),
		[]uploader.LogsUploaderMetadata{{
			Tags:        "git.commit.sha:commit-123,git.repository_url:https://github.com/test/test",
			EntityID:    "entity-123",
			ContainerID: "container-123",
		}},
	)
}

// TestController_IRGenerationFailure verifies that IR generation failures
// are properly reported as error diagnostics for all affected probes.
func TestController_IRGenerationFailure(t *testing.T) {
	scraper := &fakeScraper{}
	a := &fakeActuator{}
	decoderFactory := &fakeDecoderFactory{}
	diagUploader := &fakeDiagnosticsUploader{}
	logUploaderFactory := &fakeLogsUploaderFactory{}

	processUpdate := createTestProcessUpdate()
	procID := processUpdate.ProcessID
	testError := errors.New("IR generation failed")

	scraper.updates = []rcscrape.ProcessUpdate{processUpdate}

	controller := module.NewController(
		a, logUploaderFactory, diagUploader, scraper, decoderFactory,
	)
	require.NotNil(t, controller)
	require.NotNil(t, a.tenant)
	require.NotNil(t, a.tenant.reporter)

	reporter := a.tenant.reporter

	controller.CheckForUpdates()

	reporter.ReportIRGenFailed(procID, testError, processUpdate.Probes)

	errorCount := 0
	for _, msg := range diagUploader.messages {
		if msg.Debugger.Diagnostic.Status == uploader.StatusError {
			assert.Equal(t, "IRGenFailed", msg.Debugger.Diagnostic.DiagnosticException.Type)
			assert.Equal(t, testError.Error(), msg.Debugger.Diagnostic.DiagnosticException.Message)
			errorCount++
		}
	}
	assert.Equal(t, 2, errorCount)
}

// TestController_AttachmentFailure verifies that program attachment failures
// are properly reported as error diagnostics for all probes in the program.
func TestController_AttachmentFailure(t *testing.T) {
	scraper := &fakeScraper{}
	a := &fakeActuator{}
	decoderFactory := &fakeDecoderFactory{}
	diagUploader := &fakeDiagnosticsUploader{}
	logUploaderFactory := &fakeLogsUploaderFactory{}

	processUpdate := createTestProcessUpdate()
	program := createTestProgram()
	procID := processUpdate.ProcessID
	testError := errors.New("attachment failed")

	scraper.updates = []rcscrape.ProcessUpdate{processUpdate}

	controller := module.NewController(
		a, logUploaderFactory, diagUploader, scraper, decoderFactory,
	)

	controller.CheckForUpdates()

	require.Len(t, diagUploader.messages, 2)
	for _, msg := range diagUploader.messages {
		assert.Equal(t, uploader.StatusReceived, msg.Debugger.Diagnostic.Status)
		assert.Contains(t, []string{"probe-1", "probe-2"}, msg.Debugger.Diagnostic.ProbeID)
	}

	a.tenant.reporter.ReportAttachingFailed(procID, program, testError)

	require.Len(t, diagUploader.messages, 4)
	errorCount := 0
	for _, msg := range diagUploader.messages {
		if msg.Debugger.Diagnostic.Status == uploader.StatusError &&
			msg.Debugger.Diagnostic.DiagnosticException.Type == "AttachmentFailed" {
			assert.Equal(t, testError.Error(), msg.Debugger.Diagnostic.DiagnosticException.Message)
			assert.Contains(t, []string{"probe-1", "probe-2"}, msg.Debugger.Diagnostic.ProbeID)
			errorCount++
		}
	}
	assert.Equal(t, 2, errorCount)
}

// TestController_LoadingFailure verifies that program loading failures
// are properly reported as error diagnostics for all probes in the program.
func TestController_LoadingFailure(t *testing.T) {
	scraper := &fakeScraper{}
	a := &fakeActuator{}
	decoderFactory := &fakeDecoderFactory{}
	diagUploader := &fakeDiagnosticsUploader{}
	logUploaderFactory := &fakeLogsUploaderFactory{}

	processUpdate := createTestProcessUpdate()
	program := createTestProgram()
	procID := processUpdate.ProcessID
	testError := errors.New("loading failed")

	scraper.updates = []rcscrape.ProcessUpdate{processUpdate}

	controller := module.NewController(a, logUploaderFactory, diagUploader, scraper, decoderFactory)

	controller.CheckForUpdates()

	require.Len(t, diagUploader.messages, 2)
	for _, msg := range diagUploader.messages {
		assert.Equal(t, uploader.StatusReceived, msg.Debugger.Diagnostic.Status)
		assert.Contains(t, []string{"probe-1", "probe-2"}, msg.Debugger.Diagnostic.ProbeID)
	}

	a.tenant.reporter.ReportLoadingFailed(procID, program, testError)

	require.Len(t, diagUploader.messages, 4)
	errorCount := 0
	for _, msg := range diagUploader.messages {
		if msg.Debugger.Diagnostic.Status == uploader.StatusError &&
			msg.Debugger.Diagnostic.DiagnosticException.Type == "LoadingFailed" {
			assert.Equal(t, testError.Error(), msg.Debugger.Diagnostic.DiagnosticException.Message)
			assert.Contains(t, []string{"probe-1", "probe-2"}, msg.Debugger.Diagnostic.ProbeID)
			errorCount++
		}
	}
	assert.Equal(t, 2, errorCount)
}

// TestController_DecoderCreationFailure verifies that decoder creation
// failures are properly handled and reported during program loading.
func TestController_DecoderCreationFailure(t *testing.T) {
	scraper := &fakeScraper{}
	a := &fakeActuator{}
	decoderFactory := &fakeDecoderFactory{err: errors.New("decoder creation failed")}
	diagUploader := &fakeDiagnosticsUploader{}
	logUploaderFactory := &fakeLogsUploaderFactory{}

	processUpdate := createTestProcessUpdate()
	program := createTestProgram()
	procID := processUpdate.ProcessID

	scraper.updates = []rcscrape.ProcessUpdate{processUpdate}

	controller := module.NewController(a, logUploaderFactory, diagUploader, scraper, decoderFactory)
	controller.CheckForUpdates()

	sink, err := a.tenant.reporter.ReportLoaded(procID, processUpdate.Executable, program)
	require.Error(t, err)
	require.Nil(t, sink)
	assert.Contains(t, err.Error(), "creating decoder")
}

// TestController_EventDecodingSuccess verifies successful event decoding,
// log uploading, and probe emitting diagnostic generation.
func TestController_EventDecodingSuccess(t *testing.T) {
	scraper := &fakeScraper{}
	a := &fakeActuator{}
	probe := createTestProbe("probe-1")
	decoder := &fakeDecoder{probe: probe, output: `{"test": "data"}`}
	decoderFactory := &fakeDecoderFactory{decoder: decoder}
	diagUploader := &fakeDiagnosticsUploader{}
	logUploaderFactory := &fakeLogsUploaderFactory{}

	processUpdate := createTestProcessUpdate()
	program := createTestProgram()
	procID := processUpdate.ProcessID

	scraper.updates = []rcscrape.ProcessUpdate{processUpdate}

	controller := module.NewController(a, logUploaderFactory, diagUploader, scraper, decoderFactory)

	controller.CheckForUpdates()

	sink, err := a.tenant.reporter.ReportLoaded(procID, processUpdate.Executable, program)
	require.NoError(t, err)
	require.NotNil(t, sink)

	testEvent := decode.Event{
		Event:       []byte("test event data"),
		ServiceName: processUpdate.Service,
	}
	err = sink.HandleEvent(testEvent.Event)
	require.NoError(t, err)
	require.Len(t, decoder.decodeCalls, 1)
	call := decoder.decodeCalls[0]
	require.Equal(t, testEvent, call.event)
	require.Len(t, logUploaderFactory.uploaders, 1)
	logUploader, ok := logUploaderFactory.uploaders[uploader.LogsUploaderMetadata{}]
	require.True(t, ok)

	require.Len(t, logUploader.messages, 1)
	assert.Equal(t, json.RawMessage(`{"test": "data"}`), logUploader.messages[0])

	emittingCount := 0
	for _, msg := range diagUploader.messages {
		if msg.Debugger.Diagnostic.Status == uploader.StatusEmitting &&
			msg.Debugger.Diagnostic.ProbeID == "probe-1" {
			emittingCount++
		}
	}
	assert.Equal(t, 1, emittingCount)
}

// TestController_EventDecodingFailure verifies that event decoding failures
// are handled gracefully and reported as probe error diagnostics.
func TestController_EventDecodingFailure(t *testing.T) {
	scraper := &fakeScraper{}
	a := &fakeActuator{}
	probe := createTestProbe("probe-1")
	decoder := &fakeDecoder{probe: probe, err: errors.New("decode failed")}
	decoderFactory := &fakeDecoderFactory{decoder: decoder}
	diagUploader := &fakeDiagnosticsUploader{}
	logUploaderFactory := &fakeLogsUploaderFactory{}

	processUpdate := createTestProcessUpdate()
	program := createTestProgram()
	procID := processUpdate.ProcessID

	scraper.updates = []rcscrape.ProcessUpdate{processUpdate}

	controller := module.NewController(a, logUploaderFactory, diagUploader, scraper, decoderFactory)

	controller.CheckForUpdates()

	sink, err := a.tenant.reporter.ReportLoaded(procID, processUpdate.Executable, program)
	require.NoError(t, err)
	require.NotNil(t, sink)

	testEvent := decode.Event{}
	err = sink.HandleEvent(testEvent.Event)
	require.NoError(t, err)

	errorCount := 0
	for _, msg := range diagUploader.messages {
		if msg.Debugger.Diagnostic.Status == uploader.StatusError &&
			msg.Debugger.Diagnostic.DiagnosticException.Type == "DecodeFailed" &&
			msg.Debugger.Diagnostic.ProbeID == "probe-1" {
			errorCount++
		}
	}
	assert.Equal(t, 1, errorCount)
}

// TestController_ProcessRemoval verifies that process removals are properly
// handled by updating the internal state and notifying the actuator.
func TestController_ProcessRemoval(t *testing.T) {
	scraper := &fakeScraper{}
	a := &fakeActuator{}
	decoderFactory := &fakeDecoderFactory{}
	diagUploader := &fakeDiagnosticsUploader{}
	logUploaderFactory := &fakeLogsUploaderFactory{}

	processUpdate := createTestProcessUpdate()
	removals := []procmon.ProcessID{processUpdate.ProcessID}

	scraper.updates = []rcscrape.ProcessUpdate{processUpdate}

	controller := module.NewController(a, logUploaderFactory, diagUploader, scraper, decoderFactory)
	at := a.tenant

	controller.CheckForUpdates()
	require.Len(t, at.updates, 1)

	controller.HandleRemovals(removals)

	require.Len(t, at.updates, 2)
	require.Equal(t, at.updates[0], actuator.ProcessesUpdate{
		Processes: []actuator.ProcessUpdate{
			{
				ProcessID:  processUpdate.ProcessID,
				Executable: processUpdate.Executable,
				Probes:     processUpdate.Probes,
			},
		},
	})
	require.Equal(t, at.updates[1], actuator.ProcessesUpdate{
		Removals: removals,
	})
}

// TestController_MultipleProcesses verifies that the controller can handle
// multiple processes in a single update, generating diagnostics for all probes
// across all processes.
func TestController_MultipleProcesses(t *testing.T) {
	scraper := &fakeScraper{}
	actuator := &fakeActuator{t: t}
	decoderFactory := &fakeDecoderFactory{}
	diagUploader := &fakeDiagnosticsUploader{}
	logUploaderFactory := &fakeLogsUploaderFactory{}

	processUpdate1 := createTestProcessUpdate()
	processUpdate1.ProcessID = procmon.ProcessID{PID: 12345}
	processUpdate1.Service = "service-1"
	processUpdate1.RuntimeID = "runtime-1"

	processUpdate2 := createTestProcessUpdate()
	processUpdate2.ProcessID = procmon.ProcessID{PID: 67890}
	processUpdate2.Service = "service-2"
	processUpdate2.RuntimeID = "runtime-2"

	scraper.updates = []rcscrape.ProcessUpdate{processUpdate1, processUpdate2}

	controller := module.NewController(actuator, logUploaderFactory, diagUploader, scraper, decoderFactory)

	controller.CheckForUpdates()

	require.Len(t, actuator.tenant.updates, 1)
	actualUpdate := actuator.tenant.updates[0]
	require.Len(t, actualUpdate.Processes, 2)

	assert.Len(t, diagUploader.messages, 4)
	for _, msg := range diagUploader.messages {
		assert.Equal(t, uploader.StatusReceived, msg.Debugger.Diagnostic.Status)
	}
}

// TestController_ProbeIssueReporting verifies that probe issues in a program
// are properly reported as error diagnostics during the program loading phase.
func TestController_ProbeIssueReporting(t *testing.T) {
	scraper := &fakeScraper{}
	a := &fakeActuator{}
	decoder := &fakeDecoder{}
	decoderFactory := &fakeDecoderFactory{decoder: decoder}
	diagUploader := &fakeDiagnosticsUploader{}
	logUploaderFactory := &fakeLogsUploaderFactory{}

	processUpdate := createTestProcessUpdate()
	procID := processUpdate.ProcessID

	// Create a program with probe issues
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
					Message: "target function not found in binary",
				},
			},
		},
	}

	scraper.updates = []rcscrape.ProcessUpdate{processUpdate}

	controller := module.NewController(
		a, logUploaderFactory, diagUploader, scraper, decoderFactory,
	)

	controller.CheckForUpdates()

	sink, err := a.tenant.reporter.ReportLoaded(procID, processUpdate.Executable, program)
	require.NoError(t, err)
	require.NotNil(t, sink)

	// Verify that probe-1 succeeded (received diagnostic) and probe-2 failed (error diagnostic)
	receivedCount := 0
	issueErrorCount := 0
	for _, msg := range diagUploader.messages {
		switch msg.Debugger.Diagnostic.Status {
		case uploader.StatusReceived:
			if msg.Debugger.Diagnostic.ProbeID == "probe-1" {
				receivedCount++
			}
		case uploader.StatusError:
			if msg.Debugger.Diagnostic.ProbeID == "probe-2" {
				assert.Equal(t, "TargetNotFoundInBinary", msg.Debugger.Diagnostic.DiagnosticException.Type)
				assert.Equal(t, "target function not found in binary", msg.Debugger.Diagnostic.DiagnosticException.Message)
				issueErrorCount++
			}
		}
	}
	assert.Equal(t, 1, receivedCount)
	assert.Equal(t, 1, issueErrorCount)
}

// TestController_NoSuccessfulProbesError verifies that probe issues in a program
// are properly reported as error diagnostics during the program loading phase.
func TestController_NoSuccessfulProbesError(t *testing.T) {
	scraper := &fakeScraper{}
	a := &fakeActuator{}
	decoder := &fakeDecoder{}
	decoderFactory := &fakeDecoderFactory{decoder: decoder}
	diagUploader := &fakeDiagnosticsUploader{}
	logUploaderFactory := &fakeLogsUploaderFactory{}

	processUpdate := createTestProcessUpdate()
	procID := processUpdate.ProcessID
	scraper.updates = []rcscrape.ProcessUpdate{processUpdate}

	controller := module.NewController(
		a, logUploaderFactory, diagUploader, scraper, decoderFactory,
	)

	controller.CheckForUpdates()

	require.Len(t, diagUploader.messages, 2)
	for _, msg := range diagUploader.messages {
		assert.Equal(t, uploader.StatusReceived, msg.Debugger.Diagnostic.Status)
		assert.Contains(t, []string{"probe-1", "probe-2"}, msg.Debugger.Diagnostic.ProbeID)
	}

	testError := &actuator.NoSuccessfulProbesError{
		Issues: []ir.ProbeIssue{
			{
				ProbeDefinition: processUpdate.Probes[0],
				Issue: ir.Issue{
					Kind:    ir.IssueKindTargetNotFoundInBinary,
					Message: "boom 1",
				},
			},
			{
				ProbeDefinition: processUpdate.Probes[1],
				Issue: ir.Issue{
					Kind:    ir.IssueKindUnsupportedFeature,
					Message: "boom 2",
				},
			},
		},
	}

	a.tenant.reporter.ReportIRGenFailed(procID, testError, processUpdate.Probes)

	require.Len(t, diagUploader.messages, 4)
	errorCount := 0
	for _, msg := range diagUploader.messages {
		if msg.Debugger.Diagnostic.Status != uploader.StatusError {
			continue
		}
		switch msg.Debugger.Diagnostic.ProbeID {
		case "probe-1":
			assert.Equal(t, "TargetNotFoundInBinary", msg.Debugger.Diagnostic.DiagnosticException.Type)
			assert.Equal(t, "boom 1", msg.Debugger.Diagnostic.DiagnosticException.Message)
		case "probe-2":
			assert.Equal(t, "UnsupportedFeature", msg.Debugger.Diagnostic.DiagnosticException.Type)
			assert.Equal(t, "boom 2", msg.Debugger.Diagnostic.DiagnosticException.Message)
		default:
			t.Fatalf("unexpected probe ID: %s", msg.Debugger.Diagnostic.ProbeID)
		}
		errorCount++
	}
	assert.Equal(t, 2, errorCount)
}
