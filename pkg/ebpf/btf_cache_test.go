// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package ebpf

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	noopsimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl/noops"
	"github.com/stretchr/testify/require"
)

func requireCacheRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("requires root to exercise cache ownership and repair")
	}
}

func embeddedCacheLoader(t *testing.T, compressed bool) (*orderedBTFLoader, string, string) {
	t.Helper()
	requireCacheRoot(t)
	_, cache := stickyParent(t)
	cfg := &Config{BTFOutputDir: filepath.Join(filepath.Dir(cache), "btf"), EnableRuntimeCompiler: false}
	b, err := initBTFLoader(cfg, nil, noopsimpl.NewComponent())
	require.NoError(t, err)
	t.Cleanup(b.Flush)
	b.platform, b.platformVersion, b.kernelVersion = platformUbuntu, "22.04", "test-kernel"
	b.embeddedDir = t.TempDir()
	collection := t.TempDir()
	rel := relativeBTFTarballPath(b.platform, b.platformVersion, b.kernelVersion+".btf")
	if compressed {
		rel += ".tar.xz"
	}
	dst := filepath.Join(collection, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0700))
	if compressed {
		archive, _ := setupBTFTarXZFile(t, b.kernelVersion)
		copyFile(t, archive, dst)
	} else {
		copyFile(t, "testdata/rc-btf-test.btf", dst)
	}
	cmd := exec.Command("tar", "-cJf", filepath.Join(b.embeddedDir, btfArchiveName), rel)
	cmd.Dir = collection
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return b, filepath.Join(b.btfOutputDir, rel), filepath.Join(b.btfOutputDir, filepath.Dir(rel), b.kernelVersion)
}

func TestEmbeddedBTFCacheRepair(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		name := "raw"
		if compressed {
			name = "compressed"
		}
		t.Run(name, func(t *testing.T) {
			b, intermediate, extract := embeddedCacheLoader(t, compressed)
			require.NoError(t, os.MkdirAll(extract, 0777))
			require.NoError(t, os.WriteFile(intermediate, []byte("untrusted intermediate"), 0666))
			victim := filepath.Join(t.TempDir(), "victim")
			require.NoError(t, os.WriteFile(victim, []byte("unchanged"), 0600))
			require.NoError(t, os.Symlink(victim, filepath.Join(extract, b.kernelVersion+".btf")))
			require.NoError(t, os.WriteFile(filepath.Join(extract, "vmlinux"), []byte("untrusted cached BTF"), 0666))
			require.NoError(t, os.Chown(filepath.Dir(b.btfOutputDir), 65534, 65534))
			ret, err := b.loadEmbedded(context.Background())
			require.NoError(t, err)
			require.NotNil(t, ret.vmlinux)
			data, err := os.ReadFile(victim)
			require.NoError(t, err)
			require.Equal(t, "unchanged", string(data))
			// Cached BTF remains usable without the collection or intermediate file.
			require.NoError(t, os.Remove(intermediate))
			require.NoError(t, os.Remove(filepath.Join(b.embeddedDir, btfArchiveName)))
			// checkforBTF tests reuse independently of collection discovery.
			ret, err = b.checkforBTF(mustRelative(t, b.btfOutputDir, extract))
			require.NoError(t, err)
			require.NotNil(t, ret.vmlinux)
			require.NoError(t, b.secureBTFCache(filepath.Dir(intermediate), extract))
		})
	}
}

func mustRelative(t *testing.T, base, dir string) string {
	t.Helper()
	rel, err := filepath.Rel(base, dir)
	require.NoError(t, err)
	return rel
}

func TestEmbeddedBTFUnsafeFileParents(t *testing.T) {
	for _, parent := range []string{"root", "version", "intermediate", "extraction"} {
		for _, mode := range []os.FileMode{0770, 0777, 0777 | os.ModeSticky} {
			t.Run(parent+"/"+mode.String(), func(t *testing.T) {
				b, intermediate, extract := embeddedCacheLoader(t, false)
				require.NoError(t, b.secureBTFCache(filepath.Dir(intermediate), extract))
				dirs := map[string]string{"root": filepath.Dir(b.btfOutputDir), "version": b.btfOutputDir, "intermediate": filepath.Dir(intermediate), "extraction": extract}
				unsafe := dirs[parent]
				require.NoError(t, os.WriteFile(filepath.Join(unsafe, "planted"), []byte("untrusted"), 0600))
				require.NoError(t, os.Chmod(unsafe, mode))
				ret, err := b.loadEmbedded(context.Background())
				require.NoError(t, err)
				require.NotNil(t, ret.vmlinux)
				_, err = os.Stat(filepath.Join(unsafe, "planted"))
				require.True(t, os.IsNotExist(err))
				require.Zero(t, dirUID(t, unsafe))
			})
		}
	}
}

