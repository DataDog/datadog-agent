// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
)

// TestStateMachineProperties tests the state machine properties using
// randomized events. This is a form of property-based testing.
//
// By default, runs multiple subtests with different seeds for better coverage.
// If DYNINST_SEED is provided, runs only once with that specific seed.
// Each seed produces deterministic results.
func TestStateMachineProperties(t *testing.T) {
	const seedEnvVar = "DYNINST_SEED"
	const defaultRuns = 5

	// Run once with the specified seed. This is useful for debugging.
	if seedStr := os.Getenv(seedEnvVar); seedStr != "" {
		// Run once with the specified seed
		seed, err := strconv.ParseInt(seedStr, 10, 64)
		require.NoError(t, err, "invalid %s: %v", seedEnvVar, err)
		t.Logf("using seed: %d", seed)
		runStateMachinePropertyTest(t, seed)
		return
	}

	// Run multiple times with different seeds.
	numRuns := defaultRuns
	if numRunsStr := os.Getenv("DYNINST_NUM_RUNS"); numRunsStr != "" {
		var err error
		numRuns, err = strconv.Atoi(numRunsStr)
		require.NoError(t, err, "invalid %s: %v", "DYNINST_NUM_RUNS", err)
	}
	baseSeed := time.Now().UnixNano()

	for i := 0; i < numRuns; i++ {
		seed := baseSeed + int64(i)
		if !t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			t.Logf("using seed: %d", seed)
			// Run the test twice to ensure the output is deterministic.
			firstRun := runStateMachinePropertyTest(t, seed)
			secondRun := runStateMachinePropertyTest(t, seed)
			// Compare the bytes before converting to string to avoid the
			// extra allocations. Use require because it prints nice diffs
			// when the test fails.
			if !bytes.Equal(firstRun, secondRun) {
				require.Equal(
					t, string(firstRun), string(secondRun),
					"property test output should be deterministic",
				)
			}
		}) {
			t.FailNow()
		}
	}
}

// runStateMachinePropertyTest runs a single property test with the given seed
// and returns the accumulated output buffer.
func runStateMachinePropertyTest(t *testing.T, seed int64) []byte {
	const showOutputEnvVar = "DYNINST_SHOW_OUTPUT"
	showOutput, _ := strconv.ParseBool(os.Getenv(showOutputEnvVar))

	var outputBuf bytes.Buffer
	defer func() {
		if t.Failed() || showOutput {
			t.Logf("state snapshots:\n%s", outputBuf.String())
		}
	}()

	rng := rand.New(rand.NewSource(seed))

	pts := &propertyTestState{
		sm:               newState(),
		processIDCounter: 1000,
		rng:              rng,
	}

	effects := &effectRecorder{}

	const maxEvents = 1000
	eventCount := 0
	totalEffectsProduced := 0
	for ; eventCount < maxEvents; eventCount++ {
		// Generate random event.
		ev, ok := pts.generateRandomEvent()
		if !ok {
			if !pts.sm.isShutdown() {
				t.Fatalf(
					"Event %d: no event generated, but state is not shutdown",
					eventCount,
				)
			}
			break
		}
		if ev == nil {
			eventCount--
			continue
		}

		// Create snapshot before handling event.
		before := deepCopyState(pts.sm)

		// Apply event to state machine.
		err := handleEvent(pts.sm, effects, ev)
		if err != nil {
			t.Fatalf(
				"Event %d (%T): handleEvent returned error: %v",
				eventCount, ev, err,
			)
		}

		// Add new effects to unresolved effects.
		totalEffectsProduced += len(effects.effects)
		pts.unresolvedEffects = append(
			pts.unresolvedEffects, effects.effects...,
		)

		// Generate and log event output using shared infrastructure.
		yamlEv := wrapEventForYAML(ev)
		eventNode := &yaml.Node{}
		require.NoError(t, eventNode.Encode(yamlEv))
		output := generateEventOutput(t, eventNode, *effects, before, pts.sm)
		if eventCount > 0 {
			outputBuf.WriteString("---\n")
		}
		outputBuf.Write(output)

		effects.effects = nil

		// Validate state consistency.
		var validationErrors []error
		validateState(pts.sm, func(err error) {
			validationErrors = append(validationErrors, err)
		})

		if len(validationErrors) > 0 {
			t.Fatalf(
				"Event %d (%T): state validation failed:\n%v",
				eventCount, ev, validationErrors,
			)
		}
	}

	t.Logf("Total events: %d", eventCount)
	t.Logf("Total effects produced: %d", totalEffectsProduced)
	t.Logf("Final state: %d processes, %d programs, %d unresolved effects",
		len(pts.sm.processes), len(pts.sm.programs), len(pts.unresolvedEffects),
	)

	return outputBuf.Bytes()
}

type propertyTestState struct {
	sm                *state
	unresolvedEffects []effect
	processIDCounter  int
	rng               *rand.Rand
	shuttingDown      bool
}

// generateRandomEvent creates a random event based on current unresolved
// effects or process updates.
func (pts *propertyTestState) generateRandomEvent() (event, bool) {
	if pts.shuttingDown {
		if len(pts.unresolvedEffects) == 0 {
			return nil, false
		}
		return pts.completeRandomEffect(), true
	}
	choice := pts.rng.Float64()
	switch {
	case choice < 0.01:
		pts.shuttingDown = true
		return eventShutdown{}, true
	case choice < 0.5 || len(pts.unresolvedEffects) == 0:
		return pts.generateProcessUpdate(), true
	default:
		return pts.completeRandomEffect(), true
	}
}

