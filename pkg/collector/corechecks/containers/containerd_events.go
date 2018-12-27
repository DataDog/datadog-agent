// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build containerd

package containers

import (
	"context"
	"fmt"
	"sync"
	"time"

	ctrUtil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/namespaces"
	"github.com/gogo/protobuf/proto"
)

//ContainerdEvent contains the timestamp to make sure we flush all events that happened between two checks
type ContainerdEvent struct {
	ID        string
	Timestamp time.Time
	Topic     string
	Namespace string
	Message   string
	Extra     map[string]string
}

type Subscriber struct {
	sync.Mutex
	Name                string
	Filters             []string
	Events              []ContainerdEvent
	Namespace           string
	CollectionTimestamp int64
	isRunning           bool
}

func CreateEventSubscriber(name string, ns string, f []string) *Subscriber {
	return &Subscriber{
		Name:                name,
		Namespace:           ns,
		CollectionTimestamp: time.Now().Unix(),
		Filters:             f,
	}
}

func (s *Subscriber) CheckEvents(ctrItf ctrUtil.ContainerdItf) {
	ctx := context.Background()
	ev := ctrItf.GetEvents()
	log.Info("Starting routine to collect Containerd events ...")
	ctxNamespace := namespaces.WithNamespace(ctx, s.Namespace)
	go s.run(ev, ctxNamespace)
}

