// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows
// +build windows

package windowsevent

import (
	"errors"
	"fmt"
	"time"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/windows"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/subscription"
	"github.com/cenkalti/backoff"
)

const (
	binaryPath  = "Event.EventData.Binary"
	dataPath    = "Event.EventData.Data"
	taskPath    = "Event.System.Task"
	opcode      = "Event.System.Opcode"
	eventIDPath = "Event.System.EventID"
	// Custom path, not a Microsoft path
	eventIDQualifierPath = "Event.System.EventIDQualifier"
	maxRunes             = 1<<17 - 1 // 128 kB
	truncatedFlag        = "...TRUNCATED..."
)

// Config is a event log tailer configuration
type Config struct {
	ChannelPath string
	Query       string
}

// richEvent carries rendered information to create a richer log
type richEvent struct {
	xmlEvent string
	message  string
	task     string
	opcode   string
	level    string
}

// Tailer collects logs from Windows Event Log using a pull subscription
type Tailer struct {
	evtapi     evtapi.API
	source     *sources.LogSource
	config     *Config
	outputChan chan *message.Message
	stop       chan struct{}
	done       chan struct{}

	sub                 evtsubscribe.PullSubscription
	systemRenderContext evtapi.EventRenderContextHandle
}

// NewTailer returns a new tailer.
func NewTailer(evtapi evtapi.API, source *sources.LogSource, config *Config, outputChan chan *message.Message) *Tailer {
	return &Tailer{
		evtapi:     evtapi,
		source:     source,
		config:     config,
		outputChan: outputChan,
		stop:       make(chan struct{}, 1),
		done:       make(chan struct{}, 1),
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

func (t *Tailer) toMessage(re *richEvent) (*message.Message, error) {
	jsonEvent, err := eventToJSON(re)
	if err != nil {
		return &message.Message{}, err
	}
	return message.NewMessageWithSource([]byte(jsonEvent), message.StatusInfo, t.source, time.Now().UnixNano()), nil
}

// Start starts tailing the event log.
func (t *Tailer) Start() {
	log.Infof("Starting windows event log tailing for channel %s query %s", t.config.ChannelPath, t.config.Query)
	t.stop = make(chan struct{})
	t.done = make(chan struct{})
	go t.tail()
}

// Stop stops the tailer
func (t *Tailer) Stop() {
	log.Info("Stop tailing windows event log")
	close(t.stop)
	<-t.done

	t.sub.Stop()
}

// tail subscribes to the channel for the windows events
func (t *Tailer) tail() {
	if t.evtapi == nil {
		t.evtapi = winevtapi.New()
	}
	// subscription
	t.sub = evtsubscribe.NewPullSubscription(
		t.config.ChannelPath,
		t.config.Query,
		evtsubscribe.WithWindowsEventLogAPI(t.evtapi),
		evtsubscribe.WithEventBatchCount(10),
		evtsubscribe.WithNotifyEventsAvailable())
	err := t.sub.Start()
	if err != nil {
		err = fmt.Errorf("failed to start subscription: %v", err)
		log.Errorf("%v", err)
		t.source.Status.Error(err)
		return
	}

	// render context for system values
	t.systemRenderContext, err = t.evtapi.EvtCreateRenderContext(nil, evtapi.EvtRenderContextSystem)
	if err != nil {
		err = fmt.Errorf("failed to create system render context: %v", err)
		log.Errorf("%v", err)
		t.source.Status.Error(err)
		return
	}
	defer evtapi.EvtCloseRenderContext(t.evtapi, t.systemRenderContext)

	t.source.Status.Success()

	// wait for stop signal
	t.eventLoop()
	close(t.done)
}

func (t *Tailer) eventLoop() {
	for {
		// if subscription is not running, try to start it with an exponential backoff
		if !t.sub.Running() {
			reset_backoff := backoff.NewExponentialBackOff()
			reset_backoff.InitialInterval = 1 * time.Second
			reset_backoff.MaxInterval = 1 * time.Minute
			// retry never stops if MaxElapsedTime == 0
			reset_backoff.MaxElapsedTime = 0
			err := backoff.Retry(func() error {
				err := t.sub.Start()
				if err != nil {
					err = fmt.Errorf("failed to start subscription: %v", err)
					log.Error(err)
					return err
				}
				// if stop event is set then return PermanentError to stop the retry loop
				select {
				case <-t.stop:
					return backoff.Permanent(fmt.Errorf("stop event set"))
				default:
				}
				// subscription started!
				return nil
			}, reset_backoff)
			if err != nil {
				var permanent *backoff.PermanentError
				if errors.As(err, &permanent) {
					return
				}
				continue
			}
			t.source.Status.Success()
		}

		// subscription is running, wait for and get events
		select {
		case <-t.stop:
			return
		case _, ok := <-t.sub.EventsAvailable():
			if !ok {
				break
			}
			// events are available, read them
			for {
				events, err := t.sub.GetEvents()
				if err != nil {
					// error
					log.Errorf("GetEvents failed: %v", err)
					t.sub.Stop()
					t.source.Status.Error(fmt.Errorf("subscription stopped: %v", err))
					break
				}
				if events == nil {
					// no more events
					log.Debugf("No more events")
					break
				}
				for _, eventRecord := range events {
					t.handleEvent(eventRecord.EventRecordHandle)
					evtapi.EvtCloseRecord(t.evtapi, eventRecord.EventRecordHandle)
				}
			}
		}
	}
}

func (t *Tailer) handleEvent(eventRecordHandle evtapi.EventRecordHandle) {

	richEvt := t.enrichEvent(eventRecordHandle)

	msg, err := t.toMessage(richEvt)
	if err != nil {
		log.Warnf("Couldn't convert xml to json: %s for event %s", err, richEvt.xmlEvent)
		return
	}

	t.source.RecordBytes(int64(len(msg.Content)))
	t.outputChan <- msg
}

// enrichEvent renders event record fields using (EvtRender, EvtFormatMessage)
func (t *Tailer) enrichEvent(event evtapi.EventRecordHandle) *richEvent {
	xmlData, err := t.evtapi.EvtRenderEventXml(event)
	if err != nil {
		log.Warnf("Error rendering xml: %v", err)
		return nil
	}
	xml := windows.UTF16ToString(xmlData)
	log.Debug(xml)

	vals, err := t.evtapi.EvtRenderEventValues(t.systemRenderContext, event)
	if err != nil {
		log.Warnf("Error rendering event values: %v", err)
		return nil
	}
	defer vals.Close()

	providerName, err := vals.String(evtapi.EvtSystemProviderName)
	if err != nil {
		log.Warnf("Failed to get provider name: %v", err)
		return nil
	}

	pm, err := t.evtapi.EvtOpenPublisherMetadata(providerName, "")
	if err != nil {
		log.Warnf("Failed to get publisher metadata for provider '%s': %v", providerName, err)
		return nil
	}
	defer evtapi.EvtClosePublisherMetadata(t.evtapi, pm)

	var message, task, opcode, level string

	message, _ = t.evtapi.EvtFormatMessage(pm, event, 0, nil, evtapi.EvtFormatMessageEvent)
	task, _ = t.evtapi.EvtFormatMessage(pm, event, 0, nil, evtapi.EvtFormatMessageTask)
	opcode, _ = t.evtapi.EvtFormatMessage(pm, event, 0, nil, evtapi.EvtFormatMessageOpcode)
	level, _ = t.evtapi.EvtFormatMessage(pm, event, 0, nil, evtapi.EvtFormatMessageLevel)

	// Truncates the message. Messages with more than 128kB are likely to be bigger
	// than 256kB when serialized and then dropped
	if len(message) >= maxRunes {
		message = message + truncatedFlag
	}

	return &richEvent{
		xmlEvent: xml,
		message:  message,
		task:     task,
		opcode:   opcode,
		level:    level,
	}
}
