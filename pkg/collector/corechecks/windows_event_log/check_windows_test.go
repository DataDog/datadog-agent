// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package evtlog

import (
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	agentConfig "github.com/DataDog/datadog-agent/pkg/config"
	agentEvent "github.com/DataDog/datadog-agent/pkg/metrics/event"
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

func (s *GetEventsTestSuite) testsetup() {
	// Ensure the log is empty
	err := s.ti.API().EvtClearLog(s.channelPath)
	require.NoError(s.T(), err)

	// Reset the expectations/calls on the mock sender
	resetSender(s.sender)

	// create tmpdir to store bookmark. Necessary to isolate test runs from each other, otherwise
	// they will load bookmarks from previous runs.
	testDir := s.T().TempDir()
	mockConfig := agentConfig.Mock(s.T())
	mockConfig.Set("run_path", testDir)
}

func (s *GetEventsTestSuite) SetupTest() {
	s.testsetup()
}

func (s *GetEventsTestSuite) SetupSubTest() {
	s.testsetup()
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
	err := check.Configure(s.sender.GetSenderManager(), integration.FakeConfigHash, instanceConfig, initConfig, "test")
	if !assert.NoError(s.T(), err) {
		return nil, err
	}
	mocksender.SetSender(s.sender, check.ID())

	return check, nil
}

func TestGetEventsTestSuite(t *testing.T) {
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
			s.numEvents = 5
			suite.Run(t, &s)
		})
	}
}