// Run should only be called once, at start time.
func (s *Subscriber) run(ev containerd.EventService, ctx context.Context) error {
	stream, errC := ev.Subscribe(ctx, s.Filters...)
	s.Lock()
	s.isRunning = true
	s.Unlock()
	for {
		select {
		case message := <-stream:
			switch message.Topic {
			case "/containers/create":
				create := &events.ContainerCreate{}
				err := proto.Unmarshal(message.Event.Value, create)
				if err != nil {
					log.Errorf("Could not process create event from Containerd: %v", err)
					continue
				}
				event := ContainerdEvent{
					ID:        create.ID,
					Timestamp: message.Timestamp,
					Topic:     message.Topic,
					Namespace: message.Namespace,
					Message:   fmt.Sprintf("Container %s started, running the image %s", create.ID, create.Image),
				}
				s.addEvents(event)
			case "/containers/delete":
				delete := &events.ContainerDelete{}
				err := proto.Unmarshal(message.Event.Value, delete)
				if err != nil {
					log.Errorf("Could not process delete event from Containerd: %v", err)
					continue
				}
				event := ContainerdEvent{
					ID:        delete.ID,
					Timestamp: message.Timestamp,
					Topic:     message.Topic,
					Namespace: message.Namespace,
					Message:   fmt.Sprintf("Container %s deleted", delete.ID),
				}
				s.addEvents(event)
			case "/containers/update":
				updated := &events.ContainerUpdate{}
				err := proto.Unmarshal(message.Event.Value, updated)
				if err != nil {
					log.Errorf("Could not process update event from Containerd: %v", err)
					continue
				}
				event := ContainerdEvent{
					ID:        updated.ID,
					Timestamp: message.Timestamp,
					Topic:     message.Topic,
					Namespace: message.Namespace,
					Message:   fmt.Sprintf("Container %s updated, running image %s. Snapshot key: %s", updated.ID, updated.Image, updated.SnapshotKey),
					Extra:     updated.Labels,
				}
				s.addEvents(event)
			case "/images/update":
				updated := &events.ImageUpdate{}
				err := proto.Unmarshal(message.Event.Value, updated)
				if err != nil {
					log.Errorf("Could not process update event from Containerd: %v", err)
					continue
				}
				event := ContainerdEvent{
					ID:        updated.Name,
					Timestamp: message.Timestamp,
					Topic:     message.Topic,
					Namespace: message.Namespace,
					Message:   fmt.Sprintf("Image %s updated", updated.Name),
					Extra:     updated.Labels,
				}
				s.addEvents(event)
			case "/images/create":
				created := &events.ImageCreate{}
				err := proto.Unmarshal(message.Event.Value, created)
				if err != nil {
					log.Errorf("Could not process create event from Containerd: %v", err)
					continue
				}
				event := ContainerdEvent{
					ID:        created.Name,
					Timestamp: message.Timestamp,
					Topic:     message.Topic,
					Namespace: message.Namespace,
					Message:   fmt.Sprintf("Image %s created", created.Name),
					Extra:     created.Labels,
				}
				s.addEvents(event)
			case "/images/delete":
				deleted := &events.ImageDelete{}
				err := proto.Unmarshal(message.Event.Value, deleted)
				if err != nil {
					log.Errorf("Could not process delete event from Containerd: %v", err)
					continue
				}
				event := ContainerdEvent{
					ID:        deleted.Name,
					Timestamp: message.Timestamp,
					Topic:     message.Topic,
					Namespace: message.Namespace,
					Message:   fmt.Sprintf("Image %s created", deleted.Name),
				}
				s.addEvents(event)
			case "/tasks/create":
				created := &events.TaskCreate{}
				err := proto.Unmarshal(message.Event.Value, created)
				if err != nil {
					log.Errorf("Could not process create event from Containerd: %v", err)
					continue
				}
				event := ContainerdEvent{
					ID:        created.ContainerID,
					Timestamp: message.Timestamp,
					Topic:     message.Topic,
					Namespace: message.Namespace,
					Message:   fmt.Sprintf("Task %s created with PID %d", created.ContainerID, created.Pid),
				}
				s.addEvents(event)
			case "/tasks/delete":
				deleted := &events.TaskDelete{}
				err := proto.Unmarshal(message.Event.Value, deleted)
				if err != nil {
					log.Errorf("Could not process delete event from Containerd: %v", err)
					continue
				}
				event := ContainerdEvent{
					ID:        deleted.ContainerID,
					Timestamp: message.Timestamp,
					Topic:     message.Topic,
					Namespace: message.Namespace,
					Message:   fmt.Sprintf("Task %s deleted with exit code %d", deleted.ContainerID, deleted.ExitStatus),
				}
				s.addEvents(event)
			case "/tasks/exit":
				exited := &events.TaskExit{}
				err := proto.Unmarshal(message.Event.Value, exited)
				if err != nil {
					log.Errorf("Could not process exit event from Containerd: %v", err)
					continue
				}
				event := ContainerdEvent{
					ID:        exited.ContainerID,
					Timestamp: message.Timestamp,
					Topic:     message.Topic,
					Namespace: message.Namespace,
					Message:   fmt.Sprintf("Task %s exited with exit code %d", exited.ContainerID, exited.ExitStatus),
				}
				s.addEvents(event)
			case "/tasks/oom":
				oomed := &events.TaskOOM{}
				err := proto.Unmarshal(message.Event.Value, oomed)
				if err != nil {
					log.Errorf("Could not process create event from Containerd: %v", err)
					continue
				}
				event := ContainerdEvent{
					ID:        oomed.ContainerID,
					Timestamp: message.Timestamp,
					Topic:     message.Topic,
					Namespace: message.Namespace,
					Message:   fmt.Sprintf("Task %s ran out of memory", oomed.ContainerID),
				}
				s.addEvents(event)
			case "/tasks/paused":
				paused := &events.TaskPaused{}
				err := proto.Unmarshal(message.Event.Value, paused)
				if err != nil {
					log.Errorf("Could not process create event from Containerd: %v", err)
					continue
				}
				event := ContainerdEvent{
					ID:        paused.ContainerID,
					Timestamp: message.Timestamp,
					Topic:     message.Topic,
					Namespace: message.Namespace,
					Message:   fmt.Sprintf("Task %s was paused", paused.ContainerID),
				}
				s.addEvents(event)
			case "/tasks/resumed":
				resumed := &events.TaskResumed{}
				err := proto.Unmarshal(message.Event.Value, resumed)
				if err != nil {
					log.Errorf("Could not process create event from Containerd: %v", err)
					continue
				}
				event := ContainerdEvent{
					ID:        resumed.ContainerID,
					Timestamp: message.Timestamp,
					Topic:     message.Topic,
					Namespace: message.Namespace,
					Message:   fmt.Sprintf("Task %s was resumed", resumed.ContainerID),
				}
				s.addEvents(event)
			default:
				log.Tracef("Unsupported event type from Containerd: %s ", message.Topic)
			}
		case e := <-errC:
			log.Errorf("Error while streaming logs from containerd: %s", e.Error())
			// As we only collect events from one containerd namespace, using this bool is sufficient.
			s.Lock()
			s.isRunning = false
			s.Unlock()
			return fmt.Errorf("stopping Containerd event listener routine...")
		}
	}
}

func (s *Subscriber) addEvents(event ContainerdEvent) {
	s.Lock()
	s.Events = append(s.Events, event)
	s.Unlock()
}

func (s *Subscriber) IsRunning() bool {
	s.Lock()
	defer s.Unlock()
	return s.isRunning
}

// Flush should be called every time you want to
func (s *Subscriber) Flush(timestamp int64) []ContainerdEvent {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	delta := s.CollectionTimestamp - timestamp
	if len(s.Events) == 0 {
		log.Tracef("No events collected in the last %d seconds", delta)
		return nil
	}
	s.CollectionTimestamp = timestamp
	ev := s.Events
	log.Debugf("Collecting %d events from Containerd", len(ev))
	s.Events = []ContainerdEvent{}
	return ev
}
