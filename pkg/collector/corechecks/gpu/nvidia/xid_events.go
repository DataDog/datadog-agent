// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
)

const (
	xidEventMergeWindow = 100 * time.Millisecond

	// NVML reports the maximum uint32 value when an event has no applicable MIG instance ID.
	nvmlInstanceIDNotApplicable = math.MaxUint32
)

type xidEvent struct {
	DeviceUUID  string
	XIDCode     uint64
	Timestamp   time.Time
	NVMLEvent   *observedDeviceEvent
	DriverEvent *model.DriverEvent
}

func (x xidEvent) toSample() Sample {
	origin := xidOriginUnknown
	if knownOrigin, ok := xidCodeToOrigin[x.XIDCode]; ok {
		origin = knownOrigin
	}

	text := fmt.Sprintf("NVIDIA XID %d was reported by NVML; no kernel message was available.", x.XIDCode)
	tags := []string{
		"xid_code:" + strconv.FormatUint(x.XIDCode, 10),
		"origin:" + origin,
	}
	if x.NVMLEvent != nil {
		tags = append(tags, "event_source:nvml")
		if x.NVMLEvent.GPUInstanceID != nvmlInstanceIDNotApplicable {
			tags = append(tags, "gpu_instance_id:"+strconv.FormatUint(uint64(x.NVMLEvent.GPUInstanceID), 10))
		}
		if x.NVMLEvent.ComputeInstanceID != nvmlInstanceIDNotApplicable {
			tags = append(tags, "compute_instance_id:"+strconv.FormatUint(uint64(x.NVMLEvent.ComputeInstanceID), 10))
		}
	}
	if x.DriverEvent != nil {
		tags = append(tags, "event_source:kmsg")
		if x.DriverEvent.NvidiaXid.Message != "" {
			text = x.DriverEvent.NvidiaXid.Message
		}
		tags = append(tags, driverXIDTags(x.DriverEvent.NvidiaXid)...)
	}

	return NewEvent(event.Event{
		Title:          fmt.Sprintf("XID %d error on %s", x.XIDCode, x.DeviceUUID),
		Text:           text,
		Priority:       event.PriorityNormal,
		AlertType:      event.AlertTypeError,
		SourceTypeName: "gpu",
		EventType:      "gpu_xid",
		AggregationKey: x.DeviceUUID,
	}, x.Timestamp, Medium, tags, nil)
}

func driverXIDTags(xid *model.NvidiaXid) []string {
	var tags []string
	appendEventTags(reflect.ValueOf(xid), &tags)
	return tags
}

// appendEventTags recursively emits non-zero fields annotated with `event_tag` as "tag:value".
func appendEventTags(value reflect.Value, tags *[]string) {
	if !value.IsValid() {
		return
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return
	}

	valueType := value.Type()
	for i := 0; i < value.NumField(); i++ {
		fieldType := valueType.Field(i)
		fieldValue := value.Field(i)
		tagName, tagged := fieldType.Tag.Lookup("event_tag")
		if tagged {
			if tagValue, ok := eventTagValue(fieldValue); ok {
				*tags = append(*tags, tagName+":"+tagValue)
			}
			continue
		}
		appendEventTags(fieldValue, tags)
	}
}

func eventTagValue(value reflect.Value) (string, bool) {
	pointerValue := false
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return "", false
		}
		pointerValue = true
		value = value.Elem()
	}

	switch value.Kind() {
	case reflect.String:
		return value.String(), value.String() != ""
	case reflect.Bool:
		return strconv.FormatBool(value.Bool()), value.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(value.Int(), 10), pointerValue || value.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(value.Uint(), 10), pointerValue || value.Uint() != 0
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(value.Float(), 'g', -1, value.Type().Bits()), pointerValue || value.Float() != 0
	default:
		return "", false
	}
}

// xidEventMerger correlates events from a single device.
type xidEventMerger struct {
	window        time.Duration
	pendingNVML   []xidEvent
	pendingDriver []xidEvent
	latest        []xidEvent
}

func newXIDEventMerger(window time.Duration) *xidEventMerger {
	return &xidEventMerger{window: window}
}

func (m *xidEventMerger) Refresh(queryTime time.Time, nvmlEvents []observedDeviceEvent, driverEvents []model.DriverEvent, driverEventsEnabled bool) {
	nvmlXIDs := append(slices.Clone(m.pendingNVML), convertNVMLXIDEvents(nvmlEvents)...)
	driverXIDs := append(slices.Clone(m.pendingDriver), convertDriverXIDEvents(driverEvents)...)

	sortXIDEvents(nvmlXIDs)
	sortXIDEvents(driverXIDs)

	matched, unmatchedNVML, unmatchedDriver := m.matchEvents(nvmlXIDs, driverXIDs)
	m.finalizeEvents(queryTime, matched, unmatchedNVML, unmatchedDriver, driverEventsEnabled)
}

