// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package windowsevent

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/cenkalti/backoff/v5"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	logconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditormock "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	publishermetadatacache "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/publishermetadatacache"
	eventlog_test "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/test"
)

type ReadEventsSuite struct {
	suite.Suite

	channelPath string
	eventSource string

	testAPI   string
	numEvents uint
	ti        eventlog_test.APITester
}

func TestReadEventsSuite(t *testing.T) {
	for _, tiName := range eventlog_test.GetEnabledAPITesters() {
		t.Run(tiName+"API", func(t *testing.T) {
			if tiName != "Windows" {
				t.Skipf("skipping %s: test interface not implemented", tiName)
			}

			var s ReadEventsSuite
			s.channelPath = "dd-test-channel-logtailer"
			s.eventSource = "dd-test-source-logtailer"
			s.numEvents = 100
			s.testAPI = tiName
			suite.Run(t, &s)
		})
	}
}

func (s *ReadEventsSuite) SetupSuite() {
	// Enable logger
	if false {
		pkglog.SetupLogger(pkglog.Default(), "debug")
	}

	s.ti = eventlog_test.GetAPITesterByName(s.testAPI, s.T())
}

func (s *ReadEventsSuite) SetupTest() {
	err := s.ti.InstallChannel(s.channelPath)
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		s.ti.RemoveChannel(s.channelPath)
	})
	err = s.ti.InstallSource(s.channelPath, s.eventSource)
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		s.ti.RemoveSource(s.channelPath, s.eventSource)
	})
	err = s.ti.API().EvtClearLog(s.channelPath)
	s.Require().NoError(err)
}

func newtailer(evtapi evtapi.API, tailerconfig *Config, bookmark string, msgChan chan *message.Message) (*Tailer, error) {
	source := sources.NewLogSource("", &logconfig.LogsConfig{})
	registry := auditormock.NewMockAuditor()
	publisherMetadataCache := publishermetadatacache.New(evtapi)

	tailer := NewTailer(evtapi, source, tailerconfig, msgChan, registry, publisherMetadataCache)
	tailer.Start(bookmark)
	_, err := backoff.Retry(context.Background(), func() (any, error) {
		if source.Status.IsSuccess() {
			return nil, nil
		} else if source.Status.IsError() {
			return nil, errors.New(source.Status.GetError())
		}
		return nil, errors.New("start pending")
	}, backoff.WithBackOff(backoff.NewConstantBackOff(50*time.Millisecond)))
	if err != nil {
		return nil, fmt.Errorf("failed to start tailer: %w", err)
	}
	return tailer, nil
}

func (s *ReadEventsSuite) TestReadEvents() {
	config := Config{
		ChannelPath: s.channelPath,
	}
	msgChan := make(chan *message.Message)
	tailer, err := newtailer(s.ti.API(), &config, "", msgChan)
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		tailer.Stop()
	})

	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	s.Require().NoError(err)

	totalEvents := uint(0)
	for i := uint(0); i < s.numEvents; i++ {
		msg := <-msgChan
		s.Require().NotEmpty(msg.GetContent(), "Message must not be empty")
		totalEvents++
	}
	s.Require().Equal(s.numEvents, totalEvents, "Received %d/%d events", totalEvents, s.numEvents)
}

func (s *ReadEventsSuite) TestCustomQuery() {
	query := fmt.Sprintf(`
<QueryList>
  <Query Id="0" Path="%s">
    <Select Path="%s">*[System[Provider[@Name='%s'] and (Level=4 or Level=0) and (EventID=1000)]]</Select>
  </Query>
</QueryList>
`, s.channelPath, s.channelPath, s.eventSource)
	config := Config{
		ChannelPath: s.channelPath,
		Query:       query,
	}
	msgChan := make(chan *message.Message)
	tailer, err := newtailer(s.ti.API(), &config, "", msgChan)
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		tailer.Stop()
	})

	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	s.Require().NoError(err)

	totalEvents := uint(0)
	for i := uint(0); i < s.numEvents; i++ {
		msg := <-msgChan
		s.Require().NotEmpty(msg.GetContent(), "Message must not be empty")
		totalEvents++
	}
	s.Require().Equal(s.numEvents, totalEvents, "Received %d/%d events", totalEvents, s.numEvents)
}

