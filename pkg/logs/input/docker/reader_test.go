// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build docker

package docker

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var errFoo = errors.New("foo error")
var errBar = errors.New("bar error")

type ReadCloserMock struct {
	io.Reader
	closer func() error
}

func (r *ReadCloserMock) Close() error {
	return r.closer()
}

func NewReadCloserMock(r io.Reader, closer func() error) io.ReadCloser {
	return &ReadCloserMock{
		Reader: r,
		closer: closer,
	}
}

type ReadErrorMock struct {
	io.Reader
}

func (r *ReadErrorMock) Read(p []byte) (int, error) {
	return 0, errFoo
}

func TestSafeReaderRead(t *testing.T) {
	reader := newSafeReader()
	bytesArray := []byte("foo")
	mockReadCloserNoError := NewReadCloserMock(bytes.NewReader(bytesArray), func() error {
		return nil
	})
	mockReadCloserReadError := NewReadCloserMock(&ReadErrorMock{}, func() error {
		return nil
	})

	n, err := reader.Read(bytesArray)
	assert.Equal(t, 0, n)
	assert.Equal(t, errReaderNotInitialized, err)

	reader.setUnsafeReader(mockReadCloserNoError)
	n, err = reader.Read(bytesArray)
	assert.Equal(t, len(bytesArray), n)
	assert.Nil(t, err)

	reader.setUnsafeReader(mockReadCloserReadError)
	n, err = reader.Read(bytesArray)
	assert.Equal(t, 0, n)
	assert.Equal(t, errFoo, err)

	reader.setUnsafeReader(nil)
	n, err = reader.Read(bytesArray)
	assert.Equal(t, 0, n)
	assert.Equal(t, errReaderNotInitialized, err)
}

func TestSafeReaderClose(t *testing.T) {
	reader := newSafeReader()
	mockReadCloserNoError := NewReadCloserMock(&ReadErrorMock{}, func() error {
		return nil
	})
	mockReadCloserCloseError := NewReadCloserMock(&ReadErrorMock{}, func() error {
		return errBar
	})

	err := reader.Close()
	assert.Equal(t, errReaderNotInitialized, err)

	reader.setUnsafeReader(mockReadCloserNoError)
	err = reader.Close()
	assert.Nil(t, err)

	reader.setUnsafeReader(mockReadCloserCloseError)
	err = reader.Close()
	assert.Equal(t, errBar, err)

	reader.setUnsafeReader(nil)
	err = reader.Close()
	assert.Equal(t, errReaderNotInitialized, err)
}

func Test_safeReader_getBackoffAndIncrement(t *testing.T) {
	type fields struct {
		backoffRetry           int
		backoffWaitDuration    time.Duration
		backoffDefaultDuration time.Duration
	}
	tests := []struct {
		name             string
		fields           fields
		want             time.Duration
		wantRetry        int
		wantWaitDuration time.Duration
	}{
		{
			name: "init backoff, should return 0",
			fields: fields{
				backoffRetry:           0,
				backoffWaitDuration:    0,
				backoffDefaultDuration: time.Second,
			},
			want:             0,
			wantRetry:        1,
			wantWaitDuration: time.Second,
		},
		{
			name: "second backoff, should return 1",
			fields: fields{
				backoffRetry:           1,
				backoffWaitDuration:    time.Second,
				backoffDefaultDuration: time.Second,
			},
			want:             time.Second,
			wantRetry:        2,
			wantWaitDuration: 3 * time.Second,
		},
		{
			name: "third backoff, should return 3",
			fields: fields{
				backoffRetry:           2,
				backoffWaitDuration:    3 * time.Second,
				backoffDefaultDuration: time.Second,
			},
			want:             3 * time.Second,
			wantRetry:        3,
			wantWaitDuration: 6 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &safeReader{
				backoffRetry:           tt.fields.backoffRetry,
				backoffWaitDuration:    tt.fields.backoffWaitDuration,
				backoffDefaultDuration: tt.fields.backoffDefaultDuration,
			}
			if got := s.getBackoffAndIncrement(); got != tt.want {
				t.Errorf("safeReader.getBackoffAndIncrement() = %v, want %v", got, tt.want)
			}
			if s.backoffRetry != tt.wantRetry {
				t.Errorf("safeReader.backoffRetry = %v, want %v", s.backoffRetry, tt.wantRetry)
			}
			if s.backoffWaitDuration != tt.wantWaitDuration {
				t.Errorf("safeReader.backoffWaitDuration = %v, want %v", s.backoffWaitDuration, tt.wantWaitDuration)
			}
		})
	}
}