func (m *xidEventMerger) finalizeEvents(queryTime time.Time, matched, unmatchedNVML, unmatchedDriver []xidEvent, driverEventsEnabled bool) {
	finalized := matched
	m.pendingNVML = m.pendingNVML[:0]
	m.pendingDriver = m.pendingDriver[:0]
	if !driverEventsEnabled {
		finalized = append(finalized, unmatchedNVML...)
		finalized = append(finalized, unmatchedDriver...)
	} else {
		cutoff := queryTime.Add(-m.window)
		finalized, m.pendingNVML = finalizeOldXIDEvents(finalized, unmatchedNVML, cutoff)
		finalized, m.pendingDriver = finalizeOldXIDEvents(finalized, unmatchedDriver, cutoff)
	}

	sortXIDEvents(finalized)
	m.latest = finalized
}

func (m *xidEventMerger) GetEvents() []xidEvent {
	return slices.Clone(m.latest)
}

func convertNVMLXIDEvents(events []observedDeviceEvent) []xidEvent {
	xids := make([]xidEvent, 0, len(events))
	for _, event := range events {
		if event.EventType != nvml.EventTypeXidCriticalError {
			continue
		}
		nvmlEvent := event
		xids = append(xids, xidEvent{
			DeviceUUID: event.DeviceUUID,
			XIDCode:    event.EventData,
			Timestamp:  event.ObservedAt,
			NVMLEvent:  &nvmlEvent,
		})
	}
	return xids
}

func convertDriverXIDEvents(events []model.DriverEvent) []xidEvent {
	xids := make([]xidEvent, 0, len(events))
	for i := range events {
		event := events[i]
		if event.Type != model.DriverEventTypeNvidiaXid || event.NvidiaXid == nil {
			continue
		}
		xids = append(xids, xidEvent{
			DeviceUUID:  event.DeviceUUID,
			XIDCode:     event.NvidiaXid.XidCode,
			Timestamp:   event.Timestamp,
			DriverEvent: &event,
		})
	}
	return xids
}

// matchEvents groups events by XID code, then pairs each source in timestamp order
// when timestamps are within the correlation window. Events that cannot be paired remain unmatched.
// Both input slices must belong to the same device and be sorted by timestamp.
func (m *xidEventMerger) matchEvents(nvmlEvents, driverEvents []xidEvent) (matched, unmatchedNVML, unmatchedDriver []xidEvent) {
	nvmlByCode := make(map[uint64][]xidEvent)
	driverByCode := make(map[uint64][]xidEvent)
	codes := make(map[uint64]struct{})
	for _, event := range nvmlEvents {
		nvmlByCode[event.XIDCode] = append(nvmlByCode[event.XIDCode], event)
		codes[event.XIDCode] = struct{}{}
	}
	for _, event := range driverEvents {
		driverByCode[event.XIDCode] = append(driverByCode[event.XIDCode], event)
		codes[event.XIDCode] = struct{}{}
	}

	for code := range codes {
		nvmlGroup := nvmlByCode[code]
		driverGroup := driverByCode[code]
		nvmlIndex, driverIndex := 0, 0
		for nvmlIndex < len(nvmlGroup) && driverIndex < len(driverGroup) {
			nvmlEvent := nvmlGroup[nvmlIndex]
			driverEvent := driverGroup[driverIndex]
			delta := nvmlEvent.Timestamp.Sub(driverEvent.Timestamp)
			if absDuration(delta) <= m.window {
				driverEvent.NVMLEvent = nvmlEvent.NVMLEvent
				matched = append(matched, driverEvent)
				nvmlIndex++
				driverIndex++
			} else if delta < 0 {
				unmatchedNVML = append(unmatchedNVML, nvmlEvent)
				nvmlIndex++
			} else {
				unmatchedDriver = append(unmatchedDriver, driverEvent)
				driverIndex++
			}
		}
		unmatchedNVML = append(unmatchedNVML, nvmlGroup[nvmlIndex:]...)
		unmatchedDriver = append(unmatchedDriver, driverGroup[driverIndex:]...)
	}
	return matched, unmatchedNVML, unmatchedDriver
}

func finalizeOldXIDEvents(finalized, unmatched []xidEvent, cutoff time.Time) ([]xidEvent, []xidEvent) {
	var pending []xidEvent
	for _, event := range unmatched {
		if event.Timestamp.Before(cutoff) {
			finalized = append(finalized, event)
		} else {
			pending = append(pending, event)
		}
	}
	return finalized, pending
}

func sortXIDEvents(events []xidEvent) {
	sort.SliceStable(events, func(i, j int) bool {
		if !events[i].Timestamp.Equal(events[j].Timestamp) {
			return events[i].Timestamp.Before(events[j].Timestamp)
		}
		if events[i].DeviceUUID != events[j].DeviceUUID {
			return events[i].DeviceUUID < events[j].DeviceUUID
		}
		return events[i].XIDCode < events[j].XIDCode
	})
}

func absDuration(duration time.Duration) time.Duration {
	if duration < 0 {
		return -duration
	}
	return duration
}
