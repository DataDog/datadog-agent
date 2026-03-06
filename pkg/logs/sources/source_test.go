// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sources

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type LogSourceSuite struct {
	suite.Suite
	source *LogSource
}

func (s *LogSourceSuite) TestInputs() {
	s.source = NewLogSource("", nil)
	s.Equal(0, len(s.source.GetInputs()))
	s.source.AddInput("foo")
	s.Equal(1, len(s.source.GetInputs()))
	s.Equal("foo", s.source.GetInputs()[0])
	s.source.RemoveInput("foo")
	s.Equal(0, len(s.source.GetInputs()))
	s.source.RemoveInput("bar")

}

func (s *LogSourceSuite) TestDump() {
	s.source = NewLogSource("mysource", nil)
	dump := s.source.Dump(true)
	assert.Contains(s.T(), dump, "mysource")
}

func (s *LogSourceSuite) TestDumpSingleLine() {
	s.source = NewLogSource("mysource", nil)
	dump := s.source.Dump(false)
	assert.Contains(s.T(), dump, "mysource")
	// Single line dump should not contain newlines in the main structure
	assert.NotContains(s.T(), dump, "\n\t")
}

func (s *LogSourceSuite) TestDumpNilSource() {
	var nilSource *LogSource
	dump := nilSource.Dump(true)
	assert.Equal(s.T(), "&LogSource(nil)", dump)
}

func (s *LogSourceSuite) TestSourceType() {
	s.source = NewLogSource("test", nil)

	// Default source type should be empty
	assert.Equal(s.T(), SourceType(""), s.source.GetSourceType())

	// Set and get source type
	s.source.SetSourceType(DockerSourceType)
	assert.Equal(s.T(), DockerSourceType, s.source.GetSourceType())

	s.source.SetSourceType(KubernetesSourceType)
	assert.Equal(s.T(), KubernetesSourceType, s.source.GetSourceType())

	s.source.SetSourceType(IntegrationSourceType)
	assert.Equal(s.T(), IntegrationSourceType, s.source.GetSourceType())
}

func (s *LogSourceSuite) TestHiddenFromStatus() {
	s.source = NewLogSource("test", nil)

	// Default should be visible
	assert.False(s.T(), s.source.IsHiddenFromStatus())

	// Hide from status
	s.source.HideFromStatus()
	assert.True(s.T(), s.source.IsHiddenFromStatus())
}

func (s *LogSourceSuite) TestRecordBytes() {
	s.source = NewLogSource("test", nil)

	// Record bytes
	s.source.RecordBytes(100)
	assert.Equal(s.T(), int64(100), s.source.BytesRead.Get())

	s.source.RecordBytes(50)
	assert.Equal(s.T(), int64(150), s.source.BytesRead.Get())
}

func (s *LogSourceSuite) TestRecordBytesWithParent() {
	parent := NewLogSource("parent", nil)
	child := NewLogSource("child", nil)
	child.ParentSource = parent

	// Record bytes on child should also update parent
	child.RecordBytes(100)
	assert.Equal(s.T(), int64(100), child.BytesRead.Get())
	assert.Equal(s.T(), int64(100), parent.BytesRead.Get())
}

func (s *LogSourceSuite) TestGetInfoStatus() {
	s.source = NewLogSource("test", nil)

	// GetInfoStatus should return a map (already has default info providers registered)
	infoStatus := s.source.GetInfoStatus()
	assert.NotNil(s.T(), infoStatus)
}

func (s *LogSourceSuite) TestGetInfo() {
	s.source = NewLogSource("test", nil)

	// BytesRead should be registered by default
	info := s.source.GetInfo("Bytes Read")
	assert.NotNil(s.T(), info)

	// Non-existent key should return nil
	info = s.source.GetInfo("nonexistent")
	assert.Nil(s.T(), info)
}

func TestTrackerSuite(t *testing.T) {
	suite.Run(t, new(LogSourceSuite))
}
