// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package servicediscovery

import (
	"cmp"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	gocmp "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/prometheus/procfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
)

type testProc struct {
	pid     int
	cmdline []string
	env     []string
	cwd     string
	stat    procfs.ProcStat
}

var (
	bootTimeSeconds = uint64(time.Date(2000, 01, 01, 0, 0, 0, 0, time.UTC).Unix())
	// procLaunched is number of clicks (100 per second) since bootTime when the process started
	// assume it's 12 hours later
	procLaunchedClicks = uint64((12 * time.Hour).Seconds()) * 100
)

var (
	procSSHD = testProc{
		pid:     6,
		cmdline: []string{"sshd"},
		env:     []string{},
		cwd:     "",
		stat: procfs.ProcStat{
			Starttime: procLaunchedClicks,
		},
	}
	procTestService1 = testProc{
		pid:     99,
		cmdline: []string{"test-service-1"},
		env:     []string{},
		cwd:     "",
		stat: procfs.ProcStat{
			Starttime: procLaunchedClicks,
		},
	}
	procTestService1Empty = testProc{
		pid:     99,
		cmdline: []string{},
		env:     []string{},
		cwd:     "",
		stat: procfs.ProcStat{
			Starttime: procLaunchedClicks,
		},
	}
	procIgnoreService1 = testProc{
		pid:     100,
		cmdline: []string{"ignore-1"},
		env:     []string{},
		cwd:     "",
		stat: procfs.ProcStat{
			Starttime: procLaunchedClicks,
		},
	}
	procTestService1Repeat = testProc{
		pid:     101,
		cmdline: []string{"test-service-1"}, // same name as procTestService1
		env:     []string{},
		cwd:     "",
		stat: procfs.ProcStat{
			Starttime: procLaunchedClicks,
		},
	}
	procTestService1DifferentPID = testProc{
		pid:     102,
		cmdline: []string{"test-service-1"},
		env:     []string{},
		cwd:     "",
		stat: procfs.ProcStat{
			Starttime: procLaunchedClicks,
		},
	}
	procErrorProcfsCmd = testProc{
		pid:     103,
		cmdline: []string{"error-procfs-cmd"},
		env:     []string{},
		cwd:     "",
		stat: procfs.ProcStat{
			Starttime: procLaunchedClicks,
		},
	}
	procErrorProcfsStat = testProc{
		pid:     104,
		cmdline: []string{"error-procfs-stat"},
		env:     []string{},
		cwd:     "",
		stat: procfs.ProcStat{
			Starttime: procLaunchedClicks,
		},
	}
	procErrorSysProbe = testProc{
		pid:     105,
		cmdline: []string{"error-sysprobe"},
		env:     []string{},
		cwd:     "",
		stat: procfs.ProcStat{
			Starttime: procLaunchedClicks,
		},
	}
)

var (
	portTCP22 = &model.Port{
		Proto:       "tcp",
		Port:        22,
		ProcessName: "sshd",
		PID:         procSSHD.pid,
	}
	portTCP8080 = &model.Port{
		Proto:       "tcp",
		Port:        8080,
		ProcessName: "test-service-1",
		PID:         procTestService1.pid,
	}
	portTCP8080DifferentPID = &model.Port{
		Proto:       "tcp",
		Port:        8080,
		ProcessName: "test-service-1",
		PID:         procTestService1DifferentPID.pid,
	}
	portTCP8081 = &model.Port{
		Proto:       "tcp",
		Port:        8081,
		ProcessName: "ignore-1",
		PID:         procIgnoreService1.pid,
	}
	portTCP5432 = &model.Port{
		Proto:       "tcp",
		Port:        5432,
		ProcessName: "test-service-1",
		PID:         procTestService1Repeat.pid,
	}
	portTCP8081NoPID = &model.Port{
		Proto:       "tcp",
		Port:        8081,
		ProcessName: "",
		PID:         0,
	}
	portTCP8081Error = &model.Port{
		Proto:       "tcp",
		Port:        8081,
		ProcessName: "err-1",
		PID:         procErrorProcfsCmd.pid,
	}
	portTCP8082Error = &model.Port{
		Proto:       "tcp",
		Port:        8082,
		ProcessName: "err-2",
		PID:         procErrorProcfsStat.pid,
	}
	portTCP8083Error = &model.Port{
		Proto:       "tcp",
		Port:        8083,
		ProcessName: "err-3",
		PID:         procErrorSysProbe.pid,
	}
)

