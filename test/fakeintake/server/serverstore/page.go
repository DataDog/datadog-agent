// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package serverstore

import (
	"encoding/base64"
	"encoding/json"
	"errors"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// ErrInvalidCursor means the cursor cannot identify a position on this route.
var ErrInvalidCursor = errors.New("invalid payload cursor")

// ErrExpiredCursor means flush or retention removed the cursor's position.
var ErrExpiredCursor = errors.New("payload cursor expired; history must be read again")

// MaxPageSize bounds the number of records cloned and marshaled in one response.
// A single original body is never truncated; this is not a byte-size bound.
const MaxPageSize = 128

type payloadCursor struct {
	Generation string
	Route      string
	Next       uint64
}

// GetRawPayloadPage clones only the selected records under the store lock.
// Each call observes its own high-water mark. Appends remain visible on the next
// call, including after HasMore=false. There is no cross-call snapshot or pin.
func (s *inMemoryStore) GetRawPayloadPage(route, cursor string, limit int) (api.PayloadPage, error) {
	if limit < 1 || limit > MaxPageSize {
		return api.PayloadPage{}, errors.New("limit must be between 1 and 128")
	}
	var c payloadCursor
	if cursor != "" {
		data, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || json.Unmarshal(data, &c) != nil || c.Route != route || c.Generation == "" {
			return api.PayloadPage{}, ErrInvalidCursor
		}
	}
	s.mutex.RLock()
	next := s.firstSequence[route]
	if cursor != "" {
		if c.Generation != s.generation || c.Next < next {
			s.mutex.RUnlock()
			return api.PayloadPage{}, ErrExpiredCursor
		}
		next = c.Next
	}
	end := s.firstSequence[route] + uint64(len(s.rawPayloads[route]))
	if next > end {
		s.mutex.RUnlock()
		return api.PayloadPage{}, ErrInvalidCursor
	}
	stop := min(next+uint64(limit), end)
	payloads := make([]api.Payload, 0, int(stop-next))
	for _, p := range s.rawPayloads[route][int(next-s.firstSequence[route]):int(stop-s.firstSequence[route])] {
		payloads = append(payloads, p.Clone())
	}
	generation := s.generation
	s.mutex.RUnlock()
	data, err := json.Marshal(payloadCursor{generation, route, stop})
	if err != nil {
		return api.PayloadPage{}, err
	}
	return api.PayloadPage{Payloads: payloads, NextCursor: base64.RawURLEncoding.EncodeToString(data), HasMore: stop < end}, nil
}
