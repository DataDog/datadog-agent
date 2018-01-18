// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package status

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
)

type TrackerSuite struct {
	suite.Suite
	tracker *Tracker
}

func (s *TrackerSuite) SetupTest() {
	s.tracker = NewTracker("foo")
}

func (s *TrackerSuite) TestPending() {
	source := s.tracker.GetSource()
	s.Equal("foo", source.Type)
	s.Equal("Pending", source.Status)
}

func (s *TrackerSuite) TestTrackSuccess() {
	s.tracker.TrackSuccess()
	source := s.tracker.GetSource()
	s.Equal("foo", source.Type)
	s.Equal("OK", source.Status)
}

func (s *TrackerSuite) TestTrackError() {
	s.tracker.TrackError(errors.New("bar"))
	source := s.tracker.GetSource()
	s.Equal("foo", source.Type)
	s.Equal("Error: bar", source.Status)
}

func TestTrackerSuite(t *testing.T) {
	suite.Run(t, new(TrackerSuite))
}
