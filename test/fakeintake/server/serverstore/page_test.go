// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package serverstore

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/stretchr/testify/require"
)

func TestRawPayloadPages(t *testing.T) {
	s := newInMemoryStore()
	now := time.Now()
	appendRecord := func(body string, at time.Time) {
		require.NoError(t, s.AppendPayload("logs", "key", []byte(body), "zstd", "json", at, api.RequestMetadata{Headers: http.Header{"X-Values": {"a", "b"}}}))
	}
	appendRecord("first", now)
	appendRecord("second", now.Add(time.Second))
	first, err := s.GetRawPayloadPage("logs", "", 1)
	require.NoError(t, err)
	require.True(t, first.HasMore)
	require.Equal(t, []byte("first"), first.Payloads[0].Data)
	// A reader cannot mutate stored evidence through the page.
	first.Payloads[0].Data[0] = 'X'
	first.Payloads[0].Request.Headers.Set("X-Values", "changed")
	again, err := s.GetRawPayloadPage("logs", "", 1)
	require.NoError(t, err)
	require.Equal(t, []byte("first"), again.Payloads[0].Data)
	require.Equal(t, []string{"a", "b"}, again.Payloads[0].Request.Headers["X-Values"])
	second, err := s.GetRawPayloadPage("logs", first.NextCursor, 1)
	require.NoError(t, err)
	require.False(t, second.HasMore)
	require.Equal(t, []byte("second"), second.Payloads[0].Data)
	empty, err := s.GetRawPayloadPage("logs", second.NextCursor, 1)
	require.NoError(t, err)
	require.Empty(t, empty.Payloads)
	appendRecord("third", now.Add(2*time.Second))
	third, err := s.GetRawPayloadPage("logs", empty.NextCursor, 1)
	require.NoError(t, err)
	require.Equal(t, []byte("third"), third.Payloads[0].Data)
	_, err = s.GetRawPayloadPage("other", first.NextCursor, 1)
	require.ErrorIs(t, err, ErrInvalidCursor)
	_, err = s.GetRawPayloadPage("logs", "malformed", 1)
	require.ErrorIs(t, err, ErrInvalidCursor)
	for _, limit := range []int{0, MaxPageSize + 1} {
		_, err = s.GetRawPayloadPage("logs", "", limit)
		require.Error(t, err)
	}
	future, _ := json.Marshal(payloadCursor{s.generation, "logs", 99})
	_, err = s.GetRawPayloadPage("logs", base64.RawURLEncoding.EncodeToString(future), 1)
	require.ErrorIs(t, err, ErrInvalidCursor)
	s.CleanUpPayloadsOlderThan(now.Add(2 * time.Second))
	_, err = s.GetRawPayloadPage("logs", first.NextCursor, 1)
	require.ErrorIs(t, err, ErrExpiredCursor)
	retained, err := s.GetRawPayloadPage("logs", second.NextCursor, 1)
	require.NoError(t, err)
	require.Equal(t, []byte("third"), retained.Payloads[0].Data)
	s.Flush()
	_, err = s.GetRawPayloadPage("logs", third.NextCursor, 1)
	require.ErrorIs(t, err, ErrExpiredCursor)
	fresh := newInMemoryStore()
	_, err = fresh.GetRawPayloadPage("logs", third.NextCursor, 1)
	require.ErrorIs(t, err, ErrExpiredCursor)
}
