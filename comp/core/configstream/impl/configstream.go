// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package configstreamimpl implements the configstream component interface
package configstreamimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/comp/core/config"
	configstream "github.com/DataDog/datadog-agent/comp/core/configstream/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// Requires defines the dependencies for the configstream component
type Requires struct {
	Lifecycle compdef.Lifecycle
	Config    config.Component
	Log       log.Component
}

// Provides defines the output of the configstream component.
type Provides struct {
	Comp configstream.Component
}

type configStream struct {
	config config.Component
	log    log.Component

	m           sync.Mutex
	subscribers map[string]*subscription

	subscribeChan   chan *subscription
	unsubscribeChan chan string
	stopChan        chan struct{}
}

type subscription struct {
	id             string
	ch             chan *pb.ConfigEvent
	lastSequenceID uint64
}

// NewComponent creates a new configstream component.
func NewComponent(reqs Requires) Provides {
	cs := &configStream{
		config:          reqs.Config,
		log:             reqs.Log,
		subscribers:     make(map[string]*subscription),
		subscribeChan:   make(chan *subscription),
		unsubscribeChan: make(chan string),
		stopChan:        make(chan struct{}),
	}

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: func(_ context.Context) error {
			go cs.run()
			return nil
		},
		OnStop: func(_ context.Context) error {
			close(cs.stopChan)
			return nil
		},
	})

	return Provides{
		Comp: cs,
	}
}

// Subscribe returns a channel that streams configuration events, starting with a snapshot.
// It also returns an unsubscribe function that must be called to clean up.
func (cs *configStream) Subscribe(req *pb.ConfigStreamRequest) (<-chan *pb.ConfigEvent, func()) {
	subID := fmt.Sprintf("%s-%s", req.Name, uuid.New().String())
	subChan := make(chan *pb.ConfigEvent, 100) // Buffered channel to avoid blocking

	cs.subscribeChan <- &subscription{
		id: subID,
		ch: subChan,
	}

	unsubscribeFunc := func() {
		cs.unsubscribeChan <- subID
	}

	return subChan, unsubscribeFunc
}

func (cs *configStream) run() {
	updatesChan := make(chan *pb.ConfigEvent, 100)

	cs.config.OnUpdate(func(setting string, source model.Source, _, newValue interface{}, sequenceID uint64) {
		sanitizedValue, err := sanitizeValue(newValue)
		if err != nil {
			cs.log.Warnf("Failed to sanitize setting '%s': %v", setting, err)
			return
		}
		pbValue, err := structpb.NewValue(sanitizedValue)
		if err != nil {
			cs.log.Warnf("Failed to convert setting '%s' to structpb.Value: %v", setting, err)
			return
		}
		configUpdate := &pb.ConfigEvent{
			Event: &pb.ConfigEvent_Update{
				Update: &pb.ConfigUpdate{
					SequenceId: int32(sequenceID),
					Setting: &pb.ConfigSetting{
						Key:    setting,
						Value:  pbValue,
						Source: source.String(),
					},
				},
			},
		}

		select {
		case updatesChan <- configUpdate:
		default:
			cs.log.Warn("Config update channel is full, dropping update.")
		}
	})

	for {
		select {
		case sub := <-cs.subscribeChan:
			cs.addSubscriber(sub)
		case id := <-cs.unsubscribeChan:
			cs.removeSubscriber(id)
		case update := <-updatesChan:
			cs.handleConfigUpdate(update)
		case <-cs.stopChan:
			cs.m.Lock()
			for _, sub := range cs.subscribers {
				close(sub.ch)
			}
			cs.subscribers = make(map[string]*subscription)
			cs.m.Unlock()
			return
		}
	}
}

func (cs *configStream) addSubscriber(sub *subscription) {
	cs.log.Infof("New subscriber '%s' joining the config stream", sub.id)
	snapshot, seqID, err := cs.createConfigSnapshot()
	if err != nil {
		cs.log.Errorf("Failed to create config snapshot for new subscriber '%s': %v", sub.id, err)
		close(sub.ch)
		return
	}

	cs.m.Lock()
	defer cs.m.Unlock()

	sub.lastSequenceID = seqID
	cs.subscribers[sub.id] = sub

	// Send snapshot to the new subscriber
	sub.ch <- snapshot
}

func (cs *configStream) removeSubscriber(id string) {
	cs.m.Lock()
	defer cs.m.Unlock()

	if sub, ok := cs.subscribers[id]; ok {
		close(sub.ch)
		delete(cs.subscribers, id)
		cs.log.Infof("Subscriber '%s' removed from config stream", id)
	}
}

