// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package translator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/quantile"
	"github.com/DataDog/datadog-agent/pkg/quantile/summary"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
)

// TestMetrics is the struct used for serializing Datadog metrics for generating testdata.
// It contains sketches (distributions) and timeseries (all other types).
// This structure is not meant to be used directly; use AssertTranslatorMap instead.
type TestMetrics struct {
	Sketches   []TestSketch
	TimeSeries []TestTimeSeries
}

// TestDimensions copies the Dimensions struct with public fields.
// NOTE: Keep this in sync with the Dimensions struct.
type TestDimensions struct {
	Name     string
	Tags     []string
	Host     string
	OriginID string
}

type TestSketch struct {
	TestDimensions
	Timestamp uint64
	Summary   summary.Summary
	Keys      []int32
	Counts    []uint32
}

type TestTimeSeries struct {
	TestDimensions
	Type      MetricDataType
	Timestamp uint64
	Value     float64
}

// TestingT is an interface that defines a testing.T like object.
type TestingT interface {
	require.TestingT

	// Logf formats its arguments according to the format, analogous to Printf.
	Logf(format string, args ...any)
}

// AssertTranslatorMap asserts that some OTLP data is mapped into some other Datadog data by a given translator.
// OTLP data and Datadog data are stored in separate files passed as arguments.
// The Datadog data base filename must start with the OTLP data base filename possibly followed by the translator options.
//
// To generate OTLP data to be used on this assert, use the pmetric.JSONMarshaler and json.Indent.
// If the Datadog data does not match, a file ending in .actual will be generated containing the actual translator output.
func AssertTranslatorMap(t TestingT, translator *Translator, otlpfilename string, datadogfilename string) bool {
	// Check that the filenames follow conventions.
	prefix := strings.TrimSuffix(filepath.Base(otlpfilename), ".json")
	if !strings.HasPrefix(filepath.Base(datadogfilename), prefix) {
		t.Errorf("%q and %q do not follow prefix convention", otlpfilename, datadogfilename)
		return false
	}

	// Unmarshal OTLP data.
	otlpbytes, err := os.ReadFile(otlpfilename)
	require.NoError(t, err, "failed to read OTLP file %q", otlpfilename)

	var unmarshaler pmetric.JSONUnmarshaler
	otlpdata, err := unmarshaler.UnmarshalMetrics(otlpbytes)
	require.NoError(t, err, "failed to unmarshal OTLP data from file %q", otlpfilename)

	// Unmarshal expected Datadog data.
	datadogbytes, err := os.ReadFile(datadogfilename)
	require.NoError(t, err, "failed to read file %q", datadogfilename)

	var expecteddata TestMetrics
	err = json.Unmarshal(datadogbytes, &expecteddata)
	require.NoError(t, err, "failed to unmarshal Datadog data from file %q", datadogfilename)

	// Map metrics using translator.
	var consumer testConsumer
	err = translator.MapMetrics(context.Background(), otlpdata, &consumer)
	require.NoError(t, err)

	if !assert.Equal(t, expecteddata, consumer.testMetrics) {
		actualfile := datadogfilename + ".actual"
		t.Logf("Translator output does not match expected data, saving actual data on %q", actualfile)
		b, err := json.MarshalIndent(&consumer.testMetrics, "", "  ")
		require.NoError(t, err)

		err = os.WriteFile(actualfile, b, 0660)
		require.NoError(t, err)
		return false
	}

	return true
}

var _ Consumer = (*testConsumer)(nil)

type testConsumer struct {
	testMetrics TestMetrics
}

func (t *testConsumer) ConsumeAPMStats(_ pb.ClientStatsPayload) {
	// not used for this consumer, but do warn the user if they
	// try to use it
	panic("(*testConsumer).ConsumeAPMStats not implemented")
}

func (t *testConsumer) ConsumeTimeSeries(
	_ context.Context,
	dimensions *Dimensions,
	typ MetricDataType,
	timestamp uint64,
	value float64,
) {
	t.testMetrics.TimeSeries = append(t.testMetrics.TimeSeries,
		TestTimeSeries{
			TestDimensions: TestDimensions{
				Name:     dimensions.Name(),
				Tags:     dimensions.Tags(),
				Host:     dimensions.Host(),
				OriginID: dimensions.OriginID(),
			},
			Type:      typ,
			Timestamp: timestamp,
			Value:     value,
		})
}

func (t *testConsumer) ConsumeSketch(
	_ context.Context,
	dimensions *Dimensions,
	timestamp uint64,
	sketch *quantile.Sketch,
) {
	k, n := sketch.Cols()
	t.testMetrics.Sketches = append(t.testMetrics.Sketches,
		TestSketch{
			TestDimensions: TestDimensions{
				Name:     dimensions.Name(),
				Tags:     dimensions.Tags(),
				Host:     dimensions.Host(),
				OriginID: dimensions.OriginID(),
			},
			Timestamp: timestamp,
			Summary:   sketch.Basic,
			Keys:      k,
			Counts:    n,
		},
	)
}

// TestTestDimensions tests that TestDimensions fields match those of Dimensions.
func TestTestDimensions(t *testing.T) {
	testType := reflect.TypeOf(TestDimensions{})
	var testFields []string
	for i := 0; i < testType.NumField(); i++ {
		testFields = append(testFields,
			strings.ToLower(testType.Field(i).Name),
		)
	}

	trueType := reflect.TypeOf(Dimensions{})
	var trueFields []string
	for i := 0; i < trueType.NumField(); i++ {
		trueFields = append(trueFields,
			strings.ToLower(trueType.Field(i).Name),
		)
	}

	assert.ElementsMatch(t, testFields, trueFields,
		"The fields on TestDimensions and Dimensions are out of sync. Ensure that they have the exact same fields.")
}

var _ TestingT = (*testingTMock)(nil)

// testingTMock mocks a testing object for all your meta-testing needs.
type testingTMock struct{ t *testing.T }

// Errorf implements the TestingT interface.
func (m *testingTMock) Errorf(format string, args ...interface{}) {
	m.t.Logf("Would have failed with: "+format, args...)
}

// FailNow implements the TestingT interface.
func (m *testingTMock) FailNow() {
	m.t.FailNow()
}

// Logf implements the TestingT interface.
func (m *testingTMock) Logf(format string, args ...interface{}) {
	m.t.Logf("Would have logged: "+format, args...)
}

// TestAssertTranslatorMapFailure tests that AssertTranslatorMap fails correctly when inputs and outputs mismatch.
func TestAssertTranslatorMapFailure(t *testing.T) {
	otlpfile := "testdata/otlpdata/histogram/simple-delta.json"
	// Compare OTLP file with incorrect output
	ddogfile := "testdata/datadogdata/histogram/simple-delta_nobuckets-cs.json"

	translator, err := New(zap.NewNop(), WithHistogramMode(HistogramModeDistributions))
	require.NoError(t, err)
	mockTesting := &testingTMock{t}
	assert.False(t, AssertTranslatorMap(mockTesting, translator, otlpfile, ddogfile), "AssertTranslatorMap should have failed but did not")
	actualFile := ddogfile + ".actual"
	if assert.FileExists(t, actualFile, "AssertTranslatorMap did not create .actual file") {
		assert.True(t, AssertTranslatorMap(mockTesting, translator, otlpfile, actualFile), "AssertTranslatorMap should have passed with .actual output")
		require.NoError(t, os.Remove(actualFile))
	}
}
