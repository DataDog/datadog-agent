// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sources

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/comp/logs-library/tagfilter"
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

// TestDumpConcurrentWithProcessingInfo runs Dump() concurrently with ProcessingInfo.Inc() to catch races (-race).
func (s *LogSourceSuite) TestDumpConcurrentWithProcessingInfo() {
	source := NewLogSource("racesource", nil)

	stop := make(chan struct{})
	var wg sync.WaitGroup

	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
				source.ProcessingInfo.Inc("exclude_rule")
			}
		}
	})

	wg.Go(func() {
		for i := 0; i < 1000; i++ {
			source.Dump(true)
		}
		close(stop)
	})

	wg.Wait()
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
	wg.Go(func() {
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
	})

	// reader: mimics (*Builder).configToDictionary/getIntegrations reading the source concurrently.
	for i := 0; i < 1000; i++ {
		_ = source.GetTailingMode()
		_ = source.Status()
	}
	close(stop)
	wg.Wait()
}

// TestConcurrentPublicJSONAndSetTailingMode verifies that PublicJSON (which reads
// Config.TailingMode) does not race with SetTailingMode (which writes it).
// Before the fix, PublicJSON was called on Config directly without the LogSource
// lock, racing with SetTailingMode on another goroutine.
func TestConcurrentPublicJSONAndSetTailingMode(t *testing.T) {
	t.Parallel()

	source := NewLogSource("test", &config.LogsConfig{
		Type:        "file",
		Path:        "/var/log/test.log",
		TailingMode: "end",
	})

	const iterations = 5000
	var wg sync.WaitGroup
	var start sync.WaitGroup
	start.Add(1)

	// writer: mimics (*Launcher).launchTailers mutating TailingMode concurrently.
	wg.Go(func() {
		start.Wait()
		for i := range iterations {
			if i%2 == 0 {
				source.SetTailingMode("beginning")
			} else {
				source.SetTailingMode("end")
			}
		}
	})

	// reader: mimics inventorychecksImpl.getPayload calling PublicJSON concurrently.
	wg.Go(func() {
		start.Wait()
		for range iterations {
			_, _ = source.PublicJSON()
		}
	})

	start.Done()
	wg.Wait()
}

// fakeTagFilter is a minimal TagFilter for exercising LogSource's tag filter storage.
type fakeTagFilter struct{}

func (fakeTagFilter) Keep(tags []string) []string { return tags }
func (fakeTagFilter) Retains(string) bool         { return true }

func TestLogSourceTagFilterUnset(t *testing.T) {
	source := NewLogSource("test", nil)

	f, ok := source.TagFilter()
	assert.False(t, ok)
	assert.True(t, f == nil)
	assert.Nil(t, source.TagFilterState())
}

func TestLogSourceTagFilterRoundTrip(t *testing.T) {
	source := NewLogSource("test", nil)
	want := fakeTagFilter{}

	ok := source.CompareAndSwapTagFilterState(nil, NewTagFilterState(nil, want))
	assert.True(t, ok)

	got, resolved := source.TagFilter()
	assert.True(t, resolved)
	assert.Equal(t, TagFilter(want), got)
}

// TestLogSourceTagFilterResolvedInert guards the middle of the three states: a
// non-nil state whose filter is nil must read back as resolved with a filter
// that compares equal to nil, not as unresolved.
func TestLogSourceTagFilterResolvedInert(t *testing.T) {
	source := NewLogSource("test", nil)

	ok := source.CompareAndSwapTagFilterState(nil, NewTagFilterState(nil, nil))
	assert.True(t, ok)

	got, resolved := source.TagFilter()
	assert.True(t, resolved)
	assert.True(t, got == nil)
}

// TestLogSourceSetTagFilterNil guards the typed-nil trap: passing the untyped nil literal
// must still yield an interface value that compares equal to nil, even after a real
// filter was cached for an earlier generation.
func TestLogSourceSetTagFilterNil(t *testing.T) {
	source := NewLogSource("test", nil)
	gen1 := &tagfilter.Filters{}
	gen2 := &tagfilter.Filters{}

	assert.True(t, source.CompareAndSwapTagFilterState(nil, NewTagFilterState(gen1, fakeTagFilter{})))
	assert.True(t, source.CompareAndSwapTagFilterState(source.TagFilterState(), NewTagFilterState(gen2, nil)))

	got, ok := source.TagFilter()
	assert.True(t, ok)
	assert.True(t, got == nil)
}

// TestLogSourceTagFilterStateResolvedForGeneration pins the generation check that
// lets a resolver detect a stale cache after a source outlives a config reload.
func TestLogSourceTagFilterStateResolvedForGeneration(t *testing.T) {
	gen1 := &tagfilter.Filters{}
	gen2 := &tagfilter.Filters{}
	source := NewLogSource("test", nil)

	assert.False(t, source.TagFilterState().ResolvedFor(gen1), "unresolved state must never match any generation")

	source.CompareAndSwapTagFilterState(nil, NewTagFilterState(gen1, nil))
	assert.True(t, source.TagFilterState().ResolvedFor(gen1))
	assert.False(t, source.TagFilterState().ResolvedFor(gen2))
}

// TestLogSourceCompareAndSwapTagFilterState_ExactlyOneWinner races goroutines
// through the same compare-and-swap and counts successes. ResolveSourceTagFilter
// gates its one-time side effects (RegisterInfo, Messages.AddMessage) behind this
// exact call, so exactly one winner here is what makes "side effects run once" true.
func TestLogSourceCompareAndSwapTagFilterState_ExactlyOneWinner(t *testing.T) {
	source := NewLogSource("test", nil)
	old := source.TagFilterState() // nil: unresolved

	var wins atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if source.CompareAndSwapTagFilterState(old, NewTagFilterState(nil, fakeTagFilter{})) {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(1), wins.Load())
}

// TestLogSourceTagFilterConcurrent runs CompareAndSwapTagFilterState and TagFilter
// concurrently to catch races (-race).
func TestLogSourceTagFilterConcurrent(t *testing.T) {
	source := NewLogSource("racesource", nil)

	stop := make(chan struct{})
	var wg sync.WaitGroup

	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
				source.CompareAndSwapTagFilterState(source.TagFilterState(), NewTagFilterState(nil, fakeTagFilter{}))
			}
		}
	})

	wg.Go(func() {
		for i := 0; i < 1000; i++ {
			source.TagFilter()
		}
		close(stop)
	})

	wg.Wait()
}
