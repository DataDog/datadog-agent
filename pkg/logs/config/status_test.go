// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package config

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
)

type LogStatusSuite struct {
	suite.Suite
	status *LogStatus
}

func (s *LogStatusSuite) TestPending() {
	s.status = NewLogStatus()
	s.True(s.status.IsPending())
	s.False(s.status.IsSuccess())
	s.False(s.status.IsError())
	s.Equal("", s.status.GetError())
}

func (s *LogStatusSuite) TestSuccess() {
	s.status = NewLogStatus()
	s.status.Success()
	s.False(s.status.IsPending())
	s.True(s.status.IsSuccess())
	s.False(s.status.IsError())
	s.Equal("", s.status.GetError())
}

func (s *LogStatusSuite) TestError() {
	s.status = NewLogStatus()
	s.status.Error(errors.New("bar"))
	s.False(s.status.IsPending())
	s.False(s.status.IsSuccess())
	s.True(s.status.IsError())
	s.Equal("Error: bar", s.status.GetError())
}

func TestLogStatusSuite(t *testing.T) {
	suite.Run(t, new(LogStatusSuite))
}
