// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package jobmetadata validates DogStatsD GPU job metadata control events.
package jobmetadata

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// ControlEventTitle is the reserved DogStatsD event title used by the demo path.
	ControlEventTitle = "datadog.gpu.job"
	// ControlEventSourceType is the reserved DogStatsD event source type used by the demo path.
	ControlEventSourceType = "datadog_gpu_job"
	// JobIDTagPrefix is the tag prefix carrying the current job ID.
	JobIDTagPrefix = "gpu_job_id:"

	// DefaultTTL disables fallback expiry. Job tags remain until an end event by default.
	DefaultTTL = 0 * time.Second

	maxTags      = 16
	maxTagLength = 200
)

// Action is the lifecycle action carried by a GPU job metadata control event.
type Action string

const (
	// ActionUpdate publishes or refreshes job metadata tags for a container.
	ActionUpdate Action = "update"
	// ActionEnd clears job metadata tags for a container.
	ActionEnd Action = "end"
)

var (
	// ErrNoContainerID is returned when a control event cannot be mapped to a container.
	ErrNoContainerID = errors.New("missing container id")
	// ErrNoJobID is returned when an update control event does not include gpu_job_id.
	ErrNoJobID = errors.New("missing gpu_job_id tag")
	// ErrUnsupportedAction is returned when a control event has an unknown lifecycle action.
	ErrUnsupportedAction = errors.New("unsupported gpu job metadata action")
)

// Record is job metadata parsed from a control event for one container.
type Record struct {
	ContainerID string
	Action      Action
	JobID       string
	Tags        []string
	UpdatedAt   time.Time
	ExpiresAt   time.Time
}

// IsControlEvent returns true when the event is the reserved GPU job metadata event.
func IsControlEvent(title, sourceType string) bool {
	return title == ControlEventTitle && sourceType == ControlEventSourceType
}

// RecordFromEvent validates and normalizes a parsed control event. The returned
// bool is true when the event matched the reserved control-event shape and was
// consumed. The DogStatsD event text carries the action: "start", "update", and
// "heartbeat" publish tags, while "end" clears tags.
func RecordFromEvent(containerID, title, sourceType, text string, tags []string, ttl time.Duration) (Record, bool, error) {
	return recordFromEventAt(containerID, title, sourceType, text, tags, ttl, time.Now())
}

func recordFromEventAt(containerID, title, sourceType, text string, tags []string, ttl time.Duration, now time.Time) (Record, bool, error) {
	if !IsControlEvent(title, sourceType) {
		return Record{}, false, nil
	}

	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return Record{}, true, ErrNoContainerID
	}

	action, err := actionFromText(text)
	if err != nil {
		return Record{}, true, err
	}

	record := Record{
		ContainerID: containerID,
		Action:      action,
		UpdatedAt:   now,
	}

	if action == ActionEnd {
		return record, true, nil
	}

	jobID, eventTags, err := parseEventTags(tags)
	if err != nil {
		return Record{}, true, err
	}

	record.JobID = jobID
	record.Tags = normalizeTags(jobID, eventTags)
	if ttl > 0 {
		record.ExpiresAt = now.Add(ttl)
	}
	return copyRecord(record), true, nil
}

func actionFromText(text string) (Action, error) {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "", "start", "update", "heartbeat":
		return ActionUpdate, nil
	case "end", "stop", "complete", "completed":
		return ActionEnd, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupportedAction, text)
	}
}

func parseEventTags(tags []string) (string, []string, error) {
	var jobID string
	eventTags := make([]string, 0, len(tags))
	for _, rawTag := range tags {
		tag, ok := sanitizeTag(rawTag)
		if !ok {
			continue
		}
		if strings.HasPrefix(tag, JobIDTagPrefix) {
			if jobID == "" {
				jobID = strings.TrimSpace(strings.TrimPrefix(tag, JobIDTagPrefix))
			}
			continue
		}
		eventTags = append(eventTags, tag)
	}

	if jobID == "" {
		return "", nil, ErrNoJobID
	}
	return jobID, eventTags, nil
}

func normalizeTags(jobID string, tags []string) []string {
	jobTag := JobIDTagPrefix + jobID
	result := []string{jobTag}
	seen := map[string]struct{}{jobTag: {}}

	for _, rawTag := range tags {
		tag, ok := sanitizeTag(rawTag)
		if !ok || strings.HasPrefix(tag, JobIDTagPrefix) {
			continue
		}
		if _, found := seen[tag]; found {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
		if len(result) >= maxTags {
			break
		}
	}

	return result
}

func sanitizeTag(tag string) (string, bool) {
	tag = strings.TrimSpace(tag)
	if tag == "" || len(tag) > maxTagLength || !strings.Contains(tag, ":") {
		return "", false
	}
	if strings.HasPrefix(tag, "host:") || strings.HasPrefix(tag, "dd.internal:") || strings.HasPrefix(tag, "dd.internal.") {
		return "", false
	}
	return tag, true
}

func copyRecord(record Record) Record {
	record.Tags = append([]string(nil), record.Tags...)
	return record
}
