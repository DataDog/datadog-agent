// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

//nolint:revive // TODO(WINA) Fix revive linter
package windowsevent

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sys/windows"

	"github.com/cenkalti/backoff"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/framer"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/noop"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/processor"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	"github.com/DataDog/datadog-agent/pkg/logs/util/windowsevent"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/windows"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/bookmark"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/subscription"
)

// Config is a event log tailer configuration
type Config struct {
	ChannelPath string
	Query       string
	// See LogsConfig.ShouldProcessRawMessage() comment.
	ProcessRawMessage bool
}

// Tailer collects logs from Windows Event Log using a pull subscription
type Tailer struct {
	evtapi     evtapi.API
	source     *sources.LogSource
	config     *Config
	decoder    *decoder.Decoder
	outputChan chan *message.Message

	cancelTail context.CancelFunc
	doneTail   chan struct{}

	sub                 evtsubscribe.PullSubscription
	bookmark            evtbookmark.Bookmark
	systemRenderContext evtapi.EventRenderContextHandle
}

// NewTailer returns a new tailer.
func NewTailer(evtapi evtapi.API, source *sources.LogSource, config *Config, outputChan chan *message.Message) *Tailer {
	if evtapi == nil {
		evtapi = winevtapi.New()
	}

	if len(source.Config.ProcessingRules) > 0 && config.ProcessRawMessage {
		telemetry.GetStatsTelemetryProvider().Gauge(processor.UnstructuredProcessingMetricName, 1, []string{"tailer:windowsevent"})
	}

	return &Tailer{
		evtapi:     evtapi,
		source:     source,
		config:     config,
		decoder:    decoder.NewDecoderWithFraming(sources.NewReplaceableSource(source), noop.New(), framer.NoFraming, nil, status.NewInfoRegistry()),
		outputChan: outputChan,
	}
}

// Identifier returns a string that uniquely identifies a source
func Identifier(channelPath, query string) string {
	return fmt.Sprintf("eventlog:%s;%s", channelPath, query)
}

// Identifier returns a string that uniquely identifies a source
func (t *Tailer) Identifier() string {
	return Identifier(t.config.ChannelPath, t.config.Query)
}

func (t *Tailer) toMessage(m *windowsevent.Map) (*message.Message, error) {
	return windowsevent.MapToMessage(m, t.source, t.config.ProcessRawMessage)
}

// Start starts tailing the event log.
func (t *Tailer) Start(bookmark string) {
	log.Infof("Starting windows event log tailing for channel %s query %s", t.config.ChannelPath, t.config.Query)
	t.doneTail = make(chan struct{})
	ctx, ctxCancel := context.WithCancel(context.Background())
	t.cancelTail = ctxCancel
	go t.forwardMessages()
	t.decoder.Start()
	go t.tail(ctx, bookmark)
}

// Stop stops the tailer
func (t *Tailer) Stop() {
	log.Info("Stop tailing windows event log")
	t.cancelTail()
	<-t.doneTail

	t.decoder.Stop()

	t.sub.Stop()
}

func (t *Tailer) forwardMessages() {
	for decodedMessage := range t.decoder.OutputChan {
		if len(decodedMessage.GetContent()) > 0 {
			t.outputChan <- decodedMessage
		}
	}
}

func (t *Tailer) logErrorAndSetStatus(err error) {
	log.Errorf("%v", err)
	t.source.Status.Error(err)
}

// tail subscribes to the channel for the windows events
func (t *Tailer) tail(ctx context.Context, bookmark string) {
	defer close(t.doneTail)

	var err error

	opts := []evtsubscribe.PullSubscriptionOption{
		evtsubscribe.WithWindowsEventLogAPI(t.evtapi),
		evtsubscribe.WithEventBatchCount(10),
	}

	t.bookmark = nil
	if bookmark != "" {
		// load bookmark
		t.bookmark, err = evtbookmark.New(
			evtbookmark.WithWindowsEventLogAPI(t.evtapi),
			evtbookmark.FromXML(bookmark))
		if err != nil {
			log.Errorf("error loading bookmark, tailer will start at new events: %v", err)
			t.bookmark = nil
		} else {
			opts = append(opts, evtsubscribe.WithStartAfterBookmark(t.bookmark))
		}
	}
	if t.bookmark == nil {
		// new bookmark
		t.bookmark, err = evtbookmark.New(
			evtbookmark.WithWindowsEventLogAPI(t.evtapi))
		if err != nil {
			t.logErrorAndSetStatus(fmt.Errorf("error creating new bookmark: %w", err))
			return
		}
	}

	// subscription
	t.sub = evtsubscribe.NewPullSubscription(
		t.config.ChannelPath,
		t.config.Query,
		opts...,
	)
	// subscription will be started in the eventLoop

	// render context for system values
	t.systemRenderContext, err = t.evtapi.EvtCreateRenderContext(nil, evtapi.EvtRenderContextSystem)
	if err != nil {
		t.logErrorAndSetStatus(fmt.Errorf("failed to create system render context: %w", err))
		return
	}
	defer evtapi.EvtCloseRenderContext(t.evtapi, t.systemRenderContext)

	// wait for stop signal
	t.eventLoop(ctx)
}

