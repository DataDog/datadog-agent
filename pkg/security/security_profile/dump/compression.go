// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package dump

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"strings"
	"time"
)

func compressWithGZip(filename string, rawBuf []byte) (*bytes.Buffer, error) {
	var buf bytes.Buffer

	zw := gzip.NewWriter(&buf)
	zw.Name = strings.TrimSuffix(filename, ".gz")
	zw.ModTime = time.Now()

	if _, err := zw.Write(rawBuf); err != nil {
		return nil, fmt.Errorf("couldn't compress activity dump: %w", err)
	}
	// Closing the gzip stream also flushes it
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("couldn't compress activity dump: %w", err)
	}

	return &buf, nil
}
