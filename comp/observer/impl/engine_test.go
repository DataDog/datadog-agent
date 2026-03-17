// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// --- Spy helpers ---

// spyDetector records Setup calls and can optionally look up a component by name.
type spyDetector struct {
	name         string
	setupCalled  bool
	resolvedName string
	resolved     any
}

func (s *spyDetector) Name() string { return s.name }
func (s *spyDetector) Setup(get observerdef.GetComponentFunc) error {
	s.setupCalled = true
	if s.resolvedName != "" {
		s.resolved = get(s.resolvedName)
	}
	return nil
}

func (s *spyDetector) Detect(_ observerdef.StorageReader, _ int64) observerdef.DetectionResult {
	return observerdef.DetectionResult{}
}

// spyCorrelator records Setup calls and can optionally look up a component by name.
type spyCorrelator struct {
	name         string
	setupCalled  bool
	resolvedName string
	resolved     any
}

func (s *spyCorrelator) Name() string { return s.name }
func (s *spyCorrelator) Setup(get observerdef.GetComponentFunc) error {
	s.setupCalled = true
	if s.resolvedName != "" {
		s.resolved = get(s.resolvedName)
	}
	return nil
}
func (s *spyCorrelator) ProcessAnomaly(_ observerdef.Anomaly)                {}
func (s *spyCorrelator) Advance(_ int64)                                     {}
func (s *spyCorrelator) ActiveCorrelations() []observerdef.ActiveCorrelation { return nil }
func (s *spyCorrelator) Reset()                                              {}

// spyExtractor records Setup calls and can optionally look up a component by name.
type spyExtractor struct {
	name         string
	setupCalled  bool
	resolvedName string
	resolved     any
}

func (s *spyExtractor) Name() string { return s.name }
func (s *spyExtractor) Setup(get observerdef.GetComponentFunc) error {
	s.setupCalled = true
	if s.resolvedName != "" {
		s.resolved = get(s.resolvedName)
	}
	return nil
}
func (s *spyExtractor) ProcessLog(_ observerdef.LogView) []observerdef.MetricOutput { return nil }

// errorCorrelator returns an error from Setup.
type errorCorrelator struct{ spyCorrelator }

func (e *errorCorrelator) Setup(_ observerdef.GetComponentFunc) error {
	e.setupCalled = true
	return errors.New("setup failed")
}

// --- Tests ---

func newTestEngine(t testing.TB, detectors []observerdef.Detector, correlators []observerdef.Correlator, extractors []observerdef.LogMetricsExtractor) *engine {
	t.Helper()
	return mustNewEngine(t, engineConfig{
		storage:     newTimeSeriesStorage(),
		detectors:   detectors,
		correlators: correlators,
		extractors:  extractors,
	})
}

// TestEngine_SetupCalledOnDetectors verifies that Setup is invoked for every detector.
func TestEngine_SetupCalledOnDetectors(t *testing.T) {
	d1 := &spyDetector{name: "detector_a"}
	d2 := &spyDetector{name: "detector_b"}

	newTestEngine(t, []observerdef.Detector{d1, d2}, nil, nil)

	assert.True(t, d1.setupCalled, "Setup should be called on detector_a")
	assert.True(t, d2.setupCalled, "Setup should be called on detector_b")
}

// TestEngine_SetupCalledOnCorrelators verifies that Setup is invoked for every correlator.
func TestEngine_SetupCalledOnCorrelators(t *testing.T) {
	c1 := &spyCorrelator{name: "correlator_a"}
	c2 := &spyCorrelator{name: "correlator_b"}

	newTestEngine(t, nil, []observerdef.Correlator{c1, c2}, nil)

	assert.True(t, c1.setupCalled, "Setup should be called on correlator_a")
	assert.True(t, c2.setupCalled, "Setup should be called on correlator_b")
}

// TestEngine_SetupCalledOnExtractors verifies that Setup is invoked for every extractor.
func TestEngine_SetupCalledOnExtractors(t *testing.T) {
	e1 := &spyExtractor{name: "extractor_a"}
	e2 := &spyExtractor{name: "extractor_b"}

	newTestEngine(t, nil, nil, []observerdef.LogMetricsExtractor{e1, e2})

	assert.True(t, e1.setupCalled, "Setup should be called on extractor_a")
	assert.True(t, e2.setupCalled, "Setup should be called on extractor_b")
}

// TestEngine_SetupDetectorCanResolveExtractor verifies that a detector can look up an
// extractor by name via the getComponent function passed to Setup.
func TestEngine_SetupDetectorCanResolveExtractor(t *testing.T) {
	ext := &spyExtractor{name: "my_extractor"}
	det := &spyDetector{name: "my_detector", resolvedName: "my_extractor"}

	newTestEngine(t, []observerdef.Detector{det}, nil, []observerdef.LogMetricsExtractor{ext})

	require.NotNil(t, det.resolved, "detector should resolve extractor via getComponent")
	assert.Equal(t, ext, det.resolved)
}

// TestEngine_SetupCorrelatorCanResolveDetector verifies that a correlator can look up a
// detector by name via the getComponent function passed to Setup.
func TestEngine_SetupCorrelatorCanResolveDetector(t *testing.T) {
	det := &spyDetector{name: "my_detector"}
	cor := &spyCorrelator{name: "my_correlator", resolvedName: "my_detector"}

	newTestEngine(t, []observerdef.Detector{det}, []observerdef.Correlator{cor}, nil)

	require.NotNil(t, cor.resolved, "correlator should resolve detector via getComponent")
	assert.Equal(t, det, cor.resolved)
}

// TestEngine_SetupExtractorCanResolveCorrelator verifies that an extractor can look up a
// correlator by name via the getComponent function passed to Setup.
func TestEngine_SetupExtractorCanResolveCorrelator(t *testing.T) {
	cor := &spyCorrelator{name: "my_correlator"}
	ext := &spyExtractor{name: "my_extractor", resolvedName: "my_correlator"}

	newTestEngine(t, nil, []observerdef.Correlator{cor}, []observerdef.LogMetricsExtractor{ext})

	require.NotNil(t, ext.resolved, "extractor should resolve correlator via getComponent")
	assert.Equal(t, cor, ext.resolved)
}

// TestEngine_SetupUnknownComponentReturnsNil verifies that requesting a non-existent
// component by name returns nil rather than panicking.
func TestEngine_SetupUnknownComponentReturnsNil(t *testing.T) {
	det := &spyDetector{name: "my_detector", resolvedName: "does_not_exist"}

	newTestEngine(t, []observerdef.Detector{det}, nil, nil)

	assert.Nil(t, det.resolved, "unknown component should resolve to nil")
}

// TestEngine_SetupErrorPropagated verifies that newEngine returns an error when a
// component's Setup fails, including the component name in the error message.
func TestEngine_SetupErrorPropagated(t *testing.T) {
	failing := &errorCorrelator{}
	failing.name = "bad_correlator"

	_, err := newEngine(engineConfig{
		storage:     newTimeSeriesStorage(),
		correlators: []observerdef.Correlator{failing},
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, "bad_correlator")
	assert.ErrorContains(t, err, "setup failed")
	assert.True(t, failing.setupCalled)
}
