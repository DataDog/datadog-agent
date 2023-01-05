// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"github.com/mailru/easyjson/jwriter"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

const (
	// ServiceEnvVar environment variable used to report service
	ServiceEnvVar = "DD_SERVICE"
)

var eventZero model.Event

// ValidateField validates the value of a field
/*func (fh *FieldHandlers) ValidateField(field eval.Field, fieldValue eval.FieldValue) error {
	if err := m.Model.ValidateField(field, fieldValue); err != nil {
		return err
	}

	switch field {
	case "bpf.map.name":
		if offset, found := m.probe.constantOffsets[constantfetch.OffsetNameBPFMapStructName]; !found || offset == constantfetch.ErrorSentinel {
			return fmt.Errorf("%s is not available on this kernel version", field)
		}

	case "bpf.prog.name":
		if offset, found := m.probe.constantOffsets[constantfetch.OffsetNameBPFProgAuxStructName]; !found || offset == constantfetch.ErrorSentinel {
			return fmt.Errorf("%s is not available on this kernel version", field)
		}
	}

	return nil
}*/

// NewEvent returns a new event
func NewEvent(fh *FieldHandlers, marshaler *EventMarshaler) *model.Event {
	return &model.Event{
		FieldHandlers: fh,
		JSONMarshaler: marshaler.MarshalJSONEvent,
	}
}

type EventMarshaler struct {
	probe *Probe
}

// MarshalJSONEvent returns the JSON encoding of the event
func (em *EventMarshaler) MarshalJSONEvent(ev *model.Event) ([]byte, error) {
	s := NewEventSerializer(ev, em.probe)
	w := &jwriter.Writer{
		Flags: jwriter.NilSliceAsEmpty | jwriter.NilMapAsEmpty,
	}
	s.MarshalEasyJSON(w)
	return w.BuildBytes()
}
