// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package helm

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
)

// Ref: https://github.com/helm/helm/blob/v3.8.0/pkg/storage/driver/util.go#L56

var b64 = base64.StdEncoding
var magicGzip = []byte{0x1f, 0x8b, 0x08}

// ParseRelease decodes Helm's stored "release" data.
func ParseRelease(data string) (*Release, error) {
	b, err := b64.DecodeString(data)
	if err != nil {
		return nil, err
	}
	if len(b) < 4 {
		// Avoid a panic if b[0:3] cannot be accessed.
		return nil, fmt.Errorf("the byte array is too short (expected at least 4 bytes, got %d instead): it cannot contain a Helm release", len(b))
	}

	if bytes.Equal(b[0:3], magicGzip) {
		r, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		b2, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		b = b2
	}

	var rls Release
	if err := json.Unmarshal(b, &rls); err != nil {
		return nil, err
	}
	return &rls, nil
}