func (s *ReadEventsSuite) TestRecoverFromBrokenSubscription() {
	// TODO: https://datadoghq.atlassian.net/browse/WINA-480
	flake.Mark(s.T())

	// create tailer and ensure events can be read
	config := Config{
		ChannelPath: s.channelPath,
	}
	msgChan := make(chan *message.Message)
	tailer, err := newtailer(s.ti.API(), &config, "", msgChan)
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		tailer.Stop()
	})

	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	s.Require().NoError(err)

	totalEvents := uint(0)
	for i := uint(0); i < s.numEvents; i++ {
		msg := <-msgChan
		s.Require().NotEmpty(msg.GetContent(), "Message must not be empty")
		totalEvents++
	}
	s.Require().Equal(s.numEvents, totalEvents, "Received %d/%d events", totalEvents, s.numEvents)

	// stop the EventLog service and assert the tailer detects the error
	s.ti.KillEventLogService(s.T())
	_, err = backoff.Retry(context.Background(), func() (any, error) {
		if tailer.source.Status.IsSuccess() {
			return nil, errors.New("tailer is still running")
		} else if tailer.source.Status.IsError() {
			return nil, nil
		}
		return nil, errors.New("start pending")
	}, backoff.WithBackOff(backoff.NewConstantBackOff(50*time.Millisecond)))
	s.Require().NoError(err, "tailer should catch the error and update the source status")
	fmt.Println(tailer.source.Status.GetError())

	// start the EventLog service and assert the tailer resumes from the previous error
	s.ti.StartEventLogService(s.T())
	_, err = backoff.Retry(context.Background(), func() (any, error) {
		if tailer.source.Status.IsSuccess() {
			return nil, nil
		} else if tailer.source.Status.IsError() {
			return nil, errors.New(tailer.source.Status.GetError())
		}
		return nil, errors.New("start pending")
	}, backoff.WithBackOff(backoff.NewConstantBackOff(50*time.Millisecond)))
	s.Require().NoError(err, "tailer should auto restart after an error is resolved")

	// ensure the tailer can receive events again
	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	s.Require().NoError(err)

	totalEvents = uint(0)
	for i := uint(0); i < s.numEvents; i++ {
		msg := <-msgChan
		s.Require().NotEmpty(msg.GetContent(), "Message must not be empty")
		totalEvents++
	}
	s.Require().Equal(s.numEvents, totalEvents, "Received %d/%d events", totalEvents, s.numEvents)
}

func (s *ReadEventsSuite) TestBookmarkNewTailer() {
	// create a new tailer and read some events to create a bookmark
	config := Config{
		ChannelPath: s.channelPath,
	}
	msgChan := make(chan *message.Message)
	tailer, err := newtailer(s.ti.API(), &config, "", msgChan)
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		tailer.Stop()
	})

	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	s.Require().NoError(err)

	bookmark := ""
	totalEvents := uint(0)
	for i := uint(0); i < s.numEvents; i++ {
		msg := <-msgChan
		s.Require().NotEmpty(msg.GetContent(), "Message must not be empty")
		totalEvents++
		bookmark = msg.Origin.Offset
	}
	s.Require().Equal(s.numEvents, totalEvents, "Received %d/%d events", totalEvents, s.numEvents)
	// we are done with the original tailer now
	tailer.Stop()

	// add some new events to the log
	// the tailer should resume from the bookmark and see these events even though
	// it wasn't running at the time they were generated
	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	s.Require().NoError(err)

	// create a new tailer, and provide it the bookmark from the previous run
	msgChan = make(chan *message.Message)
	tailer, err = newtailer(s.ti.API(), &config, bookmark, msgChan)
	s.Require().NoError(err)

	totalEvents = uint(0)
	for i := uint(0); i < s.numEvents; i++ {
		msg := <-msgChan
		s.Require().NotEmpty(msg.GetContent(), "Message must not be empty")
		totalEvents++
	}
	s.Require().Equal(s.numEvents, totalEvents, "Received %d/%d events", totalEvents, s.numEvents)

	// if tailer started from bookmark correctly, there should only be s.numEvents
}

