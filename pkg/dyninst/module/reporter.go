// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/decode"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type controllerReporter controller

// ReportAttached implements actuator.Reporter.
func (c *controllerReporter) ReportAttached(
	procID actuator.ProcessID, program *ir.Program,
) {
	ctrl := (*controller)(c)
	ctrl.store.link(program.ID, procID)

	runtimeID, ok := ctrl.store.getRuntimeID(procID)
	if !ok {
		return
	}
	ctrl.procRuntimeIDbyProgramID.Store(program.ID, runtimeID)
	for _, probe := range program.Probes {
		ctrl.diagnostics.reportInstalled(runtimeID, probe.ProbeDefinition)
	}
}

// ReportAttachingFailed implements actuator.Reporter.
func (c *controllerReporter) ReportAttachingFailed(
	procID actuator.ProcessID, program *ir.Program, err error,
) {
	log.Errorf("attaching program %v to process %v failed: %v", program.ID, procID, err)
	ctrl := (*controller)(c)
	runtimeID, ok := ctrl.store.getRuntimeID(procID)
	if !ok {
		return
	}
	for _, probe := range program.Probes {
		ctrl.diagnostics.reportError(
			runtimeID, probe.ProbeDefinition, err, "AttachmentFailed",
		)
	}
}

// ReportDetached implements actuator.Reporter.
func (c *controllerReporter) ReportDetached(
	_ actuator.ProcessID, program *ir.Program,
) {
	ctrl := (*controller)(c)
	ctrl.store.unlink(program.ID)
	ctrl.procRuntimeIDbyProgramID.Delete(program.ID)
}

// ReportIRGenFailed implements actuator.Reporter.
func (c *controllerReporter) ReportIRGenFailed(
	procID actuator.ProcessID,
	err error,
	probes []ir.ProbeDefinition,
) {
	log.Errorf("IR generation for process %v failed: %v", procID, err)
	ctrl := (*controller)(c)
	runtimeID, ok := ctrl.store.getRuntimeID(procID)
	if !ok {
		return
	}
	for _, probe := range probes {
		ctrl.diagnostics.reportError(runtimeID, probe, err, "IRGenFailed")
	}
}

// ReportLoaded implements actuator.Reporter.
func (c *controllerReporter) ReportLoaded(
	procID actuator.ProcessID,
	executable actuator.Executable,
	program *ir.Program,
) (actuator.Sink, error) {
	ctrl := (*controller)(c)
	// The process must have already exited.
	runtimeID, ok := ctrl.store.updateOnLoad(procID, executable, program.ID)
	if !ok {
		return noopSink{}, nil
	}
	ctrl.procRuntimeIDbyProgramID.Store(program.ID, runtimeID)

	decoder, err := decode.NewDecoder(program)
	if err != nil {
		return nil, fmt.Errorf("creating decoder: %w", err)
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

	s := &sink{
		controller:   ctrl,
		decoder:      decoder,
		symbolicator: ctrl.store.getSymbolicator(program.ID),
		programID:    program.ID,
		service:      runtimeID.service,
		logUploader: ctrl.logUploader.GetUploader(uploader.LogsUploaderMetadata{
			Tags:        tags,
			EntityID:    entityID,
			ContainerID: containerID,
		}),
	}
	return s, nil
}

// ReportLoadingFailed implements actuator.Reporter.
func (c *controllerReporter) ReportLoadingFailed(
	procID actuator.ProcessID, program *ir.Program, err error,
) {
	log.Errorf("loading program %v to process %v failed: %v", program.ID, procID, err)
	ctrl := (*controller)(c)
	ctrl.procRuntimeIDbyProgramID.Delete(program.ID)
	runtimeID, ok := ctrl.store.getRuntimeID(procID)
	if !ok {
		return
	}
	for _, probe := range program.Probes {
		ctrl.diagnostics.reportError(
			runtimeID, probe.ProbeDefinition, err, "LoadingFailed",
		)
	}
}
