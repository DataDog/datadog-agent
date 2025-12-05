// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seelog

import (
	"errors"
	"io"
)

// splitWriter writes to multiple writers, without stopping on the first error like io.MultiWriter does
type splitWriter struct {
	writers []io.Writer
}

// newSplitWriter returns a writer which writes to all the writers
func newSplitWriter(writers ...io.Writer) io.Writer {
	if len(writers) == 1 {
		return writers[0]
	}
	return &splitWriter{writers: writers}
}

// Write writes to all the writers, joining the errors if any
//
// It returns the minimum number of bytes written
func (w *splitWriter) Write(p []byte) (n int, err error) {
	var errs []error
	minN := len(p)
	for _, writer := range w.writers {
		n, err := writer.Write(p)
		if err != nil {
			errs = append(errs, err)
			minN = min(minN, n)
		}
	}

	return minN, errors.Join(errs...)
}
