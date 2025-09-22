// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package procmon

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStateMachine(t *testing.T) {
	type step struct {
		ev       event
		analyze  []uint32         // nil if no build expected after this step
		update   *ProcessesUpdate // nil if no update expected after this step
		expAlive *[]uint32
	}

	type opt func(*step)

	upd := func(procPids ...uint32) opt {
		return func(s *step) {
			if s.update == nil {
				s.update = &ProcessesUpdate{}
			}
			for _, pid := range procPids {
				s.update.Processes = append(s.update.Processes, ProcessUpdate{
					ProcessID: ProcessID{PID: int32(pid)},
				})
			}
		}
	}

	rem := func(procPids ...uint32) opt {
		return func(s *step) {
			if s.update == nil {
				s.update = &ProcessesUpdate{}
			}
			for _, pid := range procPids {
				s.update.Removals = append(s.update.Removals, ProcessID{PID: int32(pid)})
			}
		}
	}

	analyze := func(procPids ...uint32) opt {
		return func(s *step) {
			s.analyze = append(s.analyze, procPids...)
		}
	}

	alive := func(procPids ...uint32) opt {
		return func(s *step) {
			s.expAlive = &procPids
		}
	}

	exec := func(pid uint32) event {
		return &processEvent{kind: processEventKindExec, pid: pid}
	}
	exit := func(pid uint32) event {
		return &processEvent{kind: processEventKindExit, pid: pid}
	}
	res := func(pid uint32, interesting bool, err error) event {
		return &analysisResult{
			pid:             pid,
			err:             err,
			processAnalysis: processAnalysis{interesting: interesting},
		}
	}
	interesting := func(pid uint32) event { return res(pid, true, nil) }
	uninteresting := func(pid uint32) event { return res(pid, false, nil) }
	failed := func(pid uint32, err error) event { return res(pid, false, err) }

	s := func(ev event, opts ...opt) step {
		s := step{ev: ev}
		for _, opt := range opts {
			opt(&s)
		}
		return s
	}

	tests := []struct {
		name  string
		steps []step
	}{
		{
			name: "simple exec interested",
			steps: []step{
				s(exec(1), analyze(1), alive(1)),
				s(res(1, true, nil), upd(1), alive(1)),
			},
		},
		{
			name: "exec then exit before build done",
			steps: []step{
				s(exec(2), analyze(2), alive(2)),
				s(exit(2), alive()),
				s(interesting(2), alive()),
			},
		},
		{
			name: "exec not interesting",
			steps: []step{
				s(exec(3), analyze(3), alive(3)),
				s(uninteresting(3), alive()),
			},
		},
		{
			name: "reported then removed",
			steps: []step{
				s(exec(4), analyze(4), alive(4)),
				s(interesting(4), upd(4), alive(4)),
				s(exit(4), rem(4), alive()),
			},
		},
		{
			name: "queueing delays reporting",
			steps: []step{
				s(exec(5), analyze(5), alive(5)),
				s(exec(6), alive(5, 6)),
				s(exec(7), alive(5, 6, 7)),
				s(exec(8), alive(5, 6, 7, 8)),
				s(interesting(5), analyze(6), alive(5, 6, 7, 8)),
				s(uninteresting(6), analyze(7), alive(5, 7, 8)),
				s(interesting(7), analyze(8)),
				s(failed(8, fmt.Errorf("test error")), upd(5, 7), alive(5, 7)),
				s(exit(6)),
				s(exit(7), rem(7), alive(5)),
				s(exit(8), alive(5)),
			},
		},
		{
			name: "reanalysis",
			steps: []step{
				s(exec(9), analyze(9), alive(9)),
				s(exec(10), alive(9, 10)),
				s(exec(9), alive(9, 10)), // marks for reanalysis
				s(uninteresting(9), analyze(10), alive(9, 10)),
				s(uninteresting(10), analyze(9), alive(9)),
				s(interesting(9), upd(9), alive(9)),
			},
		},
		{
			name: "readded while queued",
			steps: []step{
				s(exec(11), analyze(11), alive(11)),
				s(exec(12), alive(11, 12)),
				s(exec(12), alive(11, 12)),
				s(uninteresting(11), analyze(12), alive(12)),
				s(uninteresting(12), alive()),
			},
		},
		{
			name: "reanalyze then exit",
			steps: []step{
				s(exec(13), analyze(13), alive(13)),
				s(exec(14), alive(13, 14)),
				s(exec(13), alive(13, 14)), // marks for reanalysis
				s(exit(13), alive(14)),     // removes from alive
				s(interesting(13), analyze(14), alive(14)),
				s(uninteresting(14), alive()),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := makeState()
			for i, s := range tc.steps {
				if !t.Run(fmt.Sprint(i), func(t *testing.T) {
					mock := &mockEffects{}
					st.handle(s.ev, mock)
					if s.update != nil {
						require.Equal(
							t,
							[]ProcessesUpdate{*s.update},
							mock.updates,
						)
					} else {
						require.Empty(t, mock.updates)
					}
					require.Equal(t, s.analyze, mock.builds)
					if s.expAlive != nil {
						require.ElementsMatch(
							t,
							*s.expAlive,
							slices.Collect(maps.Keys(st.alive)),
						)
					}
					// Ensure that pending is always a subset of alive.
					require.Subset(t, st.alive, st.pending)
				}) {
					break
				}
			}
		})
	}
}

func (s *state) handle(ev event, eff effects) {
	switch e := ev.(type) {
	case *processEvent:
		s.handleProcessEvent(*e)
	case *analysisResult:
		s.handleAnalysisResult(*e)
	}
	s.analyzeOrReport(eff)
}

type event interface{ event() }

func (e *processEvent) event()   {}
func (r *analysisResult) event() {}

// It can synchronously feed processResult events back into the state machine
// and records every ProcessesUpdate sent to the actuator.
type mockEffects struct {
	updates []ProcessesUpdate
	builds  []uint32
}

func (m *mockEffects) analyzeProcess(pid uint32) {
	m.builds = append(m.builds, pid)
}

func (m *mockEffects) reportProcessesUpdate(u ProcessesUpdate) {
	m.updates = append(m.updates, u)
}
