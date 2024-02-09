// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package expvars

import (
	"expvar"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stub"
)

// Helper methods

func setUp() {
	Reset()
}

func getRunnerExpvarMap(t require.TestingT) *expvar.Map {
	runnerMapExpvar := expvar.Get("runner")
	require.NotNil(t, runnerMapExpvar)

	return runnerMapExpvar.(*expvar.Map)
}

func getRunningChecksExpvarMap(t require.TestingT) *expvar.Map {
	runnerMap := getRunnerExpvarMap(t)

	runningChecksExpvar := runnerMap.Get("Running")
	require.NotNil(t, runningChecksExpvar)

	return runningChecksExpvar.(*expvar.Map)
}

func getCheckStatsExpvarMap(t require.TestingT) map[string]map[checkid.ID]*stats.Stats {
	runnerMap := getRunnerExpvarMap(t)

	checkStatsExpvar := runnerMap.Get("Checks")
	require.NotNil(t, checkStatsExpvar)

	// This is messy but there's no easy way to get the value of a function
	// other than chain conversion unless we wanted to do a plain string
	// comparisons. We do `expvar.Var` -> `expvar.Func` -> `expvarRunnerCheckStats`
	// via `Func.Value()`.
	return checkStatsExpvar.(expvar.Func).Value().(map[string]map[checkid.ID]*stats.Stats)
}

func getExpvarMapKeys(m *expvar.Map) []string {
	keys := make([]string, 0)

	m.Do(func(kv expvar.KeyValue) {
		keys = append(keys, kv.Key)
	})

	sort.Strings(keys)

	return keys
}

func assertKeyNotSet(t require.TestingT, key string) {
	runnerMap := getRunnerExpvarMap(t)
	intExpvar := runnerMap.Get(key)
	require.Nil(t, intExpvar)
}

func getRunnerExpvarInt(t require.TestingT, key string) int {
	runnerMap := getRunnerExpvarMap(t)
	intExpvar := runnerMap.Get(key)
	if !assert.NotNil(t, intExpvar) {
		assert.FailNow(t, fmt.Sprintf("Variable '%s' not found in expvars!", key))
	}

	return int(intExpvar.(*expvar.Int).Value())
}

func changeAndAssertExpvarValue(
	t require.TestingT,
	key string,
	setter func(int),
	getter func() int64,
	amount int,
	expectedVal int) {

	setter(amount)

	// Actual expvar values
	actualExpvarValue := getRunnerExpvarInt(t, key)
	if !assert.Equal(t, expectedVal, actualExpvarValue) {
		assert.FailNow(
			t,
			fmt.Sprintf("Variable '%s' did not have the expected expvar value of '%d' (was: '%d')!",
				key,
				expectedVal,
				actualExpvarValue,
			))
	}

	// Internally-retrieved values
	actualInternalValue := int(getter())
	if !assert.Equal(t, expectedVal, actualInternalValue) {
		assert.FailNow(
			t,
			fmt.Sprintf("Variable '%s' did not have the expected internal value of '%d' (was: '%d')!",
				key,
				expectedVal,
				actualInternalValue,
			))
	}
}

type testCheck struct {
	stub.StubCheck
	id string
}

func (c *testCheck) ID() checkid.ID { return checkid.ID(c.id) }
func (c *testCheck) String() string { return checkid.IDToCheckName(c.ID()) }

func newTestCheck(id string) *testCheck {
	return &testCheck{id: id}
}

// Tests

func TestExpvarsInitialState(t *testing.T) {
	setUp()

	runnerMap := getRunnerExpvarMap(t)

	runningChecks := getRunningChecksExpvarMap(t)
	assert.Equal(t, 0, len(getExpvarMapKeys(runningChecks)))

	checks := runnerMap.Get("Checks")
	require.NotNil(t, checks)
	assert.Equal(t, "{}", checks.String())

	workers := runnerMap.Get("Workers")
	require.NotNil(t, workers)
}

func TestExpvarsInitialInternalState(t *testing.T) {
	setUp()
	assert.Equal(t, 0, len(GetCheckStats()))
}