func (pts *propertyTestState) generateProcessUpdate() event {
	numUpdates := pts.rng.Intn(3) + 1 // 1-3 process updates
	var updates []ProcessUpdate
	var removals []ProcessID

	for i := 0; i < numUpdates; i++ {
		choice := pts.rng.Float64()
		switch {
		case choice < 0.5: // 50% chance of creating new process
			pts.processIDCounter++
			processID := ProcessID{PID: int32(pts.processIDCounter)}

			numProbes := pts.rng.Intn(3) + 1 // 1-3 probes
			var probes []ir.ProbeDefinition
			for j := 0; j < numProbes; j++ {
				probe := &rcjson.LogProbe{
					LogProbeCommon: rcjson.LogProbeCommon{
						ProbeCommon: rcjson.ProbeCommon{
							ID: fmt.Sprintf(
								"probe_%d_%d", pts.processIDCounter, j,
							),
							Version:  pts.rng.Intn(5) + 1,
							Where:    &rcjson.Where{MethodName: "main"},
							Tags:     []string{"test"},
							Language: "go",
						},
						Template: "test log message",
						Segments: []json.RawMessage{
							json.RawMessage(`"test log message"`),
						},
					},
				}
				probes = append(probes, probe)
			}

			updates = append(updates, ProcessUpdate{
				ProcessID: processID,
				Executable: Executable{
					Path: fmt.Sprintf("/usr/bin/app_%d", pts.processIDCounter),
				},
				Probes: probes,
			})

		case choice < 0.8 && len(pts.sm.processes) > 0: // 30% chance of updating existing process
			// Update a random existing process with different probes
			existingProcesses := pts.existingProcesses()
			if len(existingProcesses) > 0 {
				processID := existingProcesses[pts.rng.Intn(len(existingProcesses))]
				existingProcess := pts.sm.processes[processID]

				// Generate different probes (different count and/or different
				// versions/IDs).
				numProbes := pts.rng.Intn(4) // 0-3 probes, 0 means removal
				var probes []ir.ProbeDefinition
				for j := 0; j < numProbes; j++ {
					probe := &rcjson.SnapshotProbe{
						LogProbeCommon: rcjson.LogProbeCommon{
							ProbeCommon: rcjson.ProbeCommon{
								ID: fmt.Sprintf(
									"probe_%d_%d_updated", processID.PID, j,
								),
								Version: pts.rng.Intn(5) + 1,
								Where:   &rcjson.Where{MethodName: "main"},
								Tags:    []string{"test", "updated"},

								Language: "go",
							},
						},
					}
					probes = append(probes, probe)
				}

				updates = append(updates, ProcessUpdate{
					ProcessID:  processID.ProcessID,
					Executable: existingProcess.executable,
					Probes:     probes,
				})
			}

		case len(pts.sm.processes) > 0: // 20% chance of removing existing process
			// Remove a random existing process
			existingProcesses := pts.existingProcesses()
			if len(existingProcesses) > 0 {
				toRemove := existingProcesses[pts.rng.Intn(len(existingProcesses))]
				removals = append(removals, toRemove.ProcessID)
			}
		}
	}

	return eventProcessesUpdated{
		updated: updates,
		removed: removals,
	}
}

func (pts *propertyTestState) existingProcesses() []processKey {
	existingProcesses := make([]processKey, 0, len(pts.sm.processes))
	for key := range pts.sm.processes {
		existingProcesses = append(existingProcesses, key)
	}
	slices.SortFunc(existingProcesses, func(a, b processKey) int {
		return cmp.Or(
			cmp.Compare(a.tenantID, b.tenantID),
			cmp.Compare(a.PID, b.PID),
		)
	})
	return existingProcesses
}

func (pts *propertyTestState) completeRandomEffect() event {
	if len(pts.unresolvedEffects) == 0 {
		return pts.generateProcessUpdate()
	}

	// Choose random effect to complete.
	idx := pts.rng.Intn(len(pts.unresolvedEffects))
	effect := pts.unresolvedEffects[idx]

	// Remove from unresolved effects.
	pts.unresolvedEffects = slices.Delete(pts.unresolvedEffects, idx, idx+1)

	// 80% chance of success, 20% chance of failure.
	success := pts.rng.Float64() < 0.8

	switch eff := effect.(type) {

	case effectSpawnBpfLoading:
		if success {
			return eventProgramLoaded{
				programID: eff.programID,
				loaded: &loadedProgram{
					ir: &ir.Program{ID: eff.programID},
				},
			}
		} else {
			return eventProgramLoadingFailed{
				programID: eff.programID,
				err:       fmt.Errorf("mock loading failure"),
			}
		}

	case effectAttachToProcess:
		if success {
			return eventProgramAttached{
				program: &attachedProgram{
					ir:     &ir.Program{ID: eff.programID},
					procID: eff.processID,
				},
			}
		} else {
			return eventProgramAttachingFailed{
				programID: eff.programID,
				processID: eff.processID,
				err:       fmt.Errorf("mock attaching failure"),
			}
		}

	case effectDetachFromProcess:
		return eventProgramDetached{
			programID: eff.programID,
			processID: eff.processID,
		}
	case effectUnloadProgram:
		return eventProgramUnloaded{
			programID: eff.programID,
		}

	default:
		panic(fmt.Sprintf("unknown effect: %T", eff))
	}

}
