// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sources

import (
	"sync"
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

// TestDumpConcurrentWithProcessingInfo reproduces a data race where Dump()
// formats the ProcessingInfo field with a verb that reads its raw fields
// (the rules map and the embedded sync.Mutex's internal state) via
// reflection, bypassing ProcessingInfo's own locking, while another
// goroutine concurrently calls ProcessingInfo.Inc() (e.g. from the
// processing pipeline handling in-flight messages). Run with -race to
// verify.
func (s *LogSourceSuite) TestDumpConcurrentWithProcessingInfo() {
	source := NewLogSource("racesource", nil)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				source.ProcessingInfo.Inc("exclude_rule")
			}
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			source.Dump(true)
		}
		close(stop)
	}()

	wg.Wait()
}

func TestTrackerSuite(t *testing.T) {
	suite.Run(t, new(LogSourceSuite))
}
