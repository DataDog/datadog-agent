// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package apt

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	aptlyhttp "github.com/aptly-dev/aptly/http"
	"github.com/aptly-dev/aptly/utils"
	"github.com/stretchr/testify/require"
	"github.com/xor-gate/ar"

	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/types"
)

type quietLogger struct{ types.Logger }

func (quietLogger) Debugf(string, ...interface{}) {}

func headerPackage(t *testing.T) []byte {
	t.Helper()
	var payload bytes.Buffer
	gz := gzip.NewWriter(&payload)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "types.h", Mode: 0644, Size: 6}))
	_, err := tw.Write([]byte("header"))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	var pkg bytes.Buffer
	aw := ar.NewWriter(&pkg)
	require.NoError(t, aw.WriteGlobalHeader())
	require.NoError(t, aw.WriteHeader(&ar.Header{Name: "data.tar.gz", Size: int64(payload.Len()), Mode: 0644}))
	_, err = aw.Write(payload.Bytes())
	require.NoError(t, err)
	return pkg.Bytes()
}

func TestDownloadAuthenticatesBeforeExtraction(t *testing.T) {
	pkg := headerPackage(t)
	good := utils.ChecksumInfo{Size: int64(len(pkg)), SHA256: fmt.Sprintf("%x", sha256.Sum256(pkg))}
	for _, tc := range []struct {
		name    string
		sum     utils.ChecksumInfo
		content []byte
		wantOK  bool
	}{
		{"http sha256", good, pkg, true},
		{"http sha512", utils.ChecksumInfo{Size: int64(len(pkg)), SHA512: fmt.Sprintf("%x", sha512.Sum512(pkg))}, pkg, true},
		{"substituted bytes", good, bytes.Repeat([]byte{'x'}, len(pkg)), false},
		{"wrong size", utils.ChecksumInfo{Size: 1, SHA256: good.SHA256}, pkg, false},
		{"missing hash", utils.ChecksumInfo{Size: int64(len(pkg))}, pkg, false},
		{"weak hash only", utils.ChecksumInfo{Size: int64(len(pkg)), SHA1: "1234"}, pkg, false},
		{"malformed hash", utils.ChecksumInfo{Size: int64(len(pkg)), SHA256: "bad"}, pkg, false},
		{"invalid archive", utils.ChecksumInfo{Size: 3, SHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("bad")))}, []byte("bad"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(tc.content) }))
			defer server.Close()
			dir := t.TempDir()
			b := Backend{logger: quietLogger{}}
			err := b.downloadAndExtract(aptlyhttp.NewDownloader(0, 1, nil), server.URL+"/headers.deb?download=1", tc.sum, dir)
			if tc.wantOK {
				require.NoError(t, err)
				content, e := os.ReadFile(filepath.Join(dir, "types.h"))
				require.NoError(t, e)
				require.Equal(t, "header", string(content))
			} else {
				require.Error(t, err)
				require.NoFileExists(t, filepath.Join(dir, "types.h"))
			}
			entries, e := os.ReadDir(dir)
			require.NoError(t, e)
			for _, entry := range entries {
				require.Equal(t, "types.h", entry.Name(), "download artifacts must be removed")
			}
		})
	}
}

var debianKernelTarget = &types.Target{
	Distro:    types.Distro{Display: "debian"},
	OSRelease: map[string]string{"VERSION_ID": "12", "VERSION_CODENAME": "bookworm"},
	Uname:     types.Utsname{Machine: "x86_64", Version: "#1 SMP PREEMPT_DYNAMIC Debian 6.1.148-1 (2025-08-26)"},
}

func TestOverrideRepoURI(t *testing.T) {
	debian := func(versionID string) *types.Target {
		return &types.Target{Distro: types.Distro{Display: "debian"}, OSRelease: map[string]string{"VERSION_ID": versionID}}
	}
	ubuntu := &types.Target{Distro: types.Distro{Display: "ubuntu"}, OSRelease: map[string]string{"VERSION_ID": "10"}}
	for _, tc := range []struct {
		uri, expected string
		target        *types.Target
	}{
		{"http://deb.debian.org/debian", "http://archive.debian.org/debian", debian("10")},
		{"http://security.debian.org/debian-security", "http://archive.debian.org/debian-security", debian("9")},
		{"http://deb.debian.org/debian", "http://deb.debian.org/debian", debian("12")},
		{"https://download.docker.com/linux/debian", "https://download.docker.com/linux/debian", debian("10")},
		{"http://deb.debian.org/debian", "http://deb.debian.org/debian", ubuntu},
	} {
		require.Equal(t, tc.expected, overrideRepoURI(tc.uri, tc.target), tc.uri)
	}
}

