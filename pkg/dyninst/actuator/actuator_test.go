// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator_test

import (
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

func TestNoSuccessfulProbesError(t *testing.T) {
	dyninsttest.SkipIfKernelNotSupported(t)

	cfgs := testprogs.MustGetCommonConfigs(t)
	for _, cfg := range cfgs {
		t.Run(cfg.String(), func(t *testing.T) {
			if cfg.GOARCH != runtime.GOARCH {
				t.Skipf(
					"cross-execution is not supported, running on %s",
					runtime.GOARCH,
				)
			}
			testNoSuccessfulProbesError(t, cfg)
		})
	}
}

func testNoSuccessfulProbesError(t *testing.T, cfg testprogs.Config) {
	prog := testprogs.MustGetBinary(t, "simple", cfg)
	loader, err := loader.NewLoader()
	require.NoError(t, err)
	a := actuator.NewActuator(loader)

	cmd := exec.Command(prog)
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	stat, err := os.Stat(prog)
	require.NoError(t, err)
	statT := stat.Sys().(*syscall.Stat_t)
	executable := actuator.Executable{
		Path: prog,
		Key: procmon.FileKey{
			FileHandle: procmon.FileHandle{
				Dev: statT.Dev,
				Ino: statT.Ino,
			},
			LastModified: statT.Mtim,
		},
	}

	reporter := &irGenErrorReporter{
		t:  t,
		ch: make(chan irGenFailedMessage, 1),
	}
	at := a.NewTenant("test", reporter)

	pid := actuator.ProcessID{PID: int32(cmd.Process.Pid), Service: "test"}
	at.HandleUpdate(actuator.ProcessesUpdate{
		Processes: []actuator.ProcessUpdate{{
			ProcessID:  pid,
			Executable: executable,
			Probes: []ir.ProbeDefinition{
				&rcjson.SnapshotProbe{
					LogProbeCommon: rcjson.LogProbeCommon{
						ProbeCommon: rcjson.ProbeCommon{
							ID: "test",
							Where: &rcjson.Where{
								MethodName: "main.DoesNotExist",
							},
						},
					},
				},
			},
		}},
	})
	msg, ok := <-reporter.ch
	require.True(t, ok)
	require.Equal(t, pid, msg.processID)
	var noSuccessfulProbesError *actuator.NoSuccessfulProbesError
	require.ErrorAs(t, msg.err, &noSuccessfulProbesError)
	require.Len(t, noSuccessfulProbesError.Issues, 1)
	issue := noSuccessfulProbesError.Issues[0]
	require.Equal(t, "test", issue.ProbeDefinition.GetID())
	require.Equal(t, ir.IssueKindTargetNotFoundInBinary, issue.Kind)

	require.NoError(t, stdin.Close())
	require.NoError(t, cmd.Wait())
}

type irGenErrorReporter struct {
	t         *testing.T
	ch        chan irGenFailedMessage
	closeOnce sync.Once
}

type irGenFailedMessage struct {
	processID actuator.ProcessID
	err       error
	probes    []ir.ProbeDefinition
}

var _ actuator.Reporter = (*irGenErrorReporter)(nil)

func (r *irGenErrorReporter) errf(format string, args ...any) {
	r.t.Errorf(format, args...)
	r.closeOnce.Do(func() { close(r.ch) })
}

func (r *irGenErrorReporter) ReportAttached(actuator.ProcessID, *ir.Program) {
	r.errf("ReportAttached should not be called")
}
func (r *irGenErrorReporter) ReportAttachingFailed(actuator.ProcessID, *ir.Program, error) {
	r.errf("ReportAttachingFailed should not be called")
}
func (r *irGenErrorReporter) ReportDetached(actuator.ProcessID, *ir.Program) {
	r.errf("ReportDetached should not be called")
}
func (r *irGenErrorReporter) ReportIRGenFailed(processID actuator.ProcessID, err error, probes []ir.ProbeDefinition) {
	r.ch <- irGenFailedMessage{
		processID: processID,
		err:       err,
		probes:    probes,
	}
}
func (r *irGenErrorReporter) ReportLoaded(
	actuator.ProcessID,
	actuator.Executable,
	*ir.Program,
) (actuator.Sink, error) {
	r.errf("ReportLoaded should not be called")
	return nil, nil
}
func (r *irGenErrorReporter) ReportLoadingFailed(actuator.ProcessID, *ir.Program, error) {
	r.errf("ReportLoadingFailed should not be called")
}
