// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	semconv126 "go.opentelemetry.io/otel/semconv/v1.26.0"
	semconv "go.opentelemetry.io/otel/semconv/v1.6.1"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
)

func TestObfuscateSQLSpan(t *testing.T) {
	o := obfuscate.NewObfuscator(obfuscate.Config{})
	defer o.Stop()

	t.Run("empty resource", func(t *testing.T) {
		span := &pb.Span{
			Resource: "",
			Meta:     map[string]string{},
		}
		oq, err := ObfuscateSQLSpan(o, span)
		assert.Nil(t, oq)
		assert.NoError(t, err)
		assert.Equal(t, "", span.Resource)
	})

	t.Run("non-parsable SQL query", func(t *testing.T) {
		span := &pb.Span{
			Resource: "This is completely invalid SQL that cannot be parsed at all",
			Meta:     map[string]string{},
		}
		oq, err := ObfuscateSQLSpan(o, span)
		if err != nil {
			// If obfuscation fails, resource should be set to TextNonParsable
			assert.Nil(t, oq)
			assert.Equal(t, TextNonParsable, span.Resource)
			assert.Equal(t, TextNonParsable, span.Meta[TagSQLQuery])
		} else {
			// If obfuscator can handle it, that's also valid
			require.NotNil(t, oq)
			assert.NotEmpty(t, span.Resource)
		}
	})

	t.Run("OTel db.statement matching resource", func(t *testing.T) {
		query := "SELECT * FROM orders WHERE status = 'pending'"
		span := &pb.Span{
			Resource: query,
			Meta: map[string]string{
				string(semconv.DBStatementKey): query,
			},
		}
		oq, err := ObfuscateSQLSpan(o, span)
		require.NoError(t, err)
		require.NotNil(t, oq)
		// Both resource and db.statement should be obfuscated the same way
		assert.Equal(t, span.Resource, span.Meta[string(semconv.DBStatementKey)])
		assert.Equal(t, "SELECT * FROM orders WHERE status = ?", span.Resource)
		assert.Equal(t, "SELECT * FROM orders WHERE status = ?", span.Meta[string(semconv.DBStatementKey)])
	})

	t.Run("with OTel db.statement different from resource", func(t *testing.T) {
		span := &pb.Span{
			Resource: "SELECT name FROM orders WHERE id = 1",
			Meta: map[string]string{
				string(semconv.DBStatementKey): "SELECT email FROM users WHERE id = 2",
			},
		}
		oq, err := ObfuscateSQLSpan(o, span)
		require.NoError(t, err)
		require.NotNil(t, oq)
		// Resource and db.statement should be obfuscated independently
		assert.Equal(t, "SELECT name FROM orders WHERE id = ?", span.Resource)
		assert.Equal(t, "SELECT email FROM users WHERE id = ?", span.Meta[string(semconv.DBStatementKey)])
	})

	t.Run("with OTel db.query.text matching resource", func(t *testing.T) {
		query := "SELECT * FROM products WHERE price > 100"
		span := &pb.Span{
			Resource: query,
			Meta: map[string]string{
				string(semconv126.DBQueryTextKey): query,
			},
		}
		oq, err := ObfuscateSQLSpan(o, span)
		require.NoError(t, err)
		require.NotNil(t, oq)
		// Both resource and db.query.text should be obfuscated the same way
		assert.Equal(t, span.Resource, span.Meta[string(semconv126.DBQueryTextKey)])
		assert.Equal(t, "SELECT * FROM products WHERE price > ?", span.Resource)
		assert.Equal(t, "SELECT * FROM products WHERE price > ?", span.Meta[string(semconv126.DBQueryTextKey)])
	})

	t.Run("with OTel db.query.text different from resource", func(t *testing.T) {
		span := &pb.Span{
			Resource: "SELECT name FROM orders WHERE id = 1",
			Meta: map[string]string{
				string(semconv126.DBQueryTextKey): "SELECT title FROM products WHERE id = 2",
			},
		}
		oq, err := ObfuscateSQLSpan(o, span)
		require.NoError(t, err)
		require.NotNil(t, oq)
		// Resource and db.query.text should be obfuscated independently
		assert.Equal(t, "SELECT name FROM orders WHERE id = ?", span.Resource)
		assert.Equal(t, "SELECT title FROM products WHERE id = ?", span.Meta[string(semconv126.DBQueryTextKey)])
	})

}