func mockProc(
	ctrl *gomock.Controller,
	p testProc,
) *Mockproc {
	m := NewMockproc(ctrl)
	m.EXPECT().PID().Return(p.pid).AnyTimes()

	if p.pid == procErrorProcfsCmd.pid {
		m.EXPECT().CmdLine().
			Return(nil, errors.New("procfs cmdline error")).
			AnyTimes()
	} else {
		m.EXPECT().CmdLine().Return(p.cmdline, nil).AnyTimes()
	}

	if p.pid == procErrorProcfsStat.pid {
		m.EXPECT().Stat().
			Return(procfs.ProcStat{}, errors.New("procfs stat error")).
			AnyTimes()
	} else {
		m.EXPECT().Stat().Return(p.stat, nil).AnyTimes()
	}

	return m
}

func calcTime(additionalTime time.Duration) time.Time {
	unix := time.Unix(int64(bootTimeSeconds+(procLaunchedClicks/100)), 0)
	return unix.Add(additionalTime)
}

// cmpEvents is used to sort event slices in tests.
// It returns true if a is smaller than b, false otherwise.
func cmpEvents(a, b *event) bool {
	if a == nil || a.Payload == nil {
		return true
	}
	if b == nil || b.Payload == nil {
		return false
	}
	ap := a.Payload
	bp := b.Payload

	vals := []any{
		cmp.Compare(ap.LastSeen, bp.LastSeen),
		cmp.Compare(ap.ServiceName, bp.ServiceName),
		cmp.Compare(ap.ServiceType, bp.ServiceType),
		cmp.Compare(ap.ServiceLanguage, bp.ServiceLanguage),
	}
	for _, val := range vals {
		if val != 0 {
			return val == -1
		}
	}
	return false
}

