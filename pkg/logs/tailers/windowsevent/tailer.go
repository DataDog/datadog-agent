// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

// Package windowsevent provides Windows event log tailers
package windowsevent

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sys/windows"

	"github.com/cenkalti/backoff/v5"

	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	publishermetadatacache "github.com/DataDog/datadog-agent/comp/publishermetadatacache/def"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/processor"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/util/windowsevent"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	winevtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/windows"
	evtbookmark "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/bookmark"
	evtsubscribe "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/subscription"
)

// Config is a event log tailer configuration
type Config struct {
	ChannelPath string
	Query       string
	// See LogsConfig.ShouldProcessRawMessage() comment.
	ProcessRawMessage bool
}

// registrySaverWithInitialBookmark wraps a registry and returns a provided bookmark XML on Load.
// This allows the tailer to pass the bookmark string parameter to the subscription.
// Save() updates the internal bookmark state so that subsequent Load() calls return the updated value.
type registrySaverWithInitialBookmark struct {
	registry        auditor.Registry
	identifier      string
	currentBookmark string
}

func (s *registrySaverWithInitialBookmark) Save(bookmarkXML string) error {
	s.registry.SetOffset(s.identifier, bookmarkXML)
	// Update internal bookmark so subsequent Load() calls return the updated value
	s.currentBookmark = bookmarkXML
	return nil
}

func (s *registrySaverWithInitialBookmark) Load() (string, error) {
	// Return the current bookmark
	return s.currentBookmark, nil
}

// Tailer collects logs from Windows Event Log using a pull subscription
type Tailer struct {
	evtapi     evtapi.API
	source     *sources.LogSource
	config     *Config
	decoder    decoder.Decoder
	outputChan chan *message.Message

	cancelTail context.CancelFunc
	doneTail   chan struct{}
	done       chan struct{}

	sub                    evtsubscribe.PullSubscription
	bookmark               evtbookmark.Bookmark
	systemRenderContext    evtapi.EventRenderContextHandle
	registry               auditor.Registry
	publisherMetadataCache publishermetadatacache.Component
}

// NewTailer returns a new tailer.
func NewTailer(evtapi evtapi.API, source *sources.LogSource, config *Config, outputChan chan *message.Message, registry auditor.Registry, publisherMetadataCache publishermetadatacache.Component) *Tailer {
	if evtapi == nil {
		evtapi = winevtapi.New()
	}

	if len(source.Config.ProcessingRules) > 0 && config.ProcessRawMessage {
		telemetry.GetStatsTelemetryProvider().Gauge(processor.UnstructuredProcessingMetricName, 1, []string{"tailer:windowsevent"})
	}

	return &Tailer{
		evtapi:                 evtapi,
		source:                 source,
		config:                 config,
		decoder:                decoder.NewNoopDecoder(),
		outputChan:             outputChan,
		registry:               registry,
		publisherMetadataCache: publisherMetadataCache,
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
	t.done = make(chan struct{})
	ctx, ctxCancel := context.WithCancel(context.Background())
	t.cancelTail = ctxCancel
	t.registry.SetTailed(t.Identifier(), true)
	go t.forwardMessages()
	t.decoder.Start()
	go t.tail(ctx, bookmark)
}

// Stop stops the tailer
func (t *Tailer) Stop() {
	log.Info("Stop tailing windows event log")
	t.registry.SetTailed(t.Identifier(), false)
	t.cancelTail()
	<-t.doneTail

	t.decoder.Stop()

	t.sub.Stop()

	<-t.done
}

func (t *Tailer) forwardMessages() {
	defer func() {
		// the decoder has successfully been flushed
		close(t.done)
	}()

	for decodedMessage := range t.decoder.OutputChan() {
		if len(decodedMessage.GetContent()) > 0 {
			// Leverage the existing message instead of creating a new one
			// This preserves all bookmark information and is more efficient
			msg := decodedMessage

			// Update tags to include source config tags
			if msg.Origin != nil {
				// Combine tags from multiple sources: parsing extra tags and source config tags
				tags := append(msg.ParsingExtra.Tags, t.source.Config.Tags...)
				msg.Origin.SetTags(tags)
			}

			t.outputChan <- msg
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

	// Create bookmark saver that returns the provided bookmark XML
	bookmarkSaver := &registrySaverWithInitialBookmark{
		registry:        t.registry,
		identifier:      t.Identifier(),
		currentBookmark: bookmark,
	}
	opts = append(opts, evtsubscribe.WithBookmarkSaver(bookmarkSaver))

	// Always use "now" mode - if bookmark exists it will be loaded via the saver,
	// otherwise FromLatestEvent will create one
	opts = append(opts, evtsubscribe.WithStartMode("now"))

	// Create subscription - bookmark initialization will happen in Start()
	t.sub = evtsubscribe.NewPullSubscription(
		t.config.ChannelPath,
		t.config.Query,
		opts...,
	)

	// Create an initial empty bookmark for updating as events are processed
	// This will be set to the loaded/created bookmark after subscription starts
	t.bookmark, err = evtbookmark.New(evtbookmark.WithWindowsEventLogAPI(t.evtapi))
	if err != nil {
		t.logErrorAndSetStatus(fmt.Errorf("failed to create bookmark: %w", err))
		return
	}

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

func retryForeverWithCancel(ctx context.Context, operation func() error) error {
	resetBackoff := backoff.NewExponentialBackOff()
	resetBackoff.InitialInterval = 1 * time.Second
	resetBackoff.MaxInterval = 1 * time.Minute

	// retry never stops if MaxElapsedTime == 0
	_, err := backoff.Retry(ctx, func() (any, error) {
		return nil, operation()
	}, backoff.WithBackOff(resetBackoff), backoff.WithMaxElapsedTime(0))
	return err
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
	} else {
		log.Warnf("Failed to render bookmark: %v for event %s", err, xml)
	}

	t.source.RecordBytes(int64(len(msg.GetContent())))
	t.decoder.InputChan() <- msg
}

// enrichEvent renders event record fields using EvtFormatMessage and adds them to the map.
func (t *Tailer) enrichEvent(m *windowsevent.Map, event evtapi.EventRecordHandle) error {

	vals, err := t.evtapi.EvtRenderEventValues(t.systemRenderContext, event)
	if err != nil {
		return fmt.Errorf("error rendering event values: %v", err)
	}
	defer vals.Close()

	providerName, err := vals.String(evtapi.EvtSystemProviderName)
	if err != nil {
		return fmt.Errorf("failed to get provider name: %v", err)
	}

	windowsevent.AddRenderedInfoToMap(m, t.publisherMetadataCache, providerName, event)

	return nil
}
