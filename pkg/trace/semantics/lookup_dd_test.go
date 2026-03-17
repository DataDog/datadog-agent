// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package semantics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDDSpanAccessor(t *testing.T) {
	t.Run("GetString reads from meta", func(t *testing.T) {
		a := NewDDSpanAccessor(
			map[string]string{"span.kind": "server"},
			map[string]float64{},
		)
		assert.Equal(t, "server", a.GetString("span.kind"))
	})

	t.Run("GetString does not read from metrics", func(t *testing.T) {
		a := NewDDSpanAccessor(
			map[string]string{},
			map[string]float64{"span.kind": 1},
		)
		assert.Equal(t, "", a.GetString("span.kind"))
	})

	t.Run("GetInt64 reads from metrics", func(t *testing.T) {
		a := NewDDSpanAccessor(
			map[string]string{},
			map[string]float64{"http.status_code": 200},
		)
		v, ok := a.GetInt64("http.status_code")
		assert.True(t, ok)
		assert.Equal(t, int64(200), v)
	})

	t.Run("GetInt64 does not read from meta", func(t *testing.T) {
		a := NewDDSpanAccessor(
			map[string]string{"http.status_code": "200"},
			map[string]float64{},
		)
		_, ok := a.GetInt64("http.status_code")
		assert.False(t, ok)
	})

	t.Run("GetFloat64 reads from metrics", func(t *testing.T) {
		a := NewDDSpanAccessor(
			map[string]string{},
			map[string]float64{"sampling.priority": 1.0},
		)
		v, ok := a.GetFloat64("sampling.priority")
		assert.True(t, ok)
		assert.Equal(t, 1.0, v)
	})

	t.Run("GetFloat64 does not read from meta", func(t *testing.T) {
		a := NewDDSpanAccessor(
			map[string]string{"sampling.priority": "1.0"},
			map[string]float64{},
		)
		_, ok := a.GetFloat64("sampling.priority")
		assert.False(t, ok)
	})

	t.Run("http.status_code in metrics resolves via int64 registry entry", func(t *testing.T) {
		r, err := NewEmbeddedRegistry()
		require.NoError(t, err)

		// Newer agents store http.status_code in Metrics as float64(200).
		a := NewDDSpanAccessor(
			map[string]string{},
			map[string]float64{"http.status_code": 200},
		)
		v, ok := LookupInt64(r, a, ConceptHTTPStatusCode)
		assert.True(t, ok)
		assert.Equal(t, int64(200), v)

		s := LookupString(r, a, ConceptHTTPStatusCode)
		assert.Equal(t, "200", s)
	})

	t.Run("http.status_code in meta resolves via string registry entry", func(t *testing.T) {
		r, err := NewEmbeddedRegistry()
		require.NoError(t, err)

		// Older agents store http.status_code in Meta as a string.
		a := NewDDSpanAccessor(
			map[string]string{"http.status_code": "404"},
			map[string]float64{},
		)
		v, ok := LookupInt64(r, a, ConceptHTTPStatusCode)
		assert.True(t, ok)
		assert.Equal(t, int64(404), v)

		s := LookupString(r, a, ConceptHTTPStatusCode)
		assert.Equal(t, "404", s)
	})

	t.Run("nil maps return empty/false", func(t *testing.T) {
		a := NewDDSpanAccessor(nil, nil)
		assert.Equal(t, "", a.GetString("key"))
		_, ok := a.GetInt64("key")
		assert.False(t, ok)
		_, ok = a.GetFloat64("key")
		assert.False(t, ok)
	})
}