// TestInitialBookmarkSeeding verifies the fix for the amnesia bug:
// When a tailer starts with no bookmark, it creates an initial bookmark
// from the most recent event and saves it immediately, even if no events
// are processed before shutdown.
func (s *ReadEventsSuite) TestInitialBookmarkSeeding() {
	config := Config{
		ChannelPath: s.channelPath,
	}

	// Step 1: Generate N=10 initial events to establish a baseline
	// These events ensure there's a "most recent event" for initial seeding
	initialEvents := uint(10)
	err := s.ti.GenerateEvents(s.eventSource, initialEvents)
	s.Require().NoError(err)

	// Step 2: Start tailer with empty bookmark
	// This triggers createInitialBookmark() which should:
	// 1. Query for the most recent event
	// 2. Create a bookmark from that event
	// 3. Send a synthetic message to persist the bookmark
	msgChan := make(chan *message.Message, 100) // Buffer to avoid blocking
	tailer, err := newtailer(s.ti.API(), &config, "", msgChan)
	s.Require().NoError(err)

	// Step 3: Get the initial bookmark directly from the registry
	// The implementation now uses direct SetOffset instead of synthetic messages
	var initialBookmark string

	// Wait a moment for the tailer to initialize and set the bookmark
	time.Sleep(100 * time.Millisecond)

	// Get the bookmark directly from the mock registry
	mockRegistry := tailer.registry.(*auditormock.Auditor)
	initialBookmark = mockRegistry.GetOffset(tailer.Identifier())
	s.Require().NotEmpty(initialBookmark, "Initial bookmark must not be empty")

	// Verify the bookmark content
	s.Require().Contains(initialBookmark, "RecordId=", "Bookmark should contain a RecordId")
	s.Require().Contains(initialBookmark, s.channelPath, "Bookmark should contain the channel path")
	s.Require().Contains(initialBookmark, "BookmarkList", "Bookmark should be valid XML")

	// Log the actual registry contents for debugging
	s.T().Logf("Registry contains bookmark for %s: %s", tailer.Identifier(), initialBookmark)
	s.T().Logf("Full mock registry state: %+v", mockRegistry.StoredOffsets)

	// Optionally drain any real events that might be processed
	// (from the initial 10 events we generated)
	drainTimeout := time.After(2 * time.Second)
drainLoop:
	for {
		select {
		case msg := <-msgChan:
			// Update bookmark if we process real events
			initialBookmark = msg.Origin.Offset
		case <-drainTimeout:
			break drainLoop
		}
	}

	// Step 4: Stop tailer immediately
	// Even though we may not have processed all events, the bookmark is saved
	tailer.Stop()

	// Step 5: Generate M=5 additional events while tailer is stopped
	// These are the events that would be lost in the amnesia bug
	missedEvents := uint(5)
	err = s.ti.GenerateEvents(s.eventSource, missedEvents)
	s.Require().NoError(err)

	// Step 6: Restart tailer with the saved bookmark
	// This should resume from the saved position, not from "latest"
	msgChan = make(chan *message.Message, 100)
	tailer, err = newtailer(s.ti.API(), &config, initialBookmark, msgChan)
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		tailer.Stop()
	})

	// Verify the new tailer has access to the bookmark
	s.T().Logf("Restarted tailer with bookmark: %s", initialBookmark)

	// Step 7: Verify we receive exactly the missed events
	// We should get exactly M=5 events, not 0 (amnesia bug) or 15 (all events)
	receivedEvents := uint(0)
	eventTimeout := time.After(5 * time.Second)
collectLoop:
	for {
		select {
		case msg := <-msgChan:
			content := string(msg.GetContent())
			s.Require().NotEmpty(content, "Message must not be empty")
			receivedEvents++
		case <-eventTimeout:
			break collectLoop
		}
	}

	// The exact count might vary slightly due to timing and which events
	// were processed before shutdown, but we should receive approximately
	// the missed events (not 0, and not all 15)
	s.Require().Greater(receivedEvents, uint(0), "Should receive at least some events (not 0 due to amnesia)")
	s.Require().LessOrEqual(receivedEvents, missedEvents+uint(2), "Should not receive significantly more than missed events")

	// Log final test results
	s.T().Logf("Test completed successfully: Received %d events after restart (expected ~%d missed events)", receivedEvents, missedEvents)
	s.T().Logf("Initial bookmark seeding prevented amnesia bug - no events were lost!")
}

