// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package worker

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
	"github.com/DataDog/datadog-agent/pkg/config"
)

type stubCheck struct {
	check.StubCheck
	id string
}

func (c *stubCheck) ID() check.ID   { return check.ID(c.id) }
func (c *stubCheck) String() string { return check.IDToCheckName(c.ID()) }

func newTestCheck(id string) *stubCheck {
	return &stubCheck{id: id}
}

func addExpvarsCheckStats(c check.Check) {
	expvars.AddCheckStats(c, 0, nil, nil, check.SenderStats{})
}

func setUp() {
	config.Datadog.Set(loggingFrequencyConfigKey, fmt.Sprintf("20"))

	expvars.Reset()
}

func TestShouldLogNewCheck(t *testing.T) {
	setUp()

	for idx := 0; idx < 10; idx++ {
		fakeID := check.ID(fmt.Sprintf("testcheck %d", idx))

		shouldLog, lastVerboseLog := shouldLogCheck(fakeID)

		assert.True(t, shouldLog)
		assert.False(t, lastVerboseLog)
	}
}

func TestShouldLogLastVerboseLog(t *testing.T) {
	setUp()

	for idx := 0; idx < 10; idx++ {
		testCheck := newTestCheck(fmt.Sprintf("testcheck %d", idx))

		for logIdx := 1; logIdx < 10; logIdx++ {
			addExpvarsCheckStats(testCheck)

			_, lastVerboseLog := shouldLogCheck(testCheck.ID())

			// initialCheckLoggingSeriesLimit should be 5
			if logIdx == 5 {
				assert.True(t, lastVerboseLog, fmt.Sprintf("Loop idx: %d", logIdx))
			} else {
				assert.False(t, lastVerboseLog, fmt.Sprintf("Loop idx: %d", logIdx))
			}
		}
	}
}

func TestShouldLogInitialCheckLoggingSeries(t *testing.T) {
	setUp()

	for idx := 0; idx < 5; idx++ {
		testCheck := newTestCheck(fmt.Sprintf("testcheck %d", idx))

		for logIdx := 1; logIdx < 61; logIdx++ {
			addExpvarsCheckStats(testCheck)

			shouldLog, _ := shouldLogCheck(testCheck.ID())

			// initialCheckLoggingSeriesLimit is 5 and we use 20 as our log limit config value in tests
			if logIdx <= 5 || logIdx%20 == 0 {
				assert.True(t, shouldLog, fmt.Sprintf("Loop idx: %d", logIdx))
			} else {
				assert.False(t, shouldLog, fmt.Sprintf("Loop idx: %d", logIdx))
			}
		}
	}
}