func Test_linuxImpl(t *testing.T) {
	type checkRun struct {
		aliveProcs    []testProc
		openPortsResp *model.OpenPortsResponse
		time          time.Time
	}

	tests := []struct {
		name       string
		checkRun   []*checkRun
		wantEvents []*event
	}{
		{
			name: "basic",
			checkRun: []*checkRun{
				{
					aliveProcs: []testProc{
						procSSHD,
						procIgnoreService1,
						procTestService1,
					},
					openPortsResp: &model.OpenPortsResponse{Ports: []*model.Port{
						portTCP22,
						portTCP8080,
						portTCP8081,
					}},
					time: calcTime(0),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procIgnoreService1,
						procTestService1,
					},
					openPortsResp: &model.OpenPortsResponse{Ports: []*model.Port{
						portTCP22,
						portTCP8080,
						portTCP8081,
					}},
					time: calcTime(1 * time.Minute),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procIgnoreService1,
						procTestService1,
					},
					openPortsResp: &model.OpenPortsResponse{Ports: []*model.Port{
						portTCP22,
						portTCP8080,
						portTCP8081,
					}},
					time: calcTime(20 * time.Minute),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
					},
					openPortsResp: &model.OpenPortsResponse{Ports: []*model.Port{
						portTCP22,
					}},
					time: calcTime(21 * time.Minute),
				},
			},
			wantEvents: []*event{
				{
					RequestType: "start-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						HostName:            ddHost,
						Env:                 ddEnv,
						ServiceLanguage:     "UNKNOWN",
						ServiceType:         "web_service",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(1 * time.Minute).Unix(),
						APMInstrumentation:  "none",
						ServiceNameSource:   "generated",
					},
				},
				{
					RequestType: "heartbeat-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						HostName:            ddHost,
						Env:                 ddEnv,
						ServiceLanguage:     "UNKNOWN",
						ServiceType:         "web_service",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(20 * time.Minute).Unix(),
						APMInstrumentation:  "none",
						ServiceNameSource:   "generated",
					},
				},
				{
					RequestType: "end-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						HostName:            ddHost,
						Env:                 ddEnv,
						ServiceLanguage:     "UNKNOWN",
						ServiceType:         "web_service",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(20 * time.Minute).Unix(),
						APMInstrumentation:  "none",
						ServiceNameSource:   "generated",
					},
				},
			},
		},
		{
			name: "repeated_service_name",
			checkRun: []*checkRun{
				{
					aliveProcs: []testProc{
						procSSHD,
						procIgnoreService1,
						procTestService1,
						procTestService1Repeat,
					},
					openPortsResp: &model.OpenPortsResponse{Ports: []*model.Port{
						portTCP22,
						portTCP8080,
						portTCP8081,
						portTCP5432,
					}},
					time: calcTime(0),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procIgnoreService1,
						procTestService1,
						procTestService1Repeat,
					},
					openPortsResp: &model.OpenPortsResponse{Ports: []*model.Port{
						portTCP22,
						portTCP8080,
						portTCP8081,
						portTCP5432,
					}},
					time: calcTime(1 * time.Minute),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procIgnoreService1,
						procTestService1,
						procTestService1Repeat,
					},
					openPortsResp: &model.OpenPortsResponse{Ports: []*model.Port{
						portTCP22,
						portTCP8080,
						portTCP8081,
						portTCP5432,
					}},
					time: calcTime(20 * time.Minute),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procTestService1,
					},
					openPortsResp: &model.OpenPortsResponse{Ports: []*model.Port{
						portTCP22,
						portTCP8080,
					}},
					time: calcTime(21 * time.Minute),
				},
			},
			wantEvents: []*event{
				{
					RequestType: "start-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						HostName:            ddHost,
						Env:                 ddEnv,
						ServiceLanguage:     "UNKNOWN",
						ServiceType:         "web_service",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(1 * time.Minute).Unix(),
						APMInstrumentation:  "none",
						ServiceNameSource:   "generated",
					},
				},
				{
					RequestType: "start-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						HostName:            ddHost,
						Env:                 ddEnv,
						ServiceLanguage:     "UNKNOWN",
						ServiceType:         "db",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(1 * time.Minute).Unix(),
						APMInstrumentation:  "none",
						ServiceNameSource:   "generated",
					},
				},
				{
					RequestType: "heartbeat-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						HostName:            ddHost,
						Env:                 ddEnv,
						ServiceLanguage:     "UNKNOWN",
						ServiceType:         "web_service",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(20 * time.Minute).Unix(),
						APMInstrumentation:  "none",
						ServiceNameSource:   "generated",
					},
				},
				{
					RequestType: "heartbeat-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						HostName:            ddHost,
						Env:                 ddEnv,
						ServiceLanguage:     "UNKNOWN",
						ServiceType:         "db",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(20 * time.Minute).Unix(),
						APMInstrumentation:  "none",
						ServiceNameSource:   "generated",
					},
				},
				{
					RequestType: "end-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						HostName:            ddHost,
						Env:                 ddEnv,
						ServiceLanguage:     "UNKNOWN",
						ServiceType:         "db",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(20 * time.Minute).Unix(),
						APMInstrumentation:  "none",
						ServiceNameSource:   "generated",
					},
				},
			},
		},
		{
			// in case we detect a service is restarted, we skip the stop event and send
			// another start event instead.
			name: "restart_service",
			checkRun: []*checkRun{
				{
					aliveProcs: []testProc{
						procSSHD,
						procIgnoreService1,
						procTestService1,
					},
					openPortsResp: &model.OpenPortsResponse{Ports: []*model.Port{
						portTCP22,
						portTCP8080,
						portTCP8081,
					}},
					time: calcTime(0),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procIgnoreService1,
						procTestService1,
					},
					openPortsResp: &model.OpenPortsResponse{Ports: []*model.Port{
						portTCP22,
						portTCP8080,
						portTCP8081,
					}},
					time: calcTime(1 * time.Minute),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procTestService1DifferentPID,
					},
					openPortsResp: &model.OpenPortsResponse{Ports: []*model.Port{
						portTCP22,
						portTCP8080DifferentPID,
					}},
					time: calcTime(21 * time.Minute),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procTestService1DifferentPID,
					},
					openPortsResp: &model.OpenPortsResponse{Ports: []*model.Port{
						portTCP22,
						portTCP8080DifferentPID,
					}},
					time: calcTime(22 * time.Minute),
				},
			},
			wantEvents: []*event{
				{
					RequestType: "start-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						HostName:            ddHost,
						Env:                 ddEnv,
						ServiceLanguage:     "UNKNOWN",
						ServiceType:         "web_service",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(1 * time.Minute).Unix(),
						APMInstrumentation:  "none",
						ServiceNameSource:   "generated",
					},
				},
				{
					RequestType: "start-service",
					APIVersion:  "v2",
					Payload: &eventPayload{
						NamingSchemaVersion: "1",
						ServiceName:         "test-service-1",
						HostName:            ddHost,
						Env:                 ddEnv,
						ServiceLanguage:     "UNKNOWN",
						ServiceType:         "web_service",
						StartTime:           calcTime(0).Unix(),
						LastSeen:            calcTime(22 * time.Minute).Unix(),
						APMInstrumentation:  "none",
						ServiceNameSource:   "generated",
					},
				},
			},
		},
		{
			name: "ports_without_pid_are_skipped",
			checkRun: []*checkRun{
				{
					aliveProcs: []testProc{
						procSSHD,
						procTestService1,
					},
					openPortsResp: &model.OpenPortsResponse{Ports: []*model.Port{
						portTCP22,
						portTCP8081NoPID,
					}},
					time: calcTime(0),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procTestService1,
					},
					openPortsResp: &model.OpenPortsResponse{Ports: []*model.Port{
						portTCP22,
						portTCP8081NoPID,
					}},
					time: calcTime(1),
				},
			},
			wantEvents: nil,
		},
		{
			name: "procs_generating_empty_service_name_are_skipped",
			checkRun: []*checkRun{
				{
					aliveProcs: []testProc{
						procSSHD,
						procTestService1Empty,
					},
					openPortsResp: &model.OpenPortsResponse{Ports: []*model.Port{
						portTCP22,
						portTCP8081,
					}},
					time: calcTime(0),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procTestService1Empty,
					},
					openPortsResp: &model.OpenPortsResponse{Ports: []*model.Port{
						portTCP22,
						portTCP8081,
					}},
					time: calcTime(1 * time.Minute),
				},
			},
			wantEvents: nil,
		},
		{
			name: "procs_returning_err_are_skipped_and_ignored",
			checkRun: []*checkRun{
				{
					aliveProcs: []testProc{
						procSSHD,
						procErrorProcfsCmd,
						procErrorProcfsStat,
						procErrorSysProbe,
					},
					openPortsResp: &model.OpenPortsResponse{Ports: []*model.Port{
						portTCP22,
						portTCP8081Error,
						portTCP8082Error,
						portTCP8083Error,
					}},
					time: calcTime(0),
				},
				{
					aliveProcs: []testProc{
						procSSHD,
						procErrorProcfsCmd,
						procErrorProcfsStat,
						procErrorSysProbe,
					},
					openPortsResp: &model.OpenPortsResponse{Ports: []*model.Port{
						portTCP22,
						portTCP8081Error,
						portTCP8082Error,
						portTCP8083Error,
					}},
					time: calcTime(1 * time.Minute),
				},
			},
			wantEvents: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DD_ENV", ddEnv)
			t.Setenv("DD_HOSTNAME", ddHost)
			ddconfig.SystemProbe.SetDefault("system_probe_config.enabled", true)
			ddconfig.SystemProbe.SetDefault("service_discovery.enabled", true)

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// check and mocks setup
			check := newCheck().(*Check)

			mSender := mocksender.NewMockSender(check.ID())
			mSender.SetupAcceptAll()

			err := check.Configure(
				mSender.GetSenderManager(),
				integration.FakeConfigHash,
				integration.Data(checkConfigStr),
				nil,
				"test",
			)
			require.NoError(t, err)
			require.NotNil(t, check.os)

			for _, cr := range tc.checkRun {
				mSysProbe := NewMocksystemProbeClient(ctrl)
				mSysProbe.EXPECT().GetServiceDiscoveryOpenPorts(gomock.Any()).
					Return(cr.openPortsResp, nil).
					Times(1)

				var procs []proc
				for _, p := range cr.aliveProcs {
					mProc := mockProc(ctrl, p)

					if p.pid == procErrorSysProbe.pid {
						mSysProbe.EXPECT().GetServiceDiscoveryProc(gomock.Any(), gomock.Eq(p.pid)).
							Return(nil, errors.New("sysprobe get proc error")).
							AnyTimes()
					} else {
						mSysProbe.EXPECT().GetServiceDiscoveryProc(gomock.Any(), gomock.Eq(p.pid)).
							Return(
								&model.GetProcResponse{Proc: &model.Proc{
									PID:     p.pid,
									Environ: p.env,
									CWD:     p.cwd,
								}},
								nil,
							).
							AnyTimes()
					}

					procs = append(procs, mProc)
				}

				mProcFS := NewMockprocFS(ctrl)
				mProcFS.EXPECT().AllProcs().Return(procs, nil).Times(1)

				mTimer := NewMocktimer(ctrl)
				mTimer.EXPECT().Now().Return(cr.time).AnyTimes()

				// set mocks
				check.os.(*linuxImpl).procfs = mProcFS
				check.os.(*linuxImpl).getSysProbeClient = func() (systemProbeClient, error) {
					return mSysProbe, nil
				}
				check.os.(*linuxImpl).time = mTimer
				check.os.(*linuxImpl).bootTime = bootTimeSeconds

				err = check.Run()
				require.NoError(t, err)
			}

			mSender.AssertNumberOfCalls(t, "EventPlatformEvent", len(tc.wantEvents))
			gotEvents := mockSenderEvents(t, mSender)

			diff := gocmp.Diff(tc.wantEvents, gotEvents, cmpopts.SortSlices(cmpEvents))

			if diff != "" {
				t.Errorf("event platform events mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func Test_linuxImpl_errors(t *testing.T) {
	ignoreCfg := map[string]bool{
		"ignore-1": true,
		"ignore-2": true,
	}

	tests := []struct {
		name        string
		procfs      procFS
		setMocks    func(li *linuxImpl, ctrl *gomock.Controller)
		wantErrCode errCode
	}{
		{
			name: "system_probe_conn",
			setMocks: func(li *linuxImpl, ctrl *gomock.Controller) {
				li.getSysProbeClient = func() (systemProbeClient, error) {
					return nil, errors.New("system probe conn error")
				}
			},
			wantErrCode: errorCodeSystemProbeConn,
		},
		{
			name: "procfs_allprocs_err",
			setMocks: func(li *linuxImpl, ctrl *gomock.Controller) {
				mErrProcfs := NewMockprocFS(ctrl)
				mErrProcfs.EXPECT().AllProcs().
					Return(nil, errors.New("procFS failure")).
					AnyTimes()

				li.procfs = mErrProcfs
			},
			wantErrCode: errorCodeProcfs,
		},
		{
			name: "system_probe_open_ports",
			setMocks: func(li *linuxImpl, ctrl *gomock.Controller) {
				mErrSystemProbe := NewMocksystemProbeClient(ctrl)
				mErrSystemProbe.EXPECT().GetServiceDiscoveryOpenPorts(gomock.Any()).
					Return(nil, errors.New("system-probe open ports failure")).
					AnyTimes()

				mProcfs := NewMockprocFS(ctrl)
				mProcfs.EXPECT().AllProcs().Return([]proc{}, nil).AnyTimes()

				li.procfs = mProcfs
				li.getSysProbeClient = func() (systemProbeClient, error) {
					return mErrSystemProbe, nil
				}
			},
			wantErrCode: errorCodeSystemProbeOpenPorts,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			impl, err := newLinuxImpl(ignoreCfg)
			require.NoError(t, err)
			li := impl.(*linuxImpl)

			if tc.setMocks != nil {
				tc.setMocks(li, ctrl)
			}

			ds, err := li.DiscoverServices()
			assert.Nil(t, ds, "expected nil discovered services")
			require.Error(t, err, "expected error from DiscoverServices")

			if tc.wantErrCode == "" {
				return
			}
			errc := &errWithCode{}
			require.ErrorAs(t, err, errc, "expected error to be errWithCode")
			assert.Equal(t, tc.wantErrCode, errc.Code())
		})
	}
}
