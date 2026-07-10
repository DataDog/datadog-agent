// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package smartadaptivesampling

import (
	"testing"

	"github.com/stretchr/testify/assert"

	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
)

type fakeReader struct {
	level severityeventsdef.SeverityLevel
}

func (f *fakeReader) GetSeverity() severityeventsdef.SeverityLevel {
	return f.level
}

func (f *fakeReader) set(level severityeventsdef.SeverityLevel) {
	f.level = level
}

func resetForTest(t *testing.T) {
	t.Cleanup(func() { activeReader.Store(nil) })
	activeReader.Store(nil)
}

func TestCurrent_NoReaderRegistered_DefaultsToLow(t *testing.T) {
	resetForTest(t)

	level, ok := Current()
	assert.False(t, ok)
	assert.Equal(t, severityeventsdef.SeverityLow, level)
}

func TestCurrent_ReflectsRegisteredReader(t *testing.T) {
	resetForTest(t)

	fake := &fakeReader{}
	SetReader(fake)

	fake.set(severityeventsdef.SeverityHigh)
	level, ok := Current()
	assert.True(t, ok)
	assert.Equal(t, severityeventsdef.SeverityHigh, level)

	fake.set(severityeventsdef.SeverityMedium)
	level, ok = Current()
	assert.True(t, ok)
	assert.Equal(t, severityeventsdef.SeverityMedium, level)
}
