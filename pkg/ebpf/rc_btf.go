// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"runtime"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/archive"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type btfCatalog struct {
	X64   btfArchCatalog `json:"x86_64"`
	Arm64 btfArchCatalog `json:"arm64"`
}

// keyed by distro name
type btfArchCatalog map[btfPlatform]btfDistroCatalog

func (c btfArchCatalog) get(distro btfPlatform, release, version string) *btfEntry {
	entry, ok := c[distro][release][version]
	if !ok {
		return nil
	}
	return &entry
}

// keyed by release
type btfDistroCatalog map[string]btfReleaseCatalog

// keyed by kernel version
type btfReleaseCatalog map[string]btfEntry

type btfEntry struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

func (b *orderedBTFLoader) loadRemoteConfig() (*returnBTF, error) {
	if !b.rcBTFEnabled {
		return nil, nil
	}

	platform, err := getBTFPlatform()
	if err != nil {
		return nil, fmt.Errorf("BTF platform: %s", err)
	}
	platformVersion, err := kernel.PlatformVersion()
	if err != nil {
		return nil, fmt.Errorf("platform version: %s", err)
	}
	kernelVersion, err := kernel.Release()
	if err != nil {
		return nil, fmt.Errorf("kernel release: %s", err)
	}

	ctx, cancelCause := context.WithCancelCause(context.Background())
	ctx, cancel := context.WithTimeout(ctx, b.rcTimeout)
	defer cancel()

	rcLoader := rcBTFLoader{
		ctx:             ctx,
		cancelCause:     cancelCause,
		platform:        platform,
		platformVersion: platformVersion,
		kernelVersion:   kernelVersion,
		result:          make(chan *returnBTF),
	}

	b.rcclient.Subscribe(state.ProductBTFDD, rcLoader.rcCallback)
	defer func() {
		// ignore all future updates once we return from this function
		rcLoader.ignoreUpdates.Store(true)
	}()

	// wait for one of: timeout expiring, an error (via cancel cause func), or a successful result
	var rbtf *returnBTF
	select {
	case <-ctx.Done():
	case rbtf = <-rcLoader.result:
	}

	// ensure we get the correct error (if any) from the context
	err = ctx.Err()
	if errors.Is(err, context.Canceled) {
		err = context.Cause(ctx)
	}
	return rbtf, err
}

type rcBTFLoader struct {
	b *orderedBTFLoader

	ctx             context.Context
	cancelCause     context.CancelCauseFunc
	platform        btfPlatform
	platformVersion string
	kernelVersion   string
	result          chan *returnBTF
	ignoreUpdates   atomic.Bool
}

func (r *rcBTFLoader) rcCallback(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	var rbtf *returnBTF
	for k, config := range update {
		// we should ACK all updates, even if we find a result or error earlier
		if r.ctx.Err() != nil || r.ignoreUpdates.Load() || rbtf != nil {
			applyStateCallback(k, state.ApplyStatus{State: state.ApplyStateAcknowledged})
			continue
		}

		entry, err := r.findEntry(config)
		if err != nil {
			log.Errorf("error for BTF catalog key %q: %s", k, err)
			applyStateCallback(k, state.ApplyStatus{Error: err.Error()})
			continue
		}
		// we only indicate an error to remote config if we have trouble unmarshalling the data
		applyStateCallback(k, state.ApplyStatus{State: state.ApplyStateAcknowledged})
		if entry != nil {
			rbtf, err = r.processEntry(entry)
			if err != nil {
				log.Warn(err)
			}
		}
	}
	if rbtf == nil {
		r.cancelCause(fmt.Errorf("no BTF in catalog found for %s/%s/%s", r.platform, r.platformVersion, r.kernelVersion))
		return
	}
	r.result <- rbtf
}

func (r *rcBTFLoader) processEntry(entry *btfEntry) (*returnBTF, error) {
	btfTarballBuffer, err := r.downloadFile(entry.URL, entry.SHA256)
	if err != nil {
		return nil, fmt.Errorf("BTF download: %s", err)
	}

	// extract in-memory tarball to regular BTF output directory
	relPath := relativeBTFTarballPath(r.platform, r.platformVersion, r.kernelVersion)
	extractDir := filepath.Join(filepath.Dir(relPath), r.kernelVersion)
	absExtractDir := filepath.Join(r.b.btfOutputDir, extractDir)
	if err := archive.TarXZExtractAllReader(btfTarballBuffer, absExtractDir); err != nil {
		return nil, fmt.Errorf("extract kernel BTF from tarball: %w", err)
	}
	return r.b.checkforBTF(extractDir)
}

func (r *rcBTFLoader) findEntry(config state.RawConfig) (*btfEntry, error) {
	var catalog btfCatalog
	err := json.Unmarshal(config.Config, &catalog)
	if err != nil {
		return nil, fmt.Errorf("unmarshal BTF catalog: %s", err)
	}

	var entry *btfEntry
	switch runtime.GOARCH {
	case "amd64":
		entry = catalog.X64.get(r.platform, r.platformVersion, r.kernelVersion)
	case "arm64":
		entry = catalog.Arm64.get(r.platform, r.platformVersion, r.kernelVersion)
	default:
		log.Warnf("unsupported BTF architecture: %s", runtime.GOARCH)
		return nil, nil
	}
	return entry, nil
}

func (r *rcBTFLoader) downloadFile(url string, hash string) (*bytes.Reader, error) {
	req, err := http.NewRequestWithContext(r.ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create http request: %s", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do http request: %s", err)
	}
	defer resp.Body.Close()

	memOut := &bytes.Buffer{}
	h := sha256.New()
	// copy all reads from resp.Body to hash
	tee := io.TeeReader(resp.Body, h)
	// copy all reads to in-memory buffer
	if _, err := io.Copy(memOut, tee); err != nil {
		return nil, fmt.Errorf("copy response body data: %s", err)
	}

	calcHash := hex.EncodeToString(h.Sum(nil))
	if calcHash != hash {
		return nil, fmt.Errorf("hash for %s mismatch: expected %s, got %s", url, hash, calcHash)
	}
	return bytes.NewReader(memOut.Bytes()), nil
}
