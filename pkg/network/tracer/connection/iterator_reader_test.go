// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package connection

import (
	"bufio"
	"errors"
	"io"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeReadResult struct {
	data []byte
	err  error
}

// fakeIteratorReader returns its results in order, then io.EOF.
type fakeIteratorReader struct {
	results []fakeReadResult
}

func (f *fakeIteratorReader) Read(p []byte) (int, error) {
	if len(f.results) == 0 {
		return 0, io.EOF
	}
	res := &f.results[0]
	n := copy(p, res.data)
	res.data = res.data[n:]
	if len(res.data) > 0 {
		return n, nil
	}
	err := res.err
	f.results = f.results[1:]
	return n, err
}

// eagain is what a read() on the blocking iterator file returns when the kernel stops early.
var eagain = &os.PathError{Op: "read", Path: "bpf_iter", Err: syscall.EAGAIN}

func TestEagainRetryReader(t *testing.T) {
	t.Run("retries EAGAIN until data and EOF", func(t *testing.T) {
		f := &fakeIteratorReader{results: []fakeReadResult{
			{err: eagain},
			{err: eagain},
			{data: []byte("record-1")},
			{err: eagain},
			{data: []byte("record-2")},
			{err: eagain},
		}}
		got, err := io.ReadAll(eagainRetryReader{r: f})
		require.NoError(t, err)
		assert.Equal(t, "record-1record-2", string(got))
	})

	t.Run("does not drop or duplicate bytes through a buffered reader", func(t *testing.T) {
		const recordSize = 7
		var want []byte
		var results []fakeReadResult
		for i := range 50 {
			rec := make([]byte, recordSize)
			for j := range rec {
				rec[j] = byte(i*recordSize + j)
			}
			want = append(want, rec...)
			results = append(results, fakeReadResult{data: rec})
			if i%3 == 0 {
				results = append(results, fakeReadResult{err: eagain})
			}
		}
		f := &fakeIteratorReader{results: results}
		r := bufio.NewReaderSize(eagainRetryReader{r: f}, 16)

		var got []byte
		buf := make([]byte, recordSize)
		for {
			_, err := io.ReadFull(r, buf)
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoError(t, err)
			got = append(got, buf...)
		}
		assert.Equal(t, want, got)
	})

	t.Run("propagates other errors", func(t *testing.T) {
		otherErr := &os.PathError{Op: "read", Path: "bpf_iter", Err: syscall.EINVAL}
		f := &fakeIteratorReader{results: []fakeReadResult{
			{data: []byte("record-1")},
			{err: otherErr},
			{data: []byte("record-2")},
		}}
		got, err := io.ReadAll(eagainRetryReader{r: f})
		assert.ErrorIs(t, err, syscall.EINVAL)
		assert.Equal(t, "record-1", string(got))
	})
}
