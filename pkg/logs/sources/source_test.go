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

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
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

func TestTrackerSuite(t *testing.T) {
	suite.Run(t, new(LogSourceSuite))
}

// TestConcurrentTailingModeAndStatusAccess guards against a regression of the data race
// between the file launcher mutating TailingMode/Status and the status builder reading them.
func TestConcurrentTailingModeAndStatusAccess(t *testing.T) {
	t.Parallel()
	source := NewLogSource("test", &config.LogsConfig{TailingMode: "end"})

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// writer: mimics (*Launcher).launchTailers/addSource mutating the source concurrently.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			if i%2 == 0 {
				source.SetTailingMode("beginning")
			} else {
				source.SetTailingMode("end")
			}
			source.SetStatus(NewLogSource("test", nil).Status())
		}
	}()

	// reader: mimics (*Builder).toDictionary/getIntegrations reading the source concurrently.
	for i := 0; i < 1000; i++ {
		_ = source.GetTailingMode()
		_ = source.Status()
	}
	close(stop)
	wg.Wait()
}
