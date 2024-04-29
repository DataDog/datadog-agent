// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

//go:build serverless

package replay

import (
	"io"
)

// WriteHeader writes the datadog header to the Writer argument to conform to the .dog file format.
func WriteHeader(w io.Writer) error {
	return nil
}

func fileVersion(buf []byte) (int, error) {
	return 0, nil
}
