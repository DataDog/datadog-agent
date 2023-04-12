// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
//go:build windows
// +build windows

package evtlog

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/reporter"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"golang.org/x/sys/windows"
)

type GetEventsTestSuite struct {
	suite.Suite

	channelPath string
	eventSource string
	testAPI     string
	numEvents   uint

	sender *mocksender.MockSender
	ti     eventlog_test.APITester
}

func (s *GetEventsTestSuite) SetupSuite() {
	s.ti = eventlog_test.GetAPITesterByName(s.testAPI, s.T())
	err := s.ti.InstallChannel(s.channelPath)
	require.NoError(s.T(), err)
	err = s.ti.InstallSource(s.channelPath, s.eventSource)
	require.NoError(s.T(), err)
	s.sender = mocksender.NewMockSender("")
}

func (s *GetEventsTestSuite) TearDownSuite() {
	s.ti.RemoveSource(s.channelPath, s.eventSource)
	s.ti.RemoveChannel(s.channelPath)
}

func (s *GetEventsTestSuite) SetupTest() {
	// Ensure the log is empty
	err := s.ti.API().EvtClearLog(s.channelPath)
	require.NoError(s.T(), err)
	resetSender(s.sender)
}

func (s *GetEventsTestSuite) TearDownTest() {
	err := s.ti.API().EvtClearLog(s.channelPath)
	require.NoError(s.T(), err)
	resetSender(s.sender)
}

func resetSender(sender *mocksender.MockSender) {
	// Reset collected calls
	sender.ResetCalls()
	// Reset expected calls
	sender.Mock.ExpectedCalls = sender.Mock.ExpectedCalls[0:0]
}

func (s *GetEventsTestSuite) newCheck(instanceConfig []byte, initConfig []byte) (*Check, error) {
	check := new(Check)
	check.evtapi = s.ti.API()
	err := check.Configure(integration.FakeConfigHash, instanceConfig, initConfig, "test")
	if !assert.NoError(s.T(), err) {
		return nil, err
	}
	mocksender.SetSender(s.sender, check.ID())

	return check, nil
}

func TestLaunchGetEventsTestSuite(t *testing.T) {
	testerNames := eventlog_test.GetEnabledAPITesters()

	for _, tiName := range testerNames {
		t.Run(fmt.Sprintf("%sAPI", tiName), func(t *testing.T) {
			if tiName == "Fake" {
				t.Skip("Fake API does not implement EvtRenderValues")
			}
			var s GetEventsTestSuite
			s.channelPath = "dd-test-channel-check"
			s.eventSource = "dd-test-source-check"
			s.testAPI = tiName
			s.numEvents = 1000
			suite.Run(t, &s)
		})
	}
}

func countEvents(check *Check, senderEventCall *mock.Call, numEvents uint) uint {
	eventsCollected := uint(0)
	prevEventsCollected := uint(0)
	senderEventCall.Run(func(args mock.Arguments) {
		eventsCollected += 1
	})
	for {
		check.Run()
		if eventsCollected == numEvents || prevEventsCollected == eventsCollected {
			break
		}
		prevEventsCollected = eventsCollected
	}
	return eventsCollected
}

// Test that a simple check config can collect events
func (s *GetEventsTestSuite) TestGetEvents() {
	// Put events in the log
	err := s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	instanceConfig := []byte(fmt.Sprintf(`
path: %s
start: old
`,
		s.channelPath))

	check, err := s.newCheck(instanceConfig, nil)
	require.NoError(s.T(), err)
	defer check.Cancel()

	s.sender.On("Commit").Return()
	senderEventCall := s.sender.On("Event", mock.Anything)

	eventsCollected := countEvents(check, senderEventCall, s.numEvents)

	require.Equal(s.T(), s.numEvents, eventsCollected)
	s.sender.AssertExpectations(s.T())
}

