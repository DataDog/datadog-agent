// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ebpf_bindata

package bytecode

import (
	"bytes"
	"embed"
	"io"
	"path"
)

//go:embed build/runtime-security.o
//go:embed build/runtime-security-syscall-wrapper.o
//go:embed build/runtime-security-offset-guesser.o
var bindata embed.FS

// GetReader returns a new AssetReader for the specified bundled asset
func GetReader(dir, name string) (AssetReader, error) {
	dir = "build"
	assetPath := path.Join(dir, name)

	content, err := bindata.ReadFile(assetPath)
	if err != nil {
		return nil, err
	}

	return nopCloser{bytes.NewReader(content)}, nil
}

type readerAt interface {
	io.Reader
	io.ReaderAt
}

type nopCloser struct {
	readerAt
}

func (nopCloser) Close() error { return nil }