func TestBTFCacheDirectorySymlink(t *testing.T) {
	b, _, extract := embeddedCacheLoader(t, false)
	require.NoError(t, os.MkdirAll(filepath.Dir(extract), 0700))
	target := t.TempDir()
	require.NoError(t, os.Symlink(target, extract))
	_, err := b.loadEmbedded(context.Background())
	require.ErrorContains(t, err, "is a symlink")
	entries, err := os.ReadDir(target)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestBTFWithoutCacheAccess(t *testing.T) {
	b, err := initBTFLoader(&Config{BTFPath: "testdata/rc-btf-test.btf", BTFOutputDir: "/proc/forbidden-btf-cache"}, nil, noopsimpl.NewComponent())
	require.NoError(t, err)
	t.Cleanup(b.Flush)
	ret, _, err := b.Get()
	require.NoError(t, err)
	require.NotNil(t, ret.vmlinux)
	// Native loading also never touches the cache.
	ret, err = b.loadKernel(context.Background())
	if err != nil {
		t.Skipf("native kernel BTF unavailable: %v", err)
	}
	require.NotNil(t, ret.vmlinux)
}

func TestUnprivilegedCannotPlantAfterCacheRepair(t *testing.T) {
	requireCacheRoot(t)
	// Use a direct child of /tmp so test-runner directories cannot make the
	// permission check pass merely by preventing traversal to the cache.
	base, err := os.MkdirTemp("/tmp", "btf-cache-permissions-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(base)) })
	require.NoError(t, os.Chmod(base, 0755))
	dir := filepath.Join(base, "datadog-agent", "btf")
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.Chmod(dir, 0777))
	require.NoError(t, os.Chown(filepath.Join(base, "datadog-agent"), 65534, 65534))
	plant := func() ([]byte, error) {
		cmd := exec.Command("/bin/sh", "-c", "printf planted > \"$1/planted\"", "sh", dir)
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 65534, Gid: 65534}}
		return cmd.CombinedOutput()
	}
	out, err := plant()
	require.NoError(t, err, string(out))
	require.NoError(t, SecureCacheDir("BTF cache", dir))
	out, err = plant()
	require.Error(t, err, string(out))
	_, err = os.Stat(filepath.Join(dir, "planted"))
	require.True(t, os.IsNotExist(err))
}

func TestRemoteBTFCacheRepair(t *testing.T) {
	requireCacheRoot(t)
	serverURL, hash := setupBTFServer(t)
	_, cache := stickyParent(t)
	b, err := initBTFLoader(&Config{BTFOutputDir: cache, RemoteConfigBTFDownloadHost: serverURL}, nil, noopsimpl.NewComponent())
	require.NoError(t, err)
	t.Cleanup(b.Flush)
	r := &rcBTFLoader{b: b, ctx: context.Background(), platform: platformUbuntu, platformVersion: "22.04", kernelVersion: b.kernelVersion, arch: b.arch}
	rel := relativeBTFTarballPath(r.platform, r.platformVersion, r.kernelVersion)
	extract := filepath.Join(b.btfOutputDir, filepath.Dir(rel), r.kernelVersion)
	require.NoError(t, b.secureBTFCache(extract))
	victim := filepath.Join(t.TempDir(), "victim")
	require.NoError(t, os.WriteFile(victim, []byte("unchanged"), 0600))
	require.NoError(t, os.Symlink(victim, filepath.Join(extract, r.kernelVersion+".btf")))
	require.NoError(t, os.Chmod(extract, 0777|os.ModeSticky))
	ret, err := r.processEntry(&btfEntry{SHA256: hash})
	require.NoError(t, err)
	require.NotNil(t, ret.vmlinux)
	contents, err := os.ReadFile(victim)
	require.NoError(t, err)
	require.Equal(t, "unchanged", string(contents))
	require.NoError(t, os.RemoveAll(extract))
	require.NoError(t, os.Symlink(t.TempDir(), extract))
	_, err = r.processEntry(&btfEntry{SHA256: hash})
	require.ErrorIs(t, err, errBTFExtract)
	require.ErrorContains(t, err, "is a symlink")
}