// Test that event record levels are correctly converted to Datadog Event Alerty Types
func (s *GetEventsTestSuite) TestLevels() {
	tests := []struct {
		name        string
		reportLevel uint
		alertType   string
	}{
		{"info", windows.EVENTLOG_INFORMATION_TYPE, "info"},
		{"warning", windows.EVENTLOG_WARNING_TYPE, "warning"},
		{"error", windows.EVENTLOG_ERROR_TYPE, "error"},
	}

	reporter, err := evtreporter.New(s.eventSource, s.ti.API())
	require.NoError(s.T(), err)
	defer reporter.Close()

	for _, tc := range tests {
		s.Run(tc.name, func() {
			defer resetSender(s.sender)

			alertType, err := metrics.GetAlertTypeFromString(tc.alertType)
			require.NoError(s.T(), err)

			instanceConfig := []byte(fmt.Sprintf(`
path: %s
start: now
`,
				s.channelPath))

			check, err := s.newCheck(instanceConfig, nil)
			require.NoError(s.T(), err)
			defer check.Cancel()

			// report event
			err = reporter.ReportEvent(tc.reportLevel, 0, 1000, []string{"teststring"}, nil)
			require.NoError(s.T(), err)

			s.sender.On("Commit").Return().Once()
			s.sender.On("Event", mock.MatchedBy(func(e metrics.Event) bool {
				return e.AlertType == alertType
			})).Once()

			check.Run()

			s.sender.AssertExpectations(s.T())
		})
	}
}

// Test that the event_priority configuration value is correctly applied to the Datadog Event Priority
func (s *GetEventsTestSuite) TestPriority() {
	tests := []struct {
		name          string
		confPriority  string
		eventPriority string
	}{
		{"low", "low", "low"},
		{"normal", "normal", "normal"},
		{"default", "", "normal"},
	}

	reporter, err := evtreporter.New(s.eventSource, s.ti.API())
	require.NoError(s.T(), err)
	defer reporter.Close()

	for _, tc := range tests {
		s.Run(tc.name, func() {
			defer resetSender(s.sender)

			eventPriority, err := metrics.GetEventPriorityFromString(tc.eventPriority)
			require.NoError(s.T(), err)

			instanceConfig := []byte(fmt.Sprintf(`
path: %s
start: now
`,
				s.channelPath))

			if len(tc.confPriority) > 0 {
				instanceConfig = append(instanceConfig, []byte(fmt.Sprintf("event_priority: %s", tc.confPriority))...)
			}

			check, err := s.newCheck(instanceConfig, nil)
			require.NoError(s.T(), err)
			defer check.Cancel()

			// report event
			err = reporter.ReportEvent(windows.EVENTLOG_INFORMATION_TYPE, 0, 1000, []string{"teststring"}, nil)
			require.NoError(s.T(), err)

			s.sender.On("Commit").Return().Once()
			s.sender.On("Event", mock.MatchedBy(func(e metrics.Event) bool {
				return e.Priority == eventPriority
			})).Once()

			check.Run()

			s.sender.AssertExpectations(s.T())
		})
	}
}

// Tests that the Event Query configuration value succesfully filters event records
func (s *GetEventsTestSuite) TestGetEventsWithQuery() {
	reporter, err := evtreporter.New(s.eventSource, s.ti.API())
	require.NoError(s.T(), err)
	defer reporter.Close()

	// Query for EventID=1000
	instanceConfig := []byte(fmt.Sprintf(`
path: %s
start: now
query: |
  <QueryList>
    <Query Id="0" Path="%s">
      <Select Path="%s">*[System[(EventID=1000)]]</Select>
    </Query>
  </QueryList>
`,
		s.channelPath, s.channelPath, s.channelPath))

	check, err := s.newCheck(instanceConfig, nil)
	require.NoError(s.T(), err)
	defer check.Cancel()

	matchstring := "match this string"
	nomatchstring := "should not match"
	s.sender.On("Commit").Return().Once()
	s.sender.On("Event", mock.MatchedBy(func(e metrics.Event) bool {
		return assert.Contains(s.T(), e.Text, matchstring, "reported events should match the query")
	})).Once()

	// Generate an event the query should match on (EventID=1000)
	err = reporter.ReportEvent(windows.EVENTLOG_INFORMATION_TYPE, 0, 1000, []string{matchstring}, nil)
	// Generate an event the query should not match on (EventID!=1000)
	err = reporter.ReportEvent(windows.EVENTLOG_INFORMATION_TYPE, 0, 999, []string{nomatchstring}, nil)

	check.Run()

	s.sender.AssertExpectations(s.T())
}

