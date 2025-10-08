// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"errors"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uprobe"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type runtimeImpl struct {
	store                    *processStore
	diagnostics              *diagnosticsManager
	decoderFactory           DecoderFactory
	irGenerator              IRGenerator
	programCompiler          ProgramCompiler
	kernelLoader             KernelLoader
	attacher                 Attacher
	dispatcher               Dispatcher
	logsFactory              erasedLogsUploaderFactory
	procRuntimeIDbyProgramID *sync.Map
	bufferedMessageTracker   *bufferedMessageTracker
}

type irGenFailedError struct {
	err error
}

func (e *irGenFailedError) Error() string { return e.err.Error() }

func (e *irGenFailedError) Unwrap() error { return e.err }

type irIssueError ir.Issue

func (e *irIssueError) Error() string { return e.Message }

func (rt *runtimeImpl) Load(
	programID ir.ProgramID,
	executable actuator.Executable,
	processID actuator.ProcessID,
	probes []ir.ProbeDefinition,
) (_ actuator.LoadedProgram, retErr error) {
	runtimeID, ok := rt.store.updateOnLoad(processID, executable, programID)
	if !ok {
		return nil, nil
	}

	rt.procRuntimeIDbyProgramID.Store(programID, runtimeID)
	defer func() {
		if retErr == nil {
			return
		}
		rt.procRuntimeIDbyProgramID.Delete(programID)
		var irGenFailed *irGenFailedError
		var noSuccessfulProbes *ir.NoSuccessfulProbesError
		switch {
		case errors.As(retErr, &noSuccessfulProbes):
			for i := range noSuccessfulProbes.Issues {
				issue := &noSuccessfulProbes.Issues[i]
				issueErr := (*irIssueError)(&issue.Issue)
				if rt.diagnostics.reportError(runtimeID, issue.ProbeDefinition, issueErr, issue.Kind.String()) {
					log.Debugf(
						"reported issue %v for probe %v %v: %v",
						issue.Kind, issue.ProbeDefinition.GetID(),
						issue.ProbeDefinition.GetVersion(), issueErr,
					)
				}
			}
		case errors.As(retErr, &irGenFailed):
			for _, probe := range probes {
				rt.diagnostics.reportError(runtimeID, probe, irGenFailed.err, "IRGenFailed")
			}
		default:
			for _, probe := range probes {
				rt.diagnostics.reportError(runtimeID, probe, retErr, "LoadingFailed")
			}
		}
	}()

	irProgram, err := rt.irGenerator.GenerateIR(programID, executable.Path, probes)
	if err != nil {
		return nil, &irGenFailedError{err: err}
	}

	compiled, err := rt.programCompiler.GenerateProgram(irProgram)
	if err != nil {
		return nil, fmt.Errorf("failed to generate program: %w", err)
	}
	loadedProgram, err := rt.kernelLoader.Load(compiled)
	if err != nil {
		return nil, fmt.Errorf("failed to load program: %w", err)
	}
	defer func() {
		if retErr != nil {
			loadedProgram.Close()
		}
	}()

	decoder, err := rt.decoderFactory.NewDecoder(irProgram, executable)
	if err != nil {
		return nil, fmt.Errorf("failed to create decoder: %w", err)
	}

	var tags string
	if gi := runtimeID.gitInfo; gi != nil {
		tags = fmt.Sprintf(
			"git.commit.sha:%s,git.repository_url:%s",
			gi.CommitSha, gi.RepositoryURL,
		)
	}
	var containerID, entityID string
	if ci := runtimeID.containerInfo; ci != nil {
		containerID = ci.ContainerID
		entityID = ci.EntityID
	}
	for i := range irProgram.Issues {
		issue := &irProgram.Issues[i]
		issueErr := (*irIssueError)(&issue.Issue)
		if rt.diagnostics.reportError(
			runtimeID, issue.ProbeDefinition, issueErr, issue.Kind.String(),
		) {
			log.Debugf(
				"reported issue %v for probe %v %v: %v",
				issue.Kind, issue.ProbeDefinition.GetID(),
				issue.ProbeDefinition.GetVersion(), issueErr,
			)
		}
	}

	s := &sink{
		runtime:      rt,
		decoder:      decoder,
		symbolicator: rt.store.getSymbolicator(programID),
		programID:    programID,
		service:      runtimeID.service,
		logUploader: rt.logsFactory.GetUploader(uploader.LogsUploaderMetadata{
			Tags:        tags,
			EntityID:    entityID,
			ContainerID: containerID,
		}),
		tree: rt.bufferedMessageTracker.newTree(),
	}
	rt.dispatcher.RegisterSink(programID, s)

	return &loadedProgramImpl{
		runtime:       rt,
		runtimeID:     runtimeID,
		programID:     programID,
		ir:            irProgram,
		executable:    executable,
		loadedProgram: loadedProgram,
	}, nil
}

