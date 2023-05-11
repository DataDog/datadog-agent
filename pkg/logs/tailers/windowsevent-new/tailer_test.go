// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows
// +build windows

package windowsevent

import (
	"fmt"
	"testing"
	"time"

	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cihub/seelog"

	"github.com/cenkalti/backoff"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/test"
)

type ReadEventsSuite struct {
	suite.Suite

	channelPath string
	eventSource string
	query       string

	testAPI   string
	numEvents uint
	ti        eventlog_test.APITester
}

func TestReadEventsSuite(t *testing.T) {
	testerNames := eventlog_test.GetEnabledAPITesters()

	for _, tiName := range testerNames {
		t.Run(fmt.Sprintf("%sAPI", tiName), func(t *testing.T) {
			var s ReadEventsSuite
			s.channelPath = "dd-test-channel-logtailer"
			s.eventSource = "dd-test-source-logtailer"
			s.query = "*"
			s.numEvents = 100
			s.testAPI = tiName
			suite.Run(t, &s)
		})
	}
}

func (s *ReadEventsSuite) SetupSuite() {
	// Enable logger
	if false {
		pkglog.SetupLogger(seelog.Default, "debug")
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

func newtailer(evtapi evtapi.API, tailerconfig *Config) (*Tailer, error) {
	source := sources.NewLogSource("", &config.LogsConfig{})
	msgChan := make(chan *message.Message)

	tailer := NewTailer(evtapi, source, tailerconfig, msgChan)
	tailer.Start()
	err := backoff.Retry(func() error {
		if source.Status.IsSuccess() {
			return nil
		} else if source.Status.IsError() {
			return fmt.Errorf(source.Status.GetError())
		}
		return fmt.Errorf("start pending")
	}, backoff.NewConstantBackOff(50*time.Millisecond))
	if err != nil {
		return nil, fmt.Errorf("failed to start tailer: %v", err)
	}
	return tailer, nil
}

func (s *ReadEventsSuite) TestReadEvents() {
	config := Config{
		ChannelPath: s.channelPath,
		Query:       s.query,
	}
	tailer, err := newtailer(s.ti.API(), &config)
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		tailer.Stop()
	})

	err = s.ti.GenerateEvents(s.eventSource, s.numEvents)
	s.Require().NoError(err)

	totalEvents := uint(0)
	for i := uint(0); i < s.numEvents; i++ {
		msg := <-tailer.outputChan
		s.Require().NotEmpty(msg.Content, "Message must not be empty")
		totalEvents += 1
	}
	s.Require().Equal(s.numEvents, totalEvents, "Received %d/%d events", totalEvents, s.numEvents)
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
				tailer, err := newtailer(ti.API(), &config)
				require.NoError(b, err)
				b.Cleanup(func() {
					tailer.Stop()
				})

				b.ResetTimer()
				startTime := time.Now()
				totalEvents := uint(0)
				for i := 0; i < b.N; i++ {
					b.StopTimer()
					err = ti.API().EvtClearLog(channelPath)
					require.NoError(b, err)
					err = ti.GenerateEvents(eventSource, numEvents)
					require.NoError(b, err)
					b.StartTimer()

					for i := uint(0); i < numEvents; i++ {
						msg := <-tailer.outputChan
						require.NotEmpty(b, msg.Content, "Message must not be empty")
						totalEvents += 1
					}
				}

				// TODO: Use b.Elapsed in go1.20
				elapsed := time.Since(startTime)
				b.Logf("%.2f events/s (%.3fs)", float64(totalEvents)/elapsed.Seconds(), elapsed.Seconds())

			})
		}
	}
}
