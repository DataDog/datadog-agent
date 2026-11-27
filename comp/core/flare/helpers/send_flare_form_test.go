// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || darwin || windows

package helpers

import (
	"bytes"
	"io"
)

// readPastFormFileHeader reads the multipart body until the archive's form part starts.
func readPastFormFileHeader(r io.Reader) error {
	var sent bytes.Buffer
	buf := make([]byte, 512)
	for !bytes.Contains(sent.Bytes(), []byte("application/octet-stream\r\n\r\n")) {
		n, err := r.Read(buf)
		sent.Write(buf[:n])
		if err != nil {
			return err
		}
	}
	return nil
}