func TestExpvarsReset(t *testing.T) {
	setUp()

	numCheckNames := 3
	numCheckInstances := 5
	numCheckRuns := 7

	// Add some data to the check stats
	for checkNameIdx := 0; checkNameIdx < numCheckNames; checkNameIdx++ {
		for checkIDIdx := 0; checkIDIdx < numCheckInstances; checkIDIdx++ {
			checkName := fmt.Sprintf("testcheck%d", checkNameIdx)
			checkID := fmt.Sprintf("%s:%d", checkName, checkIDIdx)
			testCheck := newTestCheck(checkID)

			for runIdx := 0; runIdx < numCheckRuns; runIdx++ {
				AddCheckStats(testCheck, 12345, nil, []error{}, stats.SenderStats{})
			}
		}
	}

	AddErrorsCount(1)
	AddRunsCount(2)
	AddRunningCheckCount(3)
	AddWarningsCount(4)

	assert.Equal(t, numCheckNames, len(GetCheckStats()))
	assert.Equal(t, numCheckNames, len(getCheckStatsExpvarMap(t)))
	assert.NotNil(t, getRunnerExpvarMap(t).Get(errorsExpvarKey))
	assert.NotNil(t, getRunnerExpvarMap(t).Get(runsExpvarKey))
	assert.NotNil(t, getRunnerExpvarMap(t).Get(runningChecksExpvarKey))
	assert.NotNil(t, getRunnerExpvarMap(t).Get(warningsExpvarKey))
	assert.NotNil(t, getRunnerExpvarMap(t).Get(workersExpvarKey))

	Reset()

	assert.Equal(t, 0, len(GetCheckStats()))
	assert.Equal(t, 0, len(getCheckStatsExpvarMap(t)))
	assert.Equal(t, 0, len(getExpvarMapKeys(getRunningChecksExpvarMap(t))))
	assert.Nil(t, getRunnerExpvarMap(t).Get(errorsExpvarKey))
	assert.Nil(t, getRunnerExpvarMap(t).Get(runsExpvarKey))
	assert.Nil(t, getRunnerExpvarMap(t).Get(runningChecksExpvarKey))
	assert.Nil(t, getRunnerExpvarMap(t).Get(warningsExpvarKey))
	assert.NotNil(t, getRunnerExpvarMap(t).Get(workersExpvarKey))
}

// TestExpvarsCheckStats includes tests of `AddCheckStats()`, `RemoveCheckStats()`, and
// `CheckStats()`
func TestExpvarsCheckStats(t *testing.T) {
	numCheckNames := 3
	numCheckInstances := 5
	numCheckRuns := 7

	var wg sync.WaitGroup
	start := make(chan struct{})

	// Each loop here is a unique check type/name
	for checkNameIdx := 0; checkNameIdx < numCheckNames; checkNameIdx++ {
		// Each loop here is a unique check instance
		for checkIDIdx := 0; checkIDIdx < numCheckInstances; checkIDIdx++ {
			checkName := fmt.Sprintf("testcheck%d", checkNameIdx)
			checkID := fmt.Sprintf("%s:%d", checkName, checkIDIdx)
			testCheck := newTestCheck(checkID)

			// TODO: Assers expvars too
			_, found := CheckStats(testCheck.ID())
			assert.False(t, found)

			// Each loop here is a single run of a check instance
			for rIdx := 0; rIdx < numCheckRuns; rIdx++ {
				// Copy the index value since loop reuses pointers
				runIdx := rIdx

				wg.Add(1)

				go func() {
					defer wg.Done()

					duration := time.Duration(runIdx+1) * time.Second
					err := fmt.Errorf("error %d", runIdx)
					warnings := []error{
						fmt.Errorf("warning %d", runIdx),
						fmt.Errorf("warning2 %d", runIdx),
					}
					expectedStats := stats.SenderStats{}

					<-start

					AddCheckStats(testCheck, duration, err, warnings, expectedStats)

					actualStats, found := CheckStats(testCheck.ID())
					require.True(t, found)

					assert.Equal(t, checkName, actualStats.CheckName)
					assert.Equal(t, checkID, string(actualStats.CheckID))
				}()
			}
		}
	}

	// Test addition of stats

	// Sanity check both internal variables and expvars
	assert.Equal(t, 0, len(GetCheckStats()))
	assert.Equal(t, 0, len(getCheckStatsExpvarMap(t)))

	// Start all goroutines and wait for them to finish
	close(start)
	wg.Wait()

	assert.Equal(t, numCheckNames, len(GetCheckStats()))
	assert.Equal(t, numCheckNames, len(getCheckStatsExpvarMap(t)))

	for checkNameIdx := 0; checkNameIdx < numCheckNames; checkNameIdx++ {
		checkName := fmt.Sprintf("testcheck%d", checkNameIdx)

		assert.Equal(t, numCheckInstances, len(GetCheckStats()[checkName]))
		assert.Equal(t, numCheckInstances, len(getCheckStatsExpvarMap(t)[checkName]))

		for checkIDIdx := 0; checkIDIdx < numCheckInstances; checkIDIdx++ {
			checkID := checkid.ID(fmt.Sprintf("%s:%d", checkName, checkIDIdx))
			actualStats, _ := CheckStats(checkID)

			// Assert that the published expvars use the same values as internal ones
			assert.Equal(t, actualStats, getCheckStatsExpvarMap(t)[checkName][checkID])

			assert.Equal(t, numCheckRuns, int(actualStats.TotalRuns))
			assert.Equal(t, numCheckRuns*2, int(actualStats.TotalWarnings))
			assert.Equal(t, numCheckRuns, int(actualStats.TotalErrors))
			assert.Equal(t, 4000, int(actualStats.AverageExecutionTime))
		}
	}

	// Test removal of stats

	for checkNameIdx := 0; checkNameIdx < numCheckNames; checkNameIdx++ {
		checkName := fmt.Sprintf("testcheck%d", checkNameIdx)

		for checkIDIdx := 0; checkIDIdx < numCheckInstances; checkIDIdx++ {
			assert.Equal(t, numCheckInstances-checkIDIdx, len(GetCheckStats()[checkName]))
			assert.Equal(t, numCheckInstances-checkIDIdx, len(getCheckStatsExpvarMap(t)[checkName]))

			checkID := checkid.ID(fmt.Sprintf("%s:%d", checkName, checkIDIdx))
			RemoveCheckStats(checkID)

			assert.Equal(t, numCheckInstances-checkIDIdx-1, len(GetCheckStats()[checkName]))
			assert.Equal(t, numCheckInstances-checkIDIdx-1, len(getCheckStatsExpvarMap(t)[checkName]))
		}
	}
}

