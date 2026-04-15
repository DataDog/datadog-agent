// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package wsl

import (
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/extract"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/types"
)

// Backend implements types.Backend for WSL
type Backend struct {
	target *types.Target
	logger types.Logger
}

// GetKernelHeaders downloads the headers to the provided directory.
func (b *Backend) GetKernelHeaders(directory string) error {
	filename := b.target.Uname.Kernel + ".tar.gz"
	url := fmt.Sprintf("https://codeload.github.com/microsoft/WSL2-Linux-Kernel/tar.gz/%s", b.target.Uname.Kernel)

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return extract.ExtractTarball(resp.Body, filename, directory, b.logger)
}

// Close releases resources.
func (b *Backend) Close() {}

// NewBackend creates a new WSL backend
func NewBackend(target *types.Target, logger types.Logger) (*Backend, error) {
	backend := &Backend{
		target: target,
		logger: logger,
	}

	return backend, nil
}
