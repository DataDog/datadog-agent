// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package cos is a backend for COS
package cos

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/extract"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/types"
)

// Backend implements types.Backend for COS
type Backend struct {
	buildID string
	logger  types.Logger
}

const (
	kernelHeadersFilename = "kernel-headers.tgz"
	bucketName            = "cos-tools"
)

// GetKernelHeaders downloads the headers to the provided directory.
func (b *Backend) GetKernelHeaders(directory string) error {
	objectName := url.QueryEscape(fmt.Sprintf("%s/%s", b.buildID, kernelHeadersFilename))
	objurl := fmt.Sprintf("https://storage.googleapis.com/download/storage/v1/b/%s/o/%s?alt=media", bucketName, objectName)

	resp, err := http.Get(objurl)
	if err != nil {
		return fmt.Errorf("start download kernel headers from COS bucket: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download kernel headers from COS bucket: %s", resp.Status)
	}

	if err := extract.ExtractTarball(resp.Body, kernelHeadersFilename, directory, b.logger); err != nil {
		return fmt.Errorf("extract kernel headers: %w", err)
	}

	return nil
}

// Close releases resources.
func (b *Backend) Close() {}

// NewBackend creates a backend for COS
func NewBackend(target *types.Target, logger types.Logger) (*Backend, error) {
	buildID := target.OSRelease["BUILD_ID"]
	if buildID == "" {
		return nil, errors.New("detect COS version: missing BUILD_ID in /etc/os-release")
	}

	return &Backend{
		logger:  logger,
		buildID: buildID,
	}, nil
}
