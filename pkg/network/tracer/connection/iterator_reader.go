// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package connection

import (
	"errors"
	"io"
	"syscall"
)

// eagainRetryReader retries reads that fail with EAGAIN without returning any data.
//
// A read() on a BPF iterator returns EAGAIN when the kernel visits MAX_ITER_OBJECTS (1M)
// objects without the program writing anything. The kernel stops the iteration before the
// next object and the next read() resumes from it, so retrying neither skips nor repeats
// records. Each EAGAIN advances the iterator by 1M objects, so the retries end once the
// iterator writes a record or reaches the end.
type eagainRetryReader struct {
	r io.Reader
}

func (e eagainRetryReader) Read(p []byte) (int, error) {
	for {
		n, err := e.r.Read(p)
		if n == 0 && errors.Is(err, syscall.EAGAIN) {
			continue
		}
		return n, err
	}
}