func countEvents(check *Check, senderEventCall *mock.Call, numEvents uint) uint {
	eventsCollected := uint(0)
	prevEventsCollected := uint(0)
	if numEvents > 0 {
		senderEventCall.Run(func(args mock.Arguments) {
			eventsCollected++
		})
	} else {
		senderEventCall.Unset()
	}
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
start: oldest
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

// Test that the check can detect and recover from a broken subscription
func (s *GetEventsTestSuite) TestRecoverFromBrokenSubscription() {
	// Put events in the log
	err := s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	instanceConfig := []byte(fmt.Sprintf(`
path: %s
start: oldest
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

	// bookmark should have been updated
	// remove the source/channel to break the subscription
	s.ti.RemoveSource(s.channelPath, s.eventSource)
	s.ti.RemoveChannel(s.channelPath)
	cmd := exec.Command("powershell.exe", "-Command", "Restart-Service", "EventLog", "-Force")
	out, err := cmd.CombinedOutput()
	require.NoError(s.T(), err, "Failed to restart EventLog service %s", out)

	// check run should return an error
	err = check.Run()
	require.Error(s.T(), err)

	// check run should fail again, this time to create the subscription
	err = check.Run()
	require.Error(s.T(), err)

	// reinstall source/channel
	err = s.ti.InstallChannel(s.channelPath)
	require.NoError(s.T(), err)
	err = s.ti.InstallSource(s.channelPath, s.eventSource)
	require.NoError(s.T(), err)

	// next check run should recreate subscription and resume from bookmark and read 0 events
	resetSender(s.sender)
	s.sender.On("Commit").Return()
	senderEventCall = s.sender.On("Event", mock.Anything)
	eventsCollected = countEvents(check, senderEventCall, 0)
	require.Equal(s.T(), uint(0), eventsCollected)
	s.sender.AssertExpectations(s.T())

	// put some new events in the log and ensure the check sees them
	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)
	resetSender(s.sender)
	s.sender.On("Commit").Return()
	senderEventCall = s.sender.On("Event", mock.Anything)
	eventsCollected = countEvents(check, senderEventCall, s.numEvents)
	require.Equal(s.T(), s.numEvents, eventsCollected)
	s.sender.AssertExpectations(s.T())
}

// Test that the check can resume from a bookmark
func (s *GetEventsTestSuite) TestBookmark() {
	// Put events in the log
	err := s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)

	// Set bookmark_frequency to be less than s.numEvents so we can test the "end of check" bookmark.
	instanceConfig := []byte(fmt.Sprintf(`
path: %s
start: oldest
bookmark_frequency: %d
`,
		s.channelPath, s.numEvents-1))

	check, err := s.newCheck(instanceConfig, nil)
	require.NoError(s.T(), err)
	defer check.Cancel()

	s.sender.On("Commit").Return()
	senderEventCall := s.sender.On("Event", mock.Anything)

	eventsCollected := countEvents(check, senderEventCall, s.numEvents)
	require.Equal(s.T(), s.numEvents, eventsCollected)
	s.sender.AssertExpectations(s.T())

	// bookmark should have been updated
	// TODO: test?

	// create a new check
	check.Cancel()
	check, err = s.newCheck(instanceConfig, nil)
	require.NoError(s.T(), err)
	defer check.Cancel()

	// new check should resume from bookmark and read 0 events
	resetSender(s.sender)
	s.sender.On("Commit").Return()
	senderEventCall = s.sender.On("Event", mock.Anything)
	eventsCollected = countEvents(check, senderEventCall, 0)
	require.Equal(s.T(), uint(0), eventsCollected)
	s.sender.AssertExpectations(s.T())

	// put some new events in the log and ensure the check sees them
	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	require.NoError(s.T(), err)
	resetSender(s.sender)
	s.sender.On("Commit").Return()
	senderEventCall = s.sender.On("Event", mock.Anything)
	eventsCollected = countEvents(check, senderEventCall, s.numEvents)
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

			alertType, err := agentEvent.GetAlertTypeFromString(tc.alertType)
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
			err = reporter.ReportEvent(tc.reportLevel, 0, 1000, nil, []string{"teststring"}, nil)
			require.NoError(s.T(), err)

			s.sender.On("Commit").Return().Once()
			s.sender.On("Event", mock.MatchedBy(func(e agentEvent.Event) bool {
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

			eventPriority, err := agentEvent.GetEventPriorityFromString(tc.eventPriority)
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
			err = reporter.ReportEvent(windows.EVENTLOG_INFORMATION_TYPE, 0, 1000, nil, []string{"teststring"}, nil)
			require.NoError(s.T(), err)

			s.sender.On("Commit").Return().Once()
			s.sender.On("Event", mock.MatchedBy(func(e agentEvent.Event) bool {
				return e.Priority == eventPriority
			})).Once()

			check.Run()

			s.sender.AssertExpectations(s.T())
		})
	}
}

// Tests that the Event Query configuration value successfully filters event records
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
	s.sender.On("Event", mock.MatchedBy(func(e agentEvent.Event) bool {
		return assert.Contains(s.T(), e.Text, matchstring, "reported events should match EventID=1000")
	})).Once()

	// Generate an event the query should match on (EventID=1000)
	err = reporter.ReportEvent(windows.EVENTLOG_INFORMATION_TYPE, 0, 1000, nil, []string{matchstring}, nil)
	s.Require().NoError(err)
	// Generate an event the query should not match on (EventID!=1000)
	err = reporter.ReportEvent(windows.EVENTLOG_INFORMATION_TYPE, 0, 999, nil, []string{nomatchstring}, nil)
	s.Require().NoError(err)

	check.Run()

	s.sender.AssertExpectations(s.T())
}

// Tests that the Event Query configuration value successfully filters event records
func (s *GetEventsTestSuite) TestGetEventsWithFilters() {
	reporter, err := evtreporter.New(s.eventSource, s.ti.API())
	require.NoError(s.T(), err)
	defer reporter.Close()

	// Query for EventID=1000
	instanceConfig := []byte(fmt.Sprintf(`
path: %s
start: now
filters:
  source:
  - '%s'
  type:
  - information
  id:
  - 1000
`, s.channelPath, s.eventSource))

	check, err := s.newCheck(instanceConfig, nil)
	require.NoError(s.T(), err)
	defer check.Cancel()

	matchstring := "match this string"
	nomatchstring := "should not match"
	s.sender.On("Commit").Return().Once()
	s.sender.On("Event", mock.MatchedBy(func(e agentEvent.Event) bool {
		return assert.Contains(s.T(), e.Text, matchstring, "reported events should match EventID=1000")
	})).Once()

	// Generate an event the query should match on (EventID=1000)
	err = reporter.ReportEvent(windows.EVENTLOG_INFORMATION_TYPE, 0, 1000, nil, []string{matchstring}, nil)
	s.Require().NoError(err)
	// Generate an event the query should not match on (EventID!=1000)
	err = reporter.ReportEvent(windows.EVENTLOG_INFORMATION_TYPE, 0, 999, nil, []string{nomatchstring}, nil)
	s.Require().NoError(err)

	check.Run()

	s.sender.AssertExpectations(s.T())
}

func (s *GetEventsTestSuite) TestCatchInvalidRegex() {
	tcs := []struct {
		name       string
		option     string
		regex      string
		errorMatch string
	}{
		{"InvalidInclude", "included_messages", "'*'", "missing argument to repetition operator"},
		{"InvalidExclude", "excluded_messages", "'*'", "missing argument to repetition operator"},
	}

	for _, tc := range tcs {
		s.Run(tc.name, func() {
			instanceConfig := []byte(fmt.Sprintf(`
path: %s
start: oldest
%s:
 - %s`, s.channelPath, tc.option, tc.regex))

			check := new(Check)
			check.evtapi = s.ti.API()
			err := check.Configure(s.sender.GetSenderManager(), integration.FakeConfigHash, instanceConfig, nil, "test")
			require.ErrorContains(s.T(), err, tc.errorMatch)
		})
	}
}

// Tests that only events that match included_messages are reported
func (s *GetEventsTestSuite) TestGetEventsWithIncludeMessage() {
	reporter, err := evtreporter.New(s.eventSource, s.ti.API())
	require.NoError(s.T(), err)
	defer reporter.Close()

	instanceConfig := []byte(fmt.Sprintf(`
path: %s
start: now
included_messages:
  - match this string
`, s.channelPath))

	check, err := s.newCheck(instanceConfig, nil)
	require.NoError(s.T(), err)
	defer check.Cancel()

	matchstring := "match this string"
	nomatchstring := "should not match"
	s.sender.On("Commit").Return().Once()
	s.sender.On("Event", mock.MatchedBy(func(e agentEvent.Event) bool {
		return assert.Contains(s.T(), e.Text, matchstring, "should only report match string")
	})).Once()

	// Generate an event that matches included_messages
	err = reporter.ReportEvent(windows.EVENTLOG_INFORMATION_TYPE, 0, 1000, nil, []string{matchstring}, nil)
	s.Require().NoError(err)
	// Geneate an event that does not match included_messages
	err = reporter.ReportEvent(windows.EVENTLOG_INFORMATION_TYPE, 0, 999, nil, []string{nomatchstring}, nil)
	s.Require().NoError(err)

	check.Run()

	s.sender.AssertExpectations(s.T())
}

// Tests that events that match excluded_messages are not reported
func (s *GetEventsTestSuite) TestGetEventsWithExcludeMessage() {
	reporter, err := evtreporter.New(s.eventSource, s.ti.API())
	require.NoError(s.T(), err)
	defer reporter.Close()

	instanceConfig := []byte(fmt.Sprintf(`
path: %s
start: now
excluded_messages:
  - match this string
`, s.channelPath))

	check, err := s.newCheck(instanceConfig, nil)
	require.NoError(s.T(), err)
	defer check.Cancel()

	matchstring := "match this string"
	nomatchstring := "should not match"
	s.sender.On("Commit").Return().Once()
	s.sender.On("Event", mock.MatchedBy(func(e agentEvent.Event) bool {
		return assert.NotContains(s.T(), e.Text, matchstring, "should not report match string")
	})).Once()

	// Generate an event that matches excluded_messages
	err = reporter.ReportEvent(windows.EVENTLOG_INFORMATION_TYPE, 0, 1000, nil, []string{matchstring}, nil)
	s.Require().NoError(err)
	// Geneate an event that does not match excluded_messages
	err = reporter.ReportEvent(windows.EVENTLOG_INFORMATION_TYPE, 0, 999, nil, []string{nomatchstring}, nil)
	s.Require().NoError(err)

	check.Run()

	s.sender.AssertExpectations(s.T())
}

// Tests that events that match excluded_messages are not reported even if they also match included_messages
func (s *GetEventsTestSuite) TestGetEventsWithExcludeMessagePrecedence() {
	reporter, err := evtreporter.New(s.eventSource, s.ti.API())
	require.NoError(s.T(), err)
	defer reporter.Close()

	instanceConfig := []byte(fmt.Sprintf(`
path: %s
start: now
included_messages:
  - match this string
excluded_messages:
  - match this string
`, s.channelPath))

	check, err := s.newCheck(instanceConfig, nil)
	require.NoError(s.T(), err)
	defer check.Cancel()

	matchstring := "match this string"
	nomatchstring := "should not match"
	s.sender.On("Commit").Return().Once()
	// no events should be reported

	// Generate an event that matches [in|ex]cluded_messages
	err = reporter.ReportEvent(windows.EVENTLOG_INFORMATION_TYPE, 0, 1000, nil, []string{matchstring}, nil)
	s.Require().NoError(err)
	// Geneate an event that does not match [in|ex]cluded_messages
	err = reporter.ReportEvent(windows.EVENTLOG_INFORMATION_TYPE, 0, 999, nil, []string{nomatchstring}, nil)
	s.Require().NoError(err)

	check.Run()

	s.sender.AssertExpectations(s.T())
}

// Tests that the tag_event_id configuration option results in an event_id tag
func (s *GetEventsTestSuite) TestGetEventsWithTagEventID() {
	tests := []struct {
		name    string
		confval bool
		tag     string
		eventID uint
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
			s.sender.On("Event", mock.MatchedBy(func(e agentEvent.Event) bool {
				if tc.confval {
					return assert.Contains(s.T(), e.Tags, tc.tag, "Tags should contain the event id")
				}
				res := true
				for _, tag := range e.Tags {
					res = res && assert.NotContains(s.T(), tag, "event_id:", "Tags should not contain the event id")
				}
				return res
			})).Once()

			err = reporter.ReportEvent(windows.EVENTLOG_INFORMATION_TYPE, 0, tc.eventID, nil, []string{"teststring"}, nil)
			s.Require().NoError(err)

			check.Run()

			s.sender.AssertExpectations(s.T())
		})
	}
}

// Tests that the tag_sid configuration option results in a sid tag
func (s *GetEventsTestSuite) TestGetEventsWithTagSID() {

	// Use LocalSystem for the SID
	reportsid, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	require.NoError(s.T(), err)
	account, domain, _, err := reportsid.LookupAccount("")
	require.NoError(s.T(), err)

	tests := []struct {
		name    string
		confval bool
		sid     *windows.SID
		tag     string
	}{
		{"disabled", false, reportsid, ""},
		{"enabled", true, reportsid, fmt.Sprintf("sid:%s\\%s", domain, account)},
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
tag_sid: %t
`,
				s.channelPath, tc.confval))

			check, err := s.newCheck(instanceConfig, nil)
			require.NoError(s.T(), err)
			defer check.Cancel()

			s.sender.On("Commit").Return().Once()
			s.sender.On("Event", mock.MatchedBy(func(e agentEvent.Event) bool {
				if tc.confval {
					return assert.Contains(s.T(), e.Tags, tc.tag, "Tags should contain the sid/username")
				}
				res := true
				for _, tag := range e.Tags {
					res = res && assert.NotContains(s.T(), tag, "sid:", "Tags should not contain the sid")
				}
				return res
			})).Once()

			err = reporter.ReportEvent(windows.EVENTLOG_INFORMATION_TYPE, 0, 1000, tc.sid, []string{"teststring"}, nil)
			s.Require().NoError(err)
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

	benchmarkStartTime := time.Now()
	benchmarkTotalEvents := uint(0)
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
					mockConfig := agentConfig.Mock(b)

					// setup check
					instanceConfig := []byte(fmt.Sprintf(`
path: %s
start: oldest
payload_size: %d
`,
						channelPath, batchCount))

					// read the log b.N times
					b.ResetTimer()
					startTime := time.Now()
					totalEvents := uint(0)
					for i := 0; i < b.N; i++ {
						// create tmpdir to store bookmark
						testDir := b.TempDir()
						mockConfig.Set("run_path", testDir)
						// create check
						check := new(Check)
						check.evtapi = ti.API()
						err = check.Configure(sender.GetSenderManager(), integration.FakeConfigHash, instanceConfig, nil, "test")
						require.NoError(b, err)
						mocksender.SetSender(sender, check.ID())
						sender.On("Commit").Return()
						senderEventCall := sender.On("Event", mock.Anything)
						// read all the events
						totalEvents += countEvents(check, senderEventCall, v)
						// clean shutdown the check and reset the mock sender expecations
						check.Cancel()
						resetSender(sender)
					}

					// TODO: Use b.Elapsed in go1.20
					elapsed := time.Since(startTime)
					b.Logf("%.2f events/s (%.3fs) N=%d", float64(totalEvents)/elapsed.Seconds(), elapsed.Seconds(), b.N)
					benchmarkTotalEvents += totalEvents
				})
			}
		}
	}

	elapsed := time.Since(benchmarkStartTime)
	b.Logf("Benchmark total: %d events %.2f events/s (%.3fs)", benchmarkTotalEvents, float64(benchmarkTotalEvents)/elapsed.Seconds(), elapsed.Seconds())
}
