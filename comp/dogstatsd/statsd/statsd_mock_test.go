// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package statsd

import (
	"bytes"
	"io"
	"testing"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
)

func TestMockGet(t *testing.T) {
	s := fxutil.Test[Mock](t, MockModule())
	c, err := s.Get()
	require.NoError(t, err)
	require.NotNil(t, c)
}

type statsdWriter struct {
	*bytes.Buffer
}

func (s *statsdWriter) Close() error {
	return nil
}

var _ io.WriteCloser = &statsdWriter{}

func TestMockProvide(t *testing.T) {
	w := &statsdWriter{
		bytes.NewBufferString(""),
	}
	// ddgostatsd.WithoutOriginDetection() to make sure we don't have the container in the output.
	mc, err := ddgostatsd.NewWithWriter(w, ddgostatsd.WithoutOriginDetection())
	assert.NoError(t, err)
	s := fxutil.Test[Mock](t,
		MockModule(),
		fx.Replace(fx.Annotate(mc, fx.As(new(MockClient)))),
	)
	c, err := s.Get()
	assert.NoError(t, err)
	_ = c.Count("foo", 1, []string{"foo:bar"}, 1)
	_ = c.Flush()
	_ = c.Close()
	assert.Equal(t, "foo:1|c|#foo:bar\n", w.String())
}
