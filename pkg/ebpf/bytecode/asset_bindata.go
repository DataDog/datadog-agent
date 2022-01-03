// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ebpf_bindata
// +build ebpf_bindata

package bytecode

import (
	"bytes"
	"io"

	bindata "github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/bindata"
)

// GetReader returns a new AssetReader for the specified bundled asset
func GetReader(dir, name string) (AssetReader, error) {
	content, err := bindata.Asset(name)
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
