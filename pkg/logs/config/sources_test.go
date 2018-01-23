// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
)

type LogSourcesSuite struct {
	suite.Suite
	sources *LogSources
}

func (s *LogSourcesSuite) TestGetSources() {
	s.sources = newLogSources([]*LogSource{})
	s.Equal(0, len(s.sources.GetSources()))
	s.sources = newLogSources([]*LogSource{NewLogSource("", nil)})
	s.Equal(1, len(s.sources.GetSources()))
}

func (s *LogSourcesSuite) TestGetValidSources() {
	source1 := NewLogSource("", nil)
	source2 := NewLogSource("", nil)
	s.sources = newLogSources([]*LogSource{source1, source2})
	s.Equal(2, len(s.sources.GetValidSources()))
	source1.Status.Error(errors.New("invalid"))
	s.Equal(1, len(s.sources.GetValidSources()))
	source1.Status.Success()
	s.Equal(2, len(s.sources.GetValidSources()))
}

func TestLogSourcesSuite(t *testing.T) {
	suite.Run(t, new(LogSourcesSuite))
}