func retryForeverWithCancel(ctx context.Context, operation backoff.Operation) error {
	resetBackoff := backoff.NewExponentialBackOff()
	resetBackoff.InitialInterval = 1 * time.Second
	resetBackoff.MaxInterval = 1 * time.Minute
	// retry never stops if MaxElapsedTime == 0
	resetBackoff.MaxElapsedTime = 0

	return backoff.Retry(operation, backoff.WithContext(resetBackoff, ctx))
}

func (t *Tailer) eventLoop(ctx context.Context) {
	for {
		// check if loop should exit
		select {
		case <-ctx.Done():
			return
		default:
		}

		// if subscription is not running, try to start it with an exponential backoff
		if !t.sub.Running() {
			err := retryForeverWithCancel(ctx, func() error {
				err := t.sub.Start()
				if err != nil {
					t.logErrorAndSetStatus(fmt.Errorf("failed to start subscription: %w", err))
					return err
				}
				// subscription started!
				return nil
			})
			if err != nil {
				// subscription failed to start, retry returned, probably because
				// ctx was cancelled. go back to top of loop to check for cancellation
				// and exit or continue looping as appropriate.
				continue
			}
			// subscription started!
			t.source.Status.Success()
		}

		// subscription is running, wait for and get events
		select {
		case <-ctx.Done():
			return
		case events, ok := <-t.sub.GetEvents():
			if !ok {
				// events channel is closed, fetch the error and stop the subscription so we may retry
				err := t.sub.Error()
				t.logErrorAndSetStatus(fmt.Errorf("GetEvents failed, stopping subscription: %w", err))
				t.sub.Stop()
				break
			}
			for _, eventRecord := range events {
				t.handleEvent(eventRecord.EventRecordHandle)
				evtapi.EvtCloseRecord(t.evtapi, eventRecord.EventRecordHandle)
			}
		}
	}
}

func (t *Tailer) handleEvent(eventRecordHandle evtapi.EventRecordHandle) {

	xmlData, err := t.evtapi.EvtRenderEventXml(eventRecordHandle)
	if err != nil {
		log.Warnf("Error rendering xml: %v", err)
		return
	}
	xml := windows.UTF16ToString(xmlData)

	m, err := windowsevent.NewMapXML([]byte(xml))
	if err != nil {
		log.Warnf("Error creating map from xml: %v", err)
		return
	}

	err = t.enrichEvent(m, eventRecordHandle)
	if err != nil {
		log.Warnf("%v", err)
		// continue to submit the event even if we failed to enrich it
	}

	err = t.bookmark.Update(eventRecordHandle)
	if err != nil {
		log.Warnf("Failed to update bookmark: %v, to event %s", err, xml)
	}

	msg, err := t.toMessage(m)
	if err != nil {
		log.Warnf("Failed to convert xml to json: %v for event %s", err, xml)
		return
	}

	// Store bookmark in origin offset so that it is persisted to disk by the auditor registry
	offset, err := t.bookmark.Render()
	if err == nil {
		msg.Origin.Identifier = t.Identifier()
		msg.Origin.Offset = offset
		t.sub.SetBookmark(t.bookmark)
	} else {
		log.Warnf("Failed to render bookmark: %v for event %s", err, xml)
	}

	t.source.RecordBytes(int64(len(msg.GetContent())))
	t.decoder.InputChan <- msg
}

// enrichEvent renders event record fields using EvtFormatMessage and adds them to the map.
func (t *Tailer) enrichEvent(m *windowsevent.Map, event evtapi.EventRecordHandle) error {

	vals, err := t.evtapi.EvtRenderEventValues(t.systemRenderContext, event)
	if err != nil {
		return fmt.Errorf("Error rendering event values: %v", err)
	}
	defer vals.Close()

	providerName, err := vals.String(evtapi.EvtSystemProviderName)
	if err != nil {
		return fmt.Errorf("Failed to get provider name: %v", err)
	}

	pm, err := t.evtapi.EvtOpenPublisherMetadata(providerName, "")
	if err != nil {
		return fmt.Errorf("Failed to get publisher metadata for provider '%s': %v", providerName, err)
	}
	defer evtapi.EvtClosePublisherMetadata(t.evtapi, pm)

	windowsevent.AddRenderedInfoToMap(m, t.evtapi, pm, event)

	return nil
}