const snapshotURIFormat = "%s://snapshot.debian.org/archive/%s/20250909T000000Z/"

func TestGetSnapshotRepoURI(t *testing.T) {
	debian := debianKernelTarget
	ubuntu := &types.Target{Distro: types.Distro{Display: "ubuntu"}, Uname: debian.Uname}
	unversioned := &types.Target{Distro: debian.Distro, Uname: types.Utsname{Version: "#1 SMP"}}
	for _, tc := range []struct {
		uri, expected string
		target        *types.Target
	}{
		{"http://deb.debian.org/debian", fmt.Sprintf(snapshotURIFormat, "http", "debian"), debian},
		{"https://security.debian.org/debian-security/", fmt.Sprintf(snapshotURIFormat, "https", "debian-security"), debian},
		{"https://download.docker.com/linux/debian", "", debian},
		{"http://deb.debian.org/debian", "", ubuntu},
		{"http://deb.debian.org/debian", "", unversioned},
	} {
		require.Equal(t, tc.expected, getSnapshotRepoURI(tc.uri, tc.target), tc.uri)
	}
}

func TestNewBackendAppendsSnapshotRepos(t *testing.T) {
	repo := func(id, uri, distribution string) remoteRepo {
		return remoteRepo{repoID: id, uri: uri, distribution: distribution, components: []string{"main"}, arch: "amd64"}
	}
	debianSnapshot := fmt.Sprintf(snapshotURIFormat, "http", "debian")
	securitySnapshot := fmt.Sprintf(snapshotURIFormat, "http", "debian-security")
	buster := &types.Target{
		Distro:    debianKernelTarget.Distro,
		OSRelease: map[string]string{"VERSION_ID": "10", "VERSION_CODENAME": "buster"},
		Uname:     debianKernelTarget.Uname,
	}
	sid := &types.Target{
		Distro:    debianKernelTarget.Distro,
		OSRelease: map[string]string{"VERSION_CODENAME": "forky"},
		Uname:     debianKernelTarget.Uname,
	}
	for _, tc := range []struct {
		sources  string
		target   *types.Target
		expected []remoteRepo
	}{{
		sources: "deb http://deb.debian.org/debian bookworm main\n" +
			"deb http://deb.debian.org/debian-security bookworm-security main\n",
		target: debianKernelTarget,
		expected: []remoteRepo{
			repo("debian-0", "http://deb.debian.org/debian", "bookworm"),
			repo("debian-1", "http://deb.debian.org/debian-security", "bookworm-security"),
			repo("debian-0-snapshot-bookworm", debianSnapshot, "bookworm"),
			repo("debian-0-snapshot-bookworm-proposed-updates", debianSnapshot, "bookworm-proposed-updates"),
			repo("debian-1-snapshot-bookworm-security", securitySnapshot, "bookworm-security"),
		},
	}, {
		// the archive.debian.org copies only keep the final kernels of each release
		sources: "deb http://deb.debian.org/debian buster main\n" +
			"deb http://security.debian.org/debian-security buster/updates main\n",
		target: buster,
		expected: []remoteRepo{
			repo("debian-0", "http://archive.debian.org/debian", "buster"),
			repo("debian-1", "http://archive.debian.org/debian-security", "buster/updates"),
			repo("debian-0-snapshot-buster", debianSnapshot, "buster"),
			repo("debian-0-snapshot-buster-proposed-updates", debianSnapshot, "buster-proposed-updates"),
			repo("debian-1-snapshot-buster/updates", securitySnapshot, "buster/updates"),
		},
	}, {
		// at the snapshot time, oldstable designated bullseye
		sources: "deb http://deb.debian.org/debian oldstable main\n" +
			"deb http://deb.debian.org/debian-security oldstable-security main\n",
		target: debianKernelTarget,
		expected: []remoteRepo{
			repo("debian-0", "http://deb.debian.org/debian", "oldstable"),
			repo("debian-1", "http://deb.debian.org/debian-security", "oldstable-security"),
			repo("debian-0-snapshot-bookworm", debianSnapshot, "bookworm"),
			repo("debian-0-snapshot-bookworm-proposed-updates", debianSnapshot, "bookworm-proposed-updates"),
			repo("debian-1-snapshot-bookworm-security", securitySnapshot, "bookworm-security"),
		},
	}, {
		sources: "deb http://deb.debian.org/debian sid main\n",
		target:  sid,
		expected: []remoteRepo{
			repo("debian-0", "http://deb.debian.org/debian", "sid"),
		},
	}} {
		dir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(dir, "sources.list.d"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "sources.list"), []byte(tc.sources), 0644))
		backend, err := NewBackend(tc.target, dir, quietLogger{})
		require.NoError(t, err)
		require.Equal(t, tc.expected, backend.repoCollection)
	}
}
