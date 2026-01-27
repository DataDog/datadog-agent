// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_http

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/httpclient"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GetChecksumFromURLHandler struct {
	httpClientProvider httpclient.Provider
}

type GetChecksumFromURLInputs struct {
	URL       string `json:"url"`
	Algorithm string `json:"algorithm"`
	Digest    string `json:"digest"`
}

type GetChecksumFromURLOutputs struct {
	Checksum string `json:"checksum"`
}

func NewGetChecksumFromURLHandler(runnerConfig *config.Config) types.Action {
	return &GetChecksumFromURLHandler{
		httpClientProvider: httpclient.NewDefaultProvider(runnerConfig),
	}
}

func (g GetChecksumFromURLHandler) Run(ctx context.Context, task *types.Task, _ *privateconnection.PrivateCredentials) (interface{}, error) {
	inputs, err := types.ExtractInputs[GetChecksumFromURLInputs](task)
	if err != nil {
		return nil, err
	}
	if inputs.URL == "" {
		return nil, errors.New("no URL provided")
	}
	data, err := fetchDataFromURL(ctx, g.httpClientProvider, inputs.URL)
	if err != nil {
		return nil, err
	}
	if inputs.Algorithm == "" {
		inputs.Algorithm = "sha256"
	}
	h, err := createHash(strings.ToLower(inputs.Algorithm))
	if h == nil {
		return nil, err
	}
	h.Write(data)
	checksum := encodeData(h.Sum(nil), inputs.Digest)
	return &GetChecksumFromURLOutputs{Checksum: checksum}, nil
}

func fetchDataFromURL(ctx context.Context, httpClientProvider httpclient.Provider, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	client, err := httpClientProvider.NewDefaultClient()
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err = resp.Body.Close(); err != nil {
			log.FromContext(ctx).Warnf("Failed to close response body: %v", err)
		}
	}()
	return io.ReadAll(resp.Body)
}

func encodeData(data []byte, digest string) string {
	switch digest {
	case "base64":
		return base64.StdEncoding.EncodeToString(data)
	case "base64url":
		return base64.RawURLEncoding.EncodeToString(data)
	case "binary":
		return string(data)
	default:
		return hex.EncodeToString(data)
	}
}

func createHash(algorithm string) (hash.Hash, error) {
	switch algorithm {
	case "md5":
		return md5.New(), nil
	case "sha1":
		return sha1.New(), nil
	case "sha256":
		return sha256.New(), nil
	case "sha384":
		return sha512.New384(), nil
	case "sha512":
		return sha512.New(), nil
	default:
		return nil, fmt.Errorf("unsupported hash algorithm: %s", algorithm)
	}
}
