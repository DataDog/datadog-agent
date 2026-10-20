// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"bytes"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/trace/api/apiutil"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

// bodyReader is a ReadCloser that deliberately does NOT implement io.WriterTo.
// This matters: io.Copy prefers src.WriteTo over dst.ReadFrom, so a bare
// *bytes.Reader (or io.NopCloser wrapping one) would bypass bytes.Buffer.ReadFrom
// and fail to reproduce the production path. net/http's *http.body has no WriteTo.
type bodyReader struct {
	r bytes.Reader
}

func (b *bodyReader) Read(p []byte) (int, error) { return b.r.Read(p) }
func (b *bodyReader) Close() error               { return nil }

// newBodyRequest builds a request with a Content-Length header, which is what
// reserveBodySize keys off of.
func newBodyRequest(payload []byte) (*http.Request, *bodyReader) {
	body := &bodyReader{}
	body.r.Reset(payload)
	req := &http.Request{
		Method:        http.MethodPost,
		Header:        http.Header{},
		Body:          body,
		ContentLength: int64(len(payload)),
	}
	req.Header.Set("Content-Length", strconv.Itoa(len(payload)))
	return req, body
}

// testReceiver is the minimal receiver reserveBodySize needs. newTestReceiverConfig
// is not reused here because it binds a TCP port and sets timeouts that have no
// bearing on body buffering.
func testReceiver(maxRequestBytes int64) *HTTPReceiver {
	cfg := config.New()
	cfg.MaxRequestBytes = maxRequestBytes
	return &HTTPReceiver{conf: cfg}
}

// TestReserveBodySizeLimit pins the MaxRequestBytes boundary. reserveBodySize
// reserves bytes.MinRead beyond the body so bytes.Buffer.ReadFrom does not have to
// reallocate; that padding must not leak into the size check, or bodies of exactly
// MaxRequestBytes would start being rejected.
func TestReserveBodySizeLimit(t *testing.T) {
	const maxBytes = 1024

	tests := []struct {
		name          string
		contentLength string
		wantErr       bool
	}{
		{"under limit", "512", false},
		{"exactly at limit", strconv.Itoa(maxBytes), false},
		{"one over limit", strconv.Itoa(maxBytes + 1), true},
		{"absent", "", false},
		{"unparseable", "not-a-number", false}, // logged and treated as unknown size
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := testReceiver(maxBytes)
			req, _ := newBodyRequest(nil)
			if tc.contentLength == "" {
				req.Header.Del("Content-Length")
			} else {
				req.Header.Set("Content-Length", tc.contentLength)
			}

			buf := &bytes.Buffer{}
			err := r.reserveBodySize(buf, req)
			if tc.wantErr {
				assert.ErrorIs(t, err, apiutil.ErrLimitedReaderLimitReached)
				return
			}
			require.NoError(t, err)
			// Whatever size was reserved, ReadFrom must never need to reallocate.
			assert.GreaterOrEqual(t, buf.Available(), bytes.MinRead)
		})
	}
}

// TestCopyRequestBodyNoRealloc verifies that receiving a body of known length into
// a cold buffer does not reallocate: the buffer reserved up front is the one the
// body lands in. Identity of the backing array is checked rather than an allocation
// count, because any run-based allocation measurement warms the buffer on its first
// pass and then reports zero regardless of how much was reserved.
func TestCopyRequestBodyNoRealloc(t *testing.T) {
	const size = 64 << 10
	payload := bytes.Repeat([]byte("x"), size)

	r := testReceiver(50 << 20)
	req, _ := newBodyRequest(payload)

	buf := &bytes.Buffer{}
	require.NoError(t, r.reserveBodySize(buf, req))
	// The buffer is empty, so its spare capacity starts where the body will land.
	reserved := buf.AvailableBuffer()
	require.NotZero(t, cap(reserved), "reserveBodySize must reserve capacity")
	reservedStart, reservedCap := &reserved[:1][0], cap(reserved)

	// copyRequestBody re-runs reserveBodySize, which is a no-op now that the buffer
	// already has room; the copy that follows is the production path.
	n, err := r.copyRequestBody(buf, req)
	require.NoError(t, err)
	require.Equal(t, int64(size), n)

	assert.Equal(t, payload, buf.Bytes(), "body must be copied intact")
	assert.Same(t, reservedStart, &buf.Bytes()[0],
		"body landed in a different array: the reserved buffer was reallocated")
	assert.Equal(t, reservedCap, cap(buf.AvailableBuffer())+buf.Len(),
		"buffer capacity changed: the reserved buffer was reallocated")
}