func TestExpvarsGetChecksStatsClone(t *testing.T) {
	numCheckNames := 3
	numCheckInstances := 5
	numCheckRuns := 7

	// Add some data to the check stats
	for checkNameIdx := 0; checkNameIdx < numCheckNames; checkNameIdx++ {
		for checkIDIdx := 0; checkIDIdx < numCheckInstances; checkIDIdx++ {
			checkName := fmt.Sprintf("testcheck%d", checkNameIdx)
			checkID := fmt.Sprintf("%s:%d", checkName, checkIDIdx)
			testCheck := newTestCheck(checkID)

			for runIdx := 0; runIdx < numCheckRuns; runIdx++ {
				AddCheckStats(testCheck, 12345, nil, []error{}, stats.SenderStats{})
			}
		}
	}

	assert.Equal(t, numCheckNames, len(GetCheckStats()))
	assert.Equal(t, numCheckNames, len(getCheckStatsExpvarMap(t)))
	assert.Equal(t, numCheckInstances, len(GetCheckStats()["testcheck1"]))
	assert.Equal(t, numCheckInstances, len(getCheckStatsExpvarMap(t)["testcheck1"]))

	GetCheckStats()["testcheckx"] = make(map[checkid.ID]*stats.Stats)
	GetCheckStats()["testcheck1"]["abc"] = GetCheckStats()["testcheck3"]["testcheck3:1"]

	assert.Equal(t, numCheckNames, len(GetCheckStats()))
	assert.Equal(t, numCheckNames, len(getCheckStatsExpvarMap(t)))
	assert.Equal(t, numCheckInstances, len(GetCheckStats()["testcheck1"]))
	assert.Equal(t, numCheckInstances, len(getCheckStatsExpvarMap(t)["testcheck1"]))
}

func TestExpvarsRunningStats(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	require.Nil(t, err)

	runningChecksMap := getRunningChecksExpvarMap(t)
	assert.Equal(t, 0, len(getExpvarMapKeys(runningChecksMap)))

	for idx := 0; idx < 10; idx++ {
		checkName := fmt.Sprintf("mycheck %d", idx)
		expectedTimestamp := time.Unix(int64(1234567890+idx), 0).In(loc)

		SetRunningStats(checkid.ID(checkName), expectedTimestamp)

		actualInternalTimestamp := GetRunningStats(checkid.ID(checkName))
		assert.Equal(t, expectedTimestamp, actualInternalTimestamp)

		actualExpvarTimestamp := runningChecksMap.Get(checkName)
		assert.Equal(t, timestamp(expectedTimestamp).String(), actualExpvarTimestamp.String())

		assert.Equal(t, idx+1, len(getExpvarMapKeys(runningChecksMap)))
	}

	for idx := 0; idx < 10; idx++ {
		checkName := fmt.Sprintf("mycheck %d", idx)

		DeleteRunningStats(checkid.ID(checkName))

		actualExpvarTimestamp := runningChecksMap.Get(checkName)
		require.Nil(t, actualExpvarTimestamp)

		actualInternalTimestamp := GetRunningStats(checkid.ID(checkName))
		assert.True(t, actualInternalTimestamp.IsZero())

		assert.Equal(t, 10-idx-1, len(getExpvarMapKeys(runningChecksMap)))
	}
}

func TestExpvarsToplevelKeys(t *testing.T) {
	setUp()

	getters := map[string]func() int64{
		"Errors":        GetErrorsCount,
		"Runs":          GetRunsCount,
		"RunningChecks": GetRunningCheckCount,
		"Warnings":      GetWarningsCount,
	}

	for keyName, setter := range map[string]func(int){
		"Errors":        AddErrorsCount,
		"Runs":          AddRunsCount,
		"RunningChecks": AddRunningCheckCount,
		"Warnings":      AddWarningsCount,
	} {

		assertKeyNotSet(t, keyName)

		changeAndAssertExpvarValue(t, keyName, setter, getters[keyName], 5, 5)
		changeAndAssertExpvarValue(t, keyName, setter, getters[keyName], 1, 6)
		changeAndAssertExpvarValue(t, keyName, setter, getters[keyName], -5, 1)
		changeAndAssertExpvarValue(t, keyName, setter, getters[keyName], 3, 4)
	}
}
