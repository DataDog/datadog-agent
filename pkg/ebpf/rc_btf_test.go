// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func skipIfUnsupported(t *testing.T) {
	_, err := getBTFPlatform()
	if err != nil {
		t.Skip(err)
	}
}

type mockRCClient struct {
	t   *testing.T
	sub func(product data.Product, fn func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)))
}

func (rc *mockRCClient) Subscribe(product data.Product, fn func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))) {
	rc.sub(product, fn)
}

func (rc *mockRCClient) SubscribeAgentTask() {}

func TestRemoteConfigBTFTimeout(t *testing.T) {
	skipIfUnsupported(t)
	cfg := &Config{
		RemoteConfigBTFEnabled: true,
		RemoteConfigBTFTimeout: 1 * time.Millisecond,
	}
	mockRC := &mockRCClient{t: t, sub: func(product data.Product, _ func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))) {
		require.Equal(t, string(product), state.ProductBTFDD)
	}}
	loader := initBTFLoader(cfg, mockRC)

	_, err := loader.loadRemoteConfig(t.Context())
	assert.True(t, errors.Is(err, context.DeadlineExceeded))
}

const configTemplate = `
{
    "%s": {
        "%s": {
            "%s": {
                "%s": {
                    "sha256": "%s"
                }
            }
        }
    }
}`

func copyFile(t *testing.T, srcPath string, dstPath string) {
	src, err := os.Open(srcPath)
	require.NoError(t, err)
	defer src.Close()

	dst, err := os.Create(dstPath)
	require.NoError(t, err)
	defer dst.Close()

	_, err = io.Copy(dst, src)
	require.NoError(t, err)
}

func setupBTFTarXZFile(t *testing.T, kv string) (string, string) {
	tmpDir := t.TempDir()
	btfFile := filepath.Join(tmpDir, kv+".btf")
	copyFile(t, "./testdata/rc-btf-test.btf", btfFile)

	archiveFile := filepath.Join(tmpDir, kv+".btf.tar.xz")
	cmd := exec.Command("tar", "-cvJ", "-f", archiveFile, kv+".btf")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Log(string(out))
		t.Fatal(err)
	}

	sum, err := hashFile(archiveFile)
	require.NoError(t, err)

	return archiveFile, sum
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("unable to read input file: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("error hashing input file: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func getCatalog(t *testing.T, shasum string) string {
	platform, err := getBTFPlatform()
	require.NoError(t, err)
	platformVersion, err := kernel.PlatformVersion()
	require.NoError(t, err)
	kernelVersion, err := kernel.Release()
	require.NoError(t, err)

	return fmt.Sprintf(configTemplate, rcArchitecture(), platform, platformVersion, kernelVersion, shasum)
}

func setupBTFServer(t *testing.T) (string, string) {
	kernelVersion, err := kernel.Release()
	require.NoError(t, err)
	archiveFile, shasum := setupBTFTarXZFile(t, kernelVersion)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f, err := os.Open(archiveFile)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}))
	t.Cleanup(srv.Close)

	return srv.URL, shasum
}

func TestRemoteConfigBTFFoundEntry(t *testing.T) {
	skipIfUnsupported(t)
	serverURL, shasum := setupBTFServer(t)
	catalog := getCatalog(t, shasum)

	cfg := NewConfig()
	cfg.RemoteConfigBTFEnabled = true
	cfg.RemoteConfigBTFDownloadHost = serverURL
	cfg.RemoteConfigBTFTimeout = 5 * time.Second
	cfg.BTFOutputDir = t.TempDir() // must use temporary directory to not pollute BTF for other tests
	mockRC := &mockRCClient{
		t: t,
		sub: func(product data.Product, fn func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))) {
			require.Equal(t, string(product), state.ProductBTFDD)
			go func() {
				time.Sleep(1 * time.Millisecond)
				update := map[string]state.RawConfig{"testkey": {Config: []byte(catalog)}}
				fn(update, func(_ string, status state.ApplyStatus) {
					assert.Equal(t, state.ApplyStateAcknowledged, status.State)
					assert.Empty(t, status.Error)
				})
			}()
		},
	}
	loader := initBTFLoader(cfg, mockRC)
	ret, err := loader.loadRemoteConfig(t.Context())
	require.NoError(t, err)
	require.NotNil(t, ret.vmlinux)
}

func TestRemoteConfigBTFHashMismatch(t *testing.T) {
	skipIfUnsupported(t)
	serverURL, _ := setupBTFServer(t)
	catalog := getCatalog(t, "badsum")

	cfg := NewConfig()
	cfg.RemoteConfigBTFEnabled = true
	cfg.RemoteConfigBTFDownloadHost = serverURL
	cfg.RemoteConfigBTFTimeout = 5 * time.Second
	mockRC := &mockRCClient{
		t: t,
		sub: func(product data.Product, fn func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))) {
			require.Equal(t, string(product), state.ProductBTFDD)
			go func() {
				time.Sleep(1 * time.Millisecond)
				update := map[string]state.RawConfig{"testkey": {Config: []byte(catalog)}}
				fn(update, func(_ string, status state.ApplyStatus) {
					assert.Equal(t, state.ApplyStateError, status.State)
					assert.NotEmpty(t, status.Error)
				})
			}()
		},
	}
	loader := initBTFLoader(cfg, mockRC)
	ret, err := loader.loadRemoteConfig(t.Context())
	require.Error(t, err)
	require.Nil(t, ret)
}

func TestRemoteConfigBTFMissingEntry(t *testing.T) {
	skipIfUnsupported(t)
	serverURL, _ := setupBTFServer(t)
	catalog := fmt.Sprintf(configTemplate, rcArchitecture(), "otherplatform", "2", "3.10", "badsum")

	cfg := NewConfig()
	cfg.RemoteConfigBTFEnabled = true
	cfg.RemoteConfigBTFDownloadHost = serverURL
	cfg.RemoteConfigBTFTimeout = 5 * time.Second
	mockRC := &mockRCClient{
		t: t,
		sub: func(product data.Product, fn func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))) {
			require.Equal(t, string(product), state.ProductBTFDD)
			go func() {
				time.Sleep(1 * time.Millisecond)
				update := map[string]state.RawConfig{"testkey": {Config: []byte(catalog)}}
				fn(update, func(_ string, status state.ApplyStatus) {
					assert.Equal(t, state.ApplyStateAcknowledged, status.State)
					assert.Empty(t, status.Error)
				})
			}()
		},
	}
	loader := initBTFLoader(cfg, mockRC)
	ret, err := loader.loadRemoteConfig(t.Context())
	require.Error(t, err)
	require.Nil(t, ret)
}
