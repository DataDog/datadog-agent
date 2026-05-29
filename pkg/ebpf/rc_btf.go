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

var (
	errBTFCatalogUnmarshal = errors.New("unmarshal BTF catalog")
	errBTFNotInCatalog     = errors.New("BTF not in catalog")
	errBTFDownload         = errors.New("BTF download")
	errBTFExtract          = errors.New("extract kernel BTF from tarball")
	errBTFHashMismatch     = errors.New("BTF hash mismatch")
	errBTFLoad             = errors.New("BTF load")
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
	SHA256 string `json:"sha256"`
}

func rcArchitecture() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "arm64"
	default:
		return ""
	}
}

func (b *orderedBTFLoader) loadRemoteConfig(ctx context.Context) (*returnBTF, error) {
	if !b.rcBTFEnabled {
		return nil, nil
	}

	arch := rcArchitecture()
	if arch == "" {
		log.Warnf("unsupported BTF architecture: %s", runtime.GOARCH)
		return nil, nil
	}
	if b.platform == "" || b.platformVersion == "" || b.kernelVersion == "" {
		plat, _ := kernel.Platform()
		log.Warnf("unsupported BTF platform/version/release: %s/%s/%s", plat, b.platformVersion, b.kernelVersion)
		return nil, nil
	}

	ctx, cancelCause := context.WithCancelCause(ctx)
	ctx, cancel := context.WithTimeout(ctx, b.rcTimeout)
	defer cancel()

	rcLoader := rcBTFLoader{
		b:               b,
		ctx:             ctx,
		cancelCause:     cancelCause,
		platform:        b.platform,
		platformVersion: b.platformVersion,
		kernelVersion:   b.kernelVersion,
		arch:            arch,
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
	err := ctx.Err()
	if rbtf == nil {
		errorTags := make([]string, len(b.fixedtags), len(b.fixedtags)+1)
		copy(errorTags, b.fixedtags)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				err = context.Cause(ctx)
			}
			if errors.Is(err, context.DeadlineExceeded) {
				errorTags = append(errorTags, "timeout")
			} else if errors.Is(err, errBTFNotInCatalog) {
				errorTags = append(errorTags, "not_in_catalog")
			} else if errors.Is(err, errBTFCatalogUnmarshal) {
				errorTags = append(errorTags, "catalog_unmarshal")
			} else if errors.Is(err, errBTFHashMismatch) {
				errorTags = append(errorTags, "hash_mismatch")
			} else if errors.Is(err, errBTFDownload) {
				errorTags = append(errorTags, "download")
			} else if errors.Is(err, errBTFExtract) {
				errorTags = append(errorTags, "extract")
			} else if errors.Is(err, errBTFLoad) {
				errorTags = append(errorTags, "load")
			} else {
				errorTags = append(errorTags, "unknown")
			}
		} else {
			errorTags = append(errorTags, "not_in_catalog")
		}
		b.telemetry.rcErrors.Inc(errorTags...)
	} else {
		b.telemetry.rcSuccess.Inc(b.fixedtags...)
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
	arch            string
	result          chan *returnBTF
	ignoreUpdates   atomic.Bool
}

func (r *rcBTFLoader) rcCallback(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	var rbtf *returnBTF
	var allErrors []error
	for k, config := range update {
		// we should ACK all updates, even if we find a result or error earlier
		if r.ctx.Err() != nil || r.ignoreUpdates.Load() || rbtf != nil {
			applyStateCallback(k, state.ApplyStatus{State: state.ApplyStateAcknowledged})
			continue
		}

		entry, err := r.findEntry(config)
		if err != nil {
			log.Errorf("BTF remote config key %q: %s", k, err)
			applyStateCallback(k, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			allErrors = append(allErrors, err)
			continue
		}

		if entry != nil {
			rbtf, err = r.processEntry(entry)
			if err != nil {
				log.Error(err)
				applyStateCallback(k, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
				allErrors = append(allErrors, err)
				continue
			}
		}
		applyStateCallback(k, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}
	if rbtf == nil {
		if allErrors != nil {
			r.cancelCause(errors.Join(allErrors...))
		} else {
			r.cancelCause(fmt.Errorf("%w for %s/%s/%s/%s", errBTFNotInCatalog, r.arch, r.platform, r.platformVersion, r.kernelVersion))
		}
		return
	}
	r.result <- rbtf
}

func (r *rcBTFLoader) processEntry(entry *btfEntry) (*returnBTF, error) {
	btfURL := fmt.Sprintf("%s/btfs/%s/%s/%s/%s.btf.tar.xz", r.b.rcDownloadHost, r.platform, r.platformVersion, r.arch, r.kernelVersion)
	btfTarballBuffer, err := r.downloadFile(btfURL, entry.SHA256)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errBTFDownload, err)
	}

	// extract in-memory tarball to regular BTF output directory
	relPath := relativeBTFTarballPath(r.platform, r.platformVersion, r.kernelVersion)
	extractDir := filepath.Join(filepath.Dir(relPath), r.kernelVersion)
	absExtractDir := filepath.Join(r.b.btfOutputDir, extractDir)
	if err := archive.TarXZExtractAllReader(btfTarballBuffer, absExtractDir); err != nil {
		return nil, fmt.Errorf("%w: %s", errBTFExtract, err)
	}
	rbtf, err := r.b.checkforBTF(extractDir)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", errBTFLoad, err)
	}
	return rbtf, nil
}

func (r *rcBTFLoader) findEntry(config state.RawConfig) (*btfEntry, error) {
	var catalog btfCatalog
	err := json.Unmarshal(config.Config, &catalog)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", errBTFCatalogUnmarshal, err)
	}

	var entry *btfEntry
	switch r.arch {
	case "x86_64":
		entry = catalog.X64.get(r.platform, r.platformVersion, r.kernelVersion)
	case "arm64":
		entry = catalog.Arm64.get(r.platform, r.platformVersion, r.kernelVersion)
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
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad http status: %s", resp.Status)
	}

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
		return nil, fmt.Errorf("%w for %s: expected %s, got %s", errBTFHashMismatch, url, hash, calcHash)
	}
	return bytes.NewReader(memOut.Bytes()), nil
}
