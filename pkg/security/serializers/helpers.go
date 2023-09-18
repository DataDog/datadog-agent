// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package serializers defines functions aiming to serialize events
package serializers

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	jwriter "github.com/mailru/easyjson/jwriter"
)

// nolint: deadcode, unused
func getUint64Pointer(i *uint64) *uint64 {
	if *i == 0 {
		return nil
	}
	return i
}

// nolint: deadcode, unused
func getUint32Pointer(i *uint32) *uint32 {
	if *i == 0 {
		return nil
	}
	return i
}

// nolint: deadcode, unused
func getTimeIfNotZero(t time.Time) *utils.EasyjsonTime {
	if t.IsZero() {
		return nil
	}
	tt := utils.NewEasyjsonTime(t)
	return &tt
}

// MarshalEvent marshal the event
func MarshalEvent(event *model.Event, probe *resolvers.Resolvers) ([]byte, error) {
	//s := NewEventSerializer(event, probe)
	w := &jwriter.Writer{
		Flags: jwriter.NilSliceAsEmpty | jwriter.NilMapAsEmpty,
	}
	//s.MarshalEasyJSON(w)
	return w.BuildBytes()
}

// MarshalCustomEvent marshal the custom event
func MarshalCustomEvent(event *events.CustomEvent) ([]byte, error) {
	w := &jwriter.Writer{
		Flags: jwriter.NilSliceAsEmpty | jwriter.NilMapAsEmpty,
	}
	event.MarshalEasyJSON(w)
	return w.BuildBytes()
}
