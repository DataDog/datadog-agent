// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package resources

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/genresources"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const maxFieldsBytes = 1_000_000

// Options controls resource retention and publication frequency.
type Options struct {
	TTL             time.Duration
	RefreshInterval time.Duration
	MaxPerRun       int
}

// Submitter is the event-platform subset of the check sender.
type Submitter interface{ EventPlatformEvent([]byte, string) }

type published struct {
	fields      map[string]json.RawMessage
	digest      [32]byte
	nextRefresh time.Time
}

// Publisher holds refresh state for one GPU check instance.
type Publisher struct {
	options Options
	last    map[string]published
	cursor  string
}

// NewPublisher creates a publisher. A zero refresh interval selects TTL/4.
func NewPublisher(options Options) (*Publisher, error) {
	if options.TTL <= 0 {
		return nil, errors.New("resource TTL must be positive")
	}
	if options.RefreshInterval == 0 {
		options.RefreshInterval = options.TTL / 4
	}
	if options.RefreshInterval <= 0 || options.RefreshInterval > options.TTL/4 {
		return nil, errors.New("resource refresh interval must be positive and at most one quarter of TTL")
	}
	if options.MaxPerRun == 0 {
		options.MaxPerRun = 64
	}
	if options.MaxPerRun < 0 {
		return nil, errors.New("maximum resources per run must be positive")
	}
	return &Publisher{options: options, last: make(map[string]published)}, nil
}

// Publish submits changed or refresh-due devices, retaining unavailable optional
// facts while a device remains enumerated. Submission does not acknowledge delivery.
func (p *Publisher) Publish(now time.Time, hostname string, devices []Device, sender Submitter) error {
	if strings.TrimSpace(hostname) == "" {
		return errors.New("canonical Agent hostname is empty")
	}
	byKey := make(map[string]Device, len(devices))
	for _, device := range devices {
		device.UUID, device.Hostname = strings.ToLower(device.UUID), hostname
		byKey[device.UUID] = device
	}
	for key := range p.last {
		if _, exists := byKey[key]; !exists {
			delete(p.last, key)
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	start := 0
	for start < len(keys) && keys[start] <= p.cursor {
		start++
	}
	if start == len(keys) {
		start = 0
	}

	var failures []error
	submitted := 0
	// Send only updated or refresh-due resources, limiting submissions per run.
	for offset := 0; offset < len(keys) && submitted < p.options.MaxPerRun; offset++ {
		key := keys[(start+offset)%len(keys)]
		p.cursor = key
		sent, err := p.publishDevice(now, byKey[key], sender)
		if err != nil {
			failures = append(failures, fmt.Errorf("GPU %s: %w", key, err))
		}
		if sent {
			submitted++
		}
	}
	return errors.Join(failures...)
}

func (p *Publisher) publishDevice(now time.Time, device Device, sender Submitter) (bool, error) {
	key := device.UUID
	encodedFields, err := json.Marshal(device)
	if err != nil {
		return false, fmt.Errorf("encode fields: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encodedFields, &fields); err != nil {
		return false, fmt.Errorf("decode fields: %w", err)
	}
	previous, exists := p.last[key]
	for name, value := range previous.fields {
		if _, known := fields[name]; !known {
			fields[name] = value
		}
	}
	encodedFields, err = json.Marshal(fields)
	if err != nil {
		return false, fmt.Errorf("encode complete fields: %w", err)
	}
	if len(encodedFields) > maxFieldsBytes {
		return false, fmt.Errorf("payload is %d bytes, exceeding limit %d", len(encodedFields), maxFieldsBytes)
	}
	digest := sha256.Sum256(encodedFields)
	if exists && digest == previous.digest && now.Before(previous.nextRefresh) {
		return false, nil
	}
	event := &genresources.GenericResourceEvent{
		// REDAPL only copies the authenticated org into events for this source.
		// Using "agent" would attribute these resources to org 0.
		Source: "integrations-core",
		Resource: &genresources.GenericResource{
			Type: "gpu_device", Key: key, FieldsJson: encodedFields,
			SeenAt: timestamppb.New(now), ExpireAt: timestamppb.New(now.Add(p.options.TTL)),
		},
	}
	encoded, err := proto.Marshal(event)
	if err != nil {
		return false, fmt.Errorf("encode event: %w", err)
	}
	sender.EventPlatformEvent(encoded, eventplatform.EventTypeGenResources)
	p.last[key] = published{fields: fields, digest: digest, nextRefresh: now.Add(p.options.RefreshInterval)}
	return true, nil
}