func (cs *configStream) handleConfigUpdate(event *pb.ConfigEvent) {
	cs.m.Lock()
	defer cs.m.Unlock()

	var snapshot *pb.ConfigEvent
	var snapshotSeqID uint64
	var snapshotErr error

	currentSequenceID := uint64(event.GetUpdate().SequenceId)

	for id, sub := range cs.subscribers {
		// Skip updates that are older than the last one we sent to this subscriber.
		if currentSequenceID <= sub.lastSequenceID {
			continue
		}

		eventToSend := event

		// Discontinuity detected for this subscriber: resync with a fresh snapshot.
		if currentSequenceID > sub.lastSequenceID+1 {
			// Lazily create the snapshot, only if it's needed for the first time in this update cycle.
			if snapshot == nil {
				snapshot, snapshotSeqID, snapshotErr = cs.createConfigSnapshot()
				if snapshotErr != nil {
					cs.log.Errorf("Failed to create resynchronization snapshot, all out-of-sync subscribers will remain so until the next update: %v", snapshotErr)
					continue // A snapshot could not be created, but other subscribers may still be able to process the incremental update.
				}
			}
			cs.log.Warnf("Discontinuity detected for subscriber '%s'. Last seen ID: %d, current ID: %d. Resynchronizing with a snapshot.", id, sub.lastSequenceID, currentSequenceID)
			sub.lastSequenceID = snapshotSeqID
			eventToSend = snapshot
		} else {
			// Contiguous update: update the sequence ID.
			sub.lastSequenceID = currentSequenceID
		}

		select {
		case sub.ch <- eventToSend:
		default:
			cs.log.Warnf("Dropping config update for subscriber '%s' because their channel is full", id)
		}
	}
}

func (cs *configStream) createConfigSnapshot() (*pb.ConfigEvent, uint64, error) {
	allSettings, sequenceID := cs.config.AllSettingsWithSequenceID()

	// Sanitize all settings to ensure compatibility with structpb.NewValue
	sanitizedSettings, err := sanitizeValue(allSettings)
	if err != nil {
		cs.log.Errorf("Failed to sanitize config settings while creating snapshot: %v", err)
		return nil, 0, err
	}

	intermediateMap := sanitizedSettings.(map[string]interface{})
	settings := make([]*pb.ConfigSetting, 0, len(intermediateMap))
	for setting, value := range intermediateMap {
		pbValue, err := structpb.NewValue(value)
		if err != nil {
			cs.log.Errorf("Failed to convert setting '%s' to structpb.Value: %v", setting, err)
			continue
		}
		source := cs.config.GetSource(setting).String()
		settings = append(settings, &pb.ConfigSetting{
			Source: source,
			Key:    setting,
			Value:  pbValue,
		})
	}

	snapshot := &pb.ConfigEvent{
		Event: &pb.ConfigEvent_Snapshot{
			Snapshot: &pb.ConfigSnapshot{
				SequenceId: int32(sequenceID),
				Settings:   settings,
			},
		},
	}

	return snapshot, sequenceID, nil
}

// sanitizeMapForJSON recursively converts map[interface{}]interface{} to map[string]interface{}
// to make the data structure JSON-serializable.
func sanitizeMapForJSON(data interface{}) interface{} {
	switch v := data.(type) {
	case map[interface{}]interface{}:
		result := make(map[string]interface{})
		for key, value := range v {
			stringKey := fmt.Sprintf("%v", key) // Convert any key type to string
			result[stringKey] = sanitizeMapForJSON(value)
		}
		return result

	case map[string]interface{}:
		result := make(map[string]interface{})
		for key, value := range v {
			result[key] = sanitizeMapForJSON(value)
		}
		return result

	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = sanitizeMapForJSON(item)
		}
		return result

	default:
		// Primitive types (string, int, bool, etc.) - return as-is
		return v
	}
}

// sanitizeValue is a workaround for `structpb.NewValue`, which cannot handle
// complex types like `map[string]string` or `map[interface{}]interface{}`.
// It first converts interface{} maps to string maps, then marshals to JSON and back
// to convert the value into a `structpb` compatible format.
func sanitizeValue(value interface{}) (interface{}, error) {
	// First sanitize to ensure JSON compatibility (handles map[interface{}]interface{})
	sanitized := sanitizeMapForJSON(value)

	// Then do the JSON round-trip for structpb compatibility
	data, err := json.Marshal(sanitized)
	if err != nil {
		return nil, err
	}
	var result interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}