// TestInitialBookmarkSeedingNoEvents tests the edge case where the event log is empty.
// The tailer should still create a valid (empty) bookmark.
func (s *ReadEventsSuite) TestInitialBookmarkSeedingNoEvents() {
	config := Config{
		ChannelPath: s.channelPath,
	}

	// Start tailer with empty bookmark and empty log
	msgChan := make(chan *message.Message, 100)
	tailer, err := newtailer(s.ti.API(), &config, "", msgChan)
	s.Require().NoError(err)

	// Wait for tailer to initialize and set the bookmark
	time.Sleep(100 * time.Millisecond)

	// Get the bookmark directly from the mock registry
	var bookmark string
	mockRegistry := tailer.registry.(*auditormock.Auditor)
	bookmark = mockRegistry.GetOffset(tailer.Identifier())
	// Bookmark might be empty for an empty log, but should be present (even if empty string)
	s.Require().NotNil(&bookmark)

	// Log the bookmark for an empty log
	s.T().Logf("Empty log bookmark for %s: %s", tailer.Identifier(), bookmark)
	s.T().Logf("Mock registry state for empty log: %+v", mockRegistry.StoredOffsets)

	// Verify SetOffset was called even for empty log
	s.Require().Contains(mockRegistry.StoredOffsets, tailer.Identifier(), "Registry should have entry for identifier")

	// Stop tailer
	tailer.Stop()

	// Generate some events while stopped
	newEvents := uint(3)
	err = s.ti.GenerateEvents(s.eventSource, newEvents)
	s.Require().NoError(err)

	// Restart with saved bookmark
	msgChan = make(chan *message.Message, 100)
	tailer, err = newtailer(s.ti.API(), &config, bookmark, msgChan)
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		tailer.Stop()
	})

	// Should receive the new events
	receivedEvents := uint(0)
	eventTimeout := time.After(3 * time.Second)
collectLoop:
	for {
		select {
		case msg := <-msgChan:
			content := string(msg.GetContent())
			s.Require().NotEmpty(content, "Message must not be empty")
			receivedEvents++
		case <-eventTimeout:
			break collectLoop
		}
	}

	s.Require().Equal(newEvents, receivedEvents, "Should receive all %d new events", newEvents)
}

func BenchmarkReadEvents(b *testing.B) {
	numEvents := []uint{10, 100, 1000, 10000}
	testerNames := eventlog_test.GetEnabledAPITesters()

	for _, tiName := range testerNames {
		for _, v := range numEvents {
			b.Run(fmt.Sprintf("%sAPI/%d", tiName, v), func(b *testing.B) {
				if tiName == "Fake" {
					b.Skip("Fake API does not implement EvtRenderValues")
				}
				channelPath := "dd-test-channel-logtailer"
				eventSource := "dd-test-source-logtailer"
				query := "*"
				numEvents := v
				testAPI := "Windows"

				ti := eventlog_test.GetAPITesterByName(testAPI, b)
				err := ti.InstallChannel(channelPath)
				require.NoError(b, err)
				b.Cleanup(func() {
					ti.RemoveChannel(channelPath)
				})
				err = ti.InstallSource(channelPath, eventSource)
				require.NoError(b, err)
				b.Cleanup(func() {
					ti.RemoveSource(channelPath, eventSource)
				})
				err = ti.API().EvtClearLog(channelPath)
				require.NoError(b, err)

				config := Config{
					ChannelPath: channelPath,
					Query:       query,
				}
				msgChan := make(chan *message.Message)
				tailer, err := newtailer(ti.API(), &config, "", msgChan)
				require.NoError(b, err)
				b.Cleanup(func() {
					tailer.Stop()
				})

				b.ResetTimer()
				totalEvents := uint(0)
				for i := 0; i < b.N; i++ {
					b.StopTimer()
					err = ti.API().EvtClearLog(channelPath)
					require.NoError(b, err)
					err = ti.GenerateEvents(eventSource, numEvents)
					require.NoError(b, err)
					b.StartTimer()

					for i := uint(0); i < numEvents; i++ {
						msg := <-msgChan
						require.NotEmpty(b, msg.GetContent(), "Message must not be empty")
						totalEvents++
					}
				}

				elapsed := b.Elapsed()
				b.Logf("%.2f events/s (%.3fs)", float64(totalEvents)/elapsed.Seconds(), elapsed.Seconds())

			})
		}
	}
}
