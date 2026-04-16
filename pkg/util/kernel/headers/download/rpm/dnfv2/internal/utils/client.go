// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"compress/gzip"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/rpm/dnfv2/types"
)

// HTTPClient wraps http.Client with utility methods
type HTTPClient struct {
	inner *http.Client
}

// NewHTTPClientFromInner wraps the provided http.Client to expose utility methods.
func NewHTTPClientFromInner(inner *http.Client) *HTTPClient {
	return &HTTPClient{inner: inner}
}

// GetWithChecksum obtains the data at url while also verifying the provided checksum (if not nil).
func (hc *HTTPClient) GetWithChecksum(ctx context.Context, url string, checksum *types.Checksum) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := hc.inner.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status for `%s`: %s", url, resp.Status)
	}

	gzipped := URLHasSuffix(url, ".gz") || resp.Header.Get("Content-Encoding") == "gzip"
	var rdr io.Reader = resp.Body
	if gzipped {
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("gzip reader: %s", err)
		}
		defer gzipReader.Close()
		rdr = gzipReader
	}

	var readContent []byte
	if checksum != nil {
		var hasher hash.Hash
		switch checksum.Type {
		case "sha256":
			hasher = sha256.New()
		case "sha1":
			hasher = sha1.New()
		default:
			return nil, fmt.Errorf("unsupported sha type: %s", checksum.Type)
		}
		readContent, err = io.ReadAll(io.TeeReader(rdr, hasher))
		if err != nil {
			return nil, err
		}
		contentSum := hasher.Sum(nil)
		if checksum.Hash != hex.EncodeToString(contentSum) {
			return nil, errors.New("failed checksum")
		}
		return readContent, nil
	}

	return io.ReadAll(rdr)
}

// Get obtains the data at url without a checksum
func (hc *HTTPClient) Get(ctx context.Context, url string) ([]byte, error) {
	return hc.GetWithChecksum(ctx, url, nil)
}
