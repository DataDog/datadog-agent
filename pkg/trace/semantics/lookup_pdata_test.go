// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package semantics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestPDataMapAccessor(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutStr("http.method", "GET")
	attrs.PutStr("db.statement", "SELECT * FROM users")
	attrs.PutInt("http.status_code", 200)
	attrs.PutDouble("custom.float", 3.14)

	accessor := NewPDataMapAccessor(attrs)

	assert.Equal(t, "GET", accessor("http.method"))
	assert.Equal(t, "SELECT * FROM users", accessor("db.statement"))
	assert.Equal(t, "200", accessor("http.status_code"))
	assert.Equal(t, "3.14", accessor("custom.float"))
	assert.Equal(t, "", accessor("nonexistent"))
}

func TestNewOTelSpanAccessor(t *testing.T) {
	spanAttrs := pcommon.NewMap()
	spanAttrs.PutStr("http.method", "POST")

	resAttrs := pcommon.NewMap()
	resAttrs.PutStr("http.method", "GET")
	resAttrs.PutStr("service.name", "my-service")

	accessor := NewOTelSpanAccessor(spanAttrs, resAttrs)

	assert.Equal(t, "POST", accessor("http.method"))
	assert.Equal(t, "my-service", accessor("service.name"))
	assert.Equal(t, "", accessor("nonexistent"))
}

func TestPDataAccessorWithRegistry(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	spanAttrs := pcommon.NewMap()
	spanAttrs.PutStr("db.statement", "SELECT * FROM users")

	resAttrs := pcommon.NewMap()

	accessor := NewOTelSpanAccessor(spanAttrs, resAttrs)
	result := LookupString(r, accessor, ConceptDBStatement)
	assert.Equal(t, "SELECT * FROM users", result)
}