type loadedProgramImpl struct {
	runtime       *runtimeImpl
	runtimeID     procRuntimeID
	programID     ir.ProgramID
	ir            *ir.Program
	executable    actuator.Executable
	loadedProgram *loader.Program
}

func (l *loadedProgramImpl) Attach(processID actuator.ProcessID, executable actuator.Executable) (actuator.AttachedProgram, error) {
	attached, err := l.runtime.attacher.Attach(l.loadedProgram, executable, processID)
	if err != nil {
		log.Errorf("rcscrape: failed to attach to process %v: %v", processID, err)
		l.runtime.reportAttachError(l.programID, l.runtimeID, l.ir, err)
		return nil, err
	}
	l.runtime.onProgramAttached(l.programID, processID, l.runtimeID, l.ir)
	return &attachedProgramImpl{
		runtime:   l.runtime,
		programID: l.programID,
		inner:     attached,
	}, nil
}

func (l *loadedProgramImpl) Close() error {
	l.loadedProgram.Close()
	l.runtime.dispatcher.UnregisterSink(l.programID)
	l.runtime.onProgramDetached(l.programID)
	return nil
}

func (l *loadedProgramImpl) IR() *ir.Program {
	return l.ir
}

type attachedProgramImpl struct {
	runtime   *runtimeImpl
	programID ir.ProgramID
	inner     actuator.AttachedProgram
}

func (a *attachedProgramImpl) Detach() error {
	err := a.inner.Detach()
	a.runtime.onProgramDetached(a.programID)
	return err
}

func (rt *runtimeImpl) onProgramAttached(
	programID ir.ProgramID,
	processID actuator.ProcessID,
	runtimeID procRuntimeID,
	program *ir.Program,
) {
	rt.store.link(programID, processID)
	rt.procRuntimeIDbyProgramID.Store(programID, runtimeID)
	for _, probe := range program.Probes {
		rt.diagnostics.reportInstalled(runtimeID, probe.ProbeDefinition)
	}
}

func (rt *runtimeImpl) onProgramDetached(programID ir.ProgramID) {
	rt.store.unlink(programID)
	rt.procRuntimeIDbyProgramID.Delete(programID)
}

func (rt *runtimeImpl) reportAttachError(
	programID ir.ProgramID, runtimeID procRuntimeID, program *ir.Program, err error,
) {
	log.Errorf("attaching program %v to process %v failed: %v", programID, runtimeID.ProcessID, err)
	for _, probe := range program.Probes {
		rt.diagnostics.reportError(runtimeID, probe.ProbeDefinition, err, "AttachmentFailed")
	}
}

func (rt *runtimeImpl) setProbeMaybeEmitting(programID ir.ProgramID, probe ir.ProbeDefinition) {
	if procRuntimeIDi, ok := rt.procRuntimeIDbyProgramID.Load(programID); ok {
		runtimeID := procRuntimeIDi.(procRuntimeID)
		rt.diagnostics.reportEmitting(runtimeID, probe)
	}
}

func (rt *runtimeImpl) reportProbeError(
	programID ir.ProgramID, probe ir.ProbeDefinition, err error, errType string,
) (reported bool) {
	procRuntimeIDi, ok := rt.procRuntimeIDbyProgramID.Load(programID)
	if !ok {
		return false
	}
	runtimeID := procRuntimeIDi.(procRuntimeID)
	return rt.diagnostics.reportError(runtimeID, probe, err, errType)
}

type defaultAttacher struct{}

func (defaultAttacher) Attach(
	program *loader.Program,
	executable actuator.Executable,
	processID actuator.ProcessID,
) (actuator.AttachedProgram, error) {
	return uprobe.Attach(program, executable, processID)
}

var _ actuator.Runtime = (*runtimeImpl)(nil)
var _ actuator.LoadedProgram = (*loadedProgramImpl)(nil)
var _ actuator.AttachedProgram = (*attachedProgramImpl)(nil)