// Tests that the tag_event_id configuration option results in an event_id tag
func (s *GetEventsTestSuite) TestGetEventsWithTagEventID() {
	tests := []struct {
		name     string
		confval  bool
		tag      string
		event_id uint
	}{
		{"disabled", false, "", 1000},
		{"enabled", true, "event_id:1000", 1000},
	}

	reporter, err := evtreporter.New(s.eventSource, s.ti.API())
	require.NoError(s.T(), err)
	defer reporter.Close()

	for _, tc := range tests {
		s.Run(tc.name, func() {
			defer resetSender(s.sender)


			instanceConfig := []byte(fmt.Sprintf(`
path: %s
start: now
tag_event_id: %t
`,
				s.channelPath, tc.confval))

			check, err := s.newCheck(instanceConfig, nil)
			require.NoError(s.T(), err)
			defer check.Cancel()

			s.sender.On("Commit").Return().Once()
			s.sender.On("Event", mock.MatchedBy(func(e metrics.Event) bool {
				if tc.confval {
					return assert.Contains(s.T(), e.Tags, tc.tag, "Tags should contain the event id")
				}
				res := true
				for _, tag := range e.Tags {
					res = res && assert.NotContains(s.T(), tag, "event_id", "Tags should not contain the event id")
				}
				return res
			})).Once()

			err = reporter.ReportEvent(windows.EVENTLOG_INFORMATION_TYPE, 0, tc.event_id, []string{"teststring"}, nil)

			check.Run()

			s.sender.AssertExpectations(s.T())
		})
	}
}

func BenchmarkGetEvents(b *testing.B) {
	channelPath := "dd-test-channel-check"
	eventSource := "dd-test-source-check"
	numEvents := []uint{10, 100, 1000}
	batchCounts := []uint{1, 10, 100, 1000}

	testerNames := eventlog_test.GetEnabledAPITesters()

	sender := mocksender.NewMockSender("")

	bench_startTime := time.Now()
	bench_total_events := uint(0)
	for _, tiName := range testerNames {
		for _, v := range numEvents {
			// setup log
			ti := eventlog_test.GetAPITesterByName(tiName, b)
			err := ti.InstallChannel(channelPath)
			require.NoError(b, err)
			err = ti.InstallSource(channelPath, eventSource)
			require.NoError(b, err)
			err = ti.API().EvtClearLog(channelPath)
			require.NoError(b, err)
			err = ti.GenerateEvents(eventSource, v)
			require.NoError(b, err)

			for _, batchCount := range batchCounts {
				if batchCount > v {
					continue
				}
				b.Run(fmt.Sprintf("%vAPI/%dEvents/%dBatch", tiName, v, batchCount), func(b *testing.B) {
					// setup check
					instanceConfig := []byte(fmt.Sprintf(`
path: %s
start: old
payload_size: %d
`,
						channelPath, batchCount))

					// read the log b.N times
					b.ResetTimer()
					startTime := time.Now()
					total_events := uint(0)
					for i := 0; i < b.N; i++ {
						check := new(Check)
						check.evtapi = ti.API()
						err = check.Configure(integration.FakeConfigHash, instanceConfig, nil, "test")
						require.NoError(b, err)
						mocksender.SetSender(sender, check.ID())
						sender.On("Commit").Return()
						senderEventCall := sender.On("Event", mock.Anything)

						total_events += countEvents(check, senderEventCall, v)

						check.Cancel()
						resetSender(sender)
					}

					// TODO: Use b.Elapsed in go1.20
					elapsed := time.Since(startTime)
					b.Logf("%.2f events/s (%.3fs) N=%d", float64(total_events)/elapsed.Seconds(), elapsed.Seconds(), b.N)
					bench_total_events += total_events
				})
			}
		}
	}

	elapsed := time.Since(bench_startTime)
	b.Logf("Benchmark total: %d events %.2f events/s (%.3fs)", bench_total_events, float64(bench_total_events)/elapsed.Seconds(), elapsed.Seconds())
}
