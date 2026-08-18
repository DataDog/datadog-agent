// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package configbackupimpl

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofrs/flock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestResolveSrcDirFromConfigFile(t *testing.T) {
	cb, dir := writeConfigDir(t)
	got, err := resolveSrcDir(cb.config)
	require.NoError(t, err)
	expected, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestResolveSrcDirFallsBackToConfPath(t *testing.T) {
	cb := makeBackup(t)
	cb.config.Set("conf_path", "/tmp/ddplan-nonexistent-conf", pkgconfigmodel.SourceFile)
	got, err := resolveSrcDir(cb.config)
	require.NoError(t, err)
	assert.Equal(t, "/tmp/ddplan-nonexistent-conf", got)
}

func TestResolveSrcDirEmptyFails(t *testing.T) {
	cb := makeBackup(t)
	// ConfigFileUsed() is empty and conf_path is explicitly empty.
	cb.config.Set("conf_path", "", pkgconfigmodel.SourceFile)
	_, err := resolveSrcDir(cb.config)
	require.Error(t, err)
}

func TestResolveSrcDirEmptyDoesNotWriteToDot(t *testing.T) {
	cb := makeBackup(t)
	// Simulate the full backup path with no config file and no conf_path.
	cb.config.Set("config_backup.enabled", true, pkgconfigmodel.SourceFile)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	cb.backup()
	// Nothing must be written to the process working directory.
	_, err = os.Stat(filepath.Join(cwd, backupDirName))
	assert.True(t, os.IsNotExist(err))
}

func TestCollectFilesSelectionAndExclusions(t *testing.T) {
	cb, dir := writeConfigDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "system-probe.yaml"), []byte("system_probe_config:\n"), 0o640))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "auth_token"), []byte("secret"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ipc_cert.pem"), []byte("cert"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "install_info"), []byte("install_method:\n"), 0o644))

	confd := filepath.Join(dir, "conf.d")
	require.NoError(t, os.MkdirAll(filepath.Join(confd, "sub.d"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(confd, "foo.yaml"), []byte("init_config:\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(confd, "bar.example"), []byte("init_config:\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(confd, "baz.json"), []byte("{}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(confd, "sub.d", "conf.yaml"), []byte("init_config:\n"), 0o644))

	cb.config.Set("confd_path", confd, pkgconfigmodel.SourceFile)

	files, err := collectFiles(cb, dir)
	require.NoError(t, err)

	paths := mapArchivePaths(files)
	assert.Contains(t, paths, "datadog.yaml")
	assert.Contains(t, paths, "system-probe.yaml")
	assert.Contains(t, paths, "install_info")
	assert.NotContains(t, paths, "auth_token")
	assert.NotContains(t, paths, "ipc_cert.pem")
	assert.Contains(t, paths, filepath.ToSlash(filepath.Join("conf.d", "foo.yaml")))
	assert.Contains(t, paths, filepath.ToSlash(filepath.Join("conf.d", "baz.json")))
	assert.Contains(t, paths, filepath.ToSlash(filepath.Join("conf.d", "sub.d", "conf.yaml")))
	assert.NotContains(t, paths, filepath.ToSlash(filepath.Join("conf.d", "bar.example")))
}

func TestCollectFilesSymlinkNotReadThrough(t *testing.T) {
	cb, dir := writeConfigDir(t)
	confd := filepath.Join(dir, "conf.d")
	require.NoError(t, os.MkdirAll(confd, 0o755))
	// A symlink pointing outside the tree must be recorded as a symlink, and
	// its target content must never enter the archive.
	outside := filepath.Join(t.TempDir(), "shadow")
	require.NoError(t, os.WriteFile(outside, []byte("root:secret"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(confd, "conf.yaml")))
	cb.config.Set("confd_path", confd, pkgconfigmodel.SourceFile)

	files, err := collectFiles(cb, dir)
	require.NoError(t, err)
	var symlink *collectedFile
	for i := range files {
		if files[i].archivePath == filepath.ToSlash(filepath.Join("conf.d", "conf.yaml")) {
			symlink = &files[i]
		}
	}
	require.NotNil(t, symlink)
	assert.NotEmpty(t, symlink.linkTarget)
	assert.Nil(t, symlink.content)
}

func TestCollectFilesExternalTree(t *testing.T) {
	cb, dir := writeConfigDir(t)
	external := filepath.Join(t.TempDir(), "compliance.d")
	require.NoError(t, os.MkdirAll(external, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(external, "cis.yaml"), []byte("benchmark:\n"), 0o644))
	cb.config.Set("compliance_config.dir", external, pkgconfigmodel.SourceFile)

	files, err := collectFiles(cb, dir)
	require.NoError(t, err)
	paths := mapArchivePaths(files)
	assert.Contains(t, paths, filepath.ToSlash(filepath.Join("external", "compliance_config.dir", "cis.yaml")))
}

func TestContentAddressedDedup(t *testing.T) {
	cb, dir := writeConfigDir(t)
	files, err := collectFiles(cb, dir)
	require.NoError(t, err)
	digest, err := computeDigest(files)
	require.NoError(t, err)

	backupDir := filepath.Join(dir, backupDirName)
	require.NoError(t, os.MkdirAll(backupDir, 0o700))

	require.NoError(t, publishSnapshot(backupDir, dir, digest, files))
	// Publishing the same digest again must not create a second archive.
	require.NoError(t, publishSnapshot(backupDir, dir, digest, files))

	archives := countArchives(backupDir)
	assert.Equal(t, 1, archives)
}

func TestFlapKeepsTwoArchives(t *testing.T) {
	cb, dir := writeConfigDir(t)
	backupDir := filepath.Join(dir, backupDirName)
	require.NoError(t, os.MkdirAll(backupDir, 0o700))

	// Configuration A.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "datadog.yaml"), []byte("api_key: A\n"), 0o600))
	filesA, err := collectFiles(cb, dir)
	require.NoError(t, err)
	digestA, err := computeDigest(filesA)
	require.NoError(t, err)
	require.NoError(t, publishSnapshot(backupDir, dir, digestA, filesA))

	// Configuration B.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "datadog.yaml"), []byte("api_key: B\n"), 0o600))
	filesB, err := collectFiles(cb, dir)
	require.NoError(t, err)
	digestB, err := computeDigest(filesB)
	require.NoError(t, err)
	require.NoError(t, publishSnapshot(backupDir, dir, digestB, filesB))

	// A/B/A/B flap: four occurrences, two distinct archives.
	now := time.Now()
	records := []startRecord{
		{Timestamp: now, Digest: digestA},
		{Timestamp: now.Add(time.Second), Digest: digestB},
		{Timestamp: now.Add(2 * time.Second), Digest: digestA},
		{Timestamp: now.Add(3 * time.Second), Digest: digestB},
	}
	require.NoError(t, rewriteStartRecords(backupDir, records))

	cb.config.Set("config_backup.max_snapshots", 10, pkgconfigmodel.SourceFile)
	cb.rotate(backupDir, digestB)

	assert.Equal(t, 2, countArchives(backupDir))
	got, err := readStartRecords(backupDir)
	require.NoError(t, err)
	assert.Len(t, got, 4)
}

func TestRotationNeverEvictsCurrentOrPredecessor(t *testing.T) {
	cb, dir := writeConfigDir(t)
	backupDir := filepath.Join(dir, backupDirName)
	require.NoError(t, os.MkdirAll(backupDir, 0o700))

	// Three distinct configurations A, B, C.
	digests := map[string]string{}
	for name, key := range map[string]string{"A": "a", "B": "b", "C": "c"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "datadog.yaml"), []byte("api_key: "+key+"\n"), 0o600))
		files, err := collectFiles(cb, dir)
		require.NoError(t, err)
		digest, err := computeDigest(files)
		require.NoError(t, err)
		digests[name] = digest
		require.NoError(t, publishSnapshot(backupDir, dir, digest, files))
	}

	// Records: A, B, C, B. Current is B; most recent distinct predecessor is C.
	now := time.Now()
	records := []startRecord{
		{Timestamp: now, Digest: digests["A"]},
		{Timestamp: now.Add(time.Second), Digest: digests["B"]},
		{Timestamp: now.Add(2 * time.Second), Digest: digests["C"]},
		{Timestamp: now.Add(3 * time.Second), Digest: digests["B"]},
	}
	require.NoError(t, rewriteStartRecords(backupDir, records))

	cb.config.Set("config_backup.max_snapshots", 2, pkgconfigmodel.SourceFile)
	cb.rotate(backupDir, digests["B"])

	// With maxSnapshots=2 and protected B (current) + C (predecessor), only A
	// is evictable.
	assert.Equal(t, 2, countArchives(backupDir))
	_, errA := os.Stat(filepath.Join(backupDir, digests["A"]+archiveSuffix))
	assert.True(t, os.IsNotExist(errA))
	_, errB := os.Stat(filepath.Join(backupDir, digests["B"]+archiveSuffix))
	assert.NoError(t, errB)
	_, errC := os.Stat(filepath.Join(backupDir, digests["C"]+archiveSuffix))
	assert.NoError(t, errC)
}

func TestPermissions(t *testing.T) {
	cb, dir := writeConfigDir(t)
	files, err := collectFiles(cb, dir)
	require.NoError(t, err)
	digest, err := computeDigest(files)
	require.NoError(t, err)

	backupDir := filepath.Join(dir, backupDirName)
	require.NoError(t, os.MkdirAll(backupDir, 0o700))
	require.NoError(t, publishSnapshot(backupDir, dir, digest, files))

	info, err := os.Stat(filepath.Join(backupDir, digest+archiveSuffix))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	dirInfo, err := os.Stat(backupDir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
}

func TestStaleTmpCleanup(t *testing.T) {
	_, dir := writeConfigDir(t)
	backupDir := filepath.Join(dir, backupDirName)
	require.NoError(t, os.MkdirAll(backupDir, 0o700))
	stale := filepath.Join(backupDir, tmpPrefix+"12345-abc.tar.gz")
	require.NoError(t, os.WriteFile(stale, []byte("partial"), 0o600))
	cleanStaleTmp(backupDir)
	_, err := os.Stat(stale)
	assert.True(t, os.IsNotExist(err))
}

func TestReadOnlyBackupDir(t *testing.T) {
	cb, dir := writeConfigDir(t)
	backupDir := filepath.Join(dir, backupDirName)
	require.NoError(t, os.MkdirAll(backupDir, 0o700))
	require.NoError(t, os.Chmod(backupDir, 0o500))
	defer os.Chmod(backupDir, 0o700) //nolint:errcheck

	files, err := collectFiles(cb, dir)
	require.NoError(t, err)
	digest, err := computeDigest(files)
	require.NoError(t, err)
	// Publishing into a read-only directory must fail, not panic.
	err = publishSnapshot(backupDir, dir, digest, files)
	require.Error(t, err)
}

func TestFlushContentionSkipsRotation(t *testing.T) {
	cb, dir := writeConfigDir(t)
	backupDir := filepath.Join(dir, backupDirName)
	require.NoError(t, os.MkdirAll(backupDir, 0o700))

	lock := flock.New(filepath.Join(backupDir, lockFileName))
	held, err := lock.TryLock()
	require.NoError(t, err)
	require.True(t, held)
	defer lock.Unlock()

	// With the lock held, rotation must time out and skip, not hang.
	cb.config.Set("config_backup.max_snapshots", 10, pkgconfigmodel.SourceFile)
	done := make(chan struct{})
	runRotate := func() {
		cb.rotate(backupDir, "deadbeef")
		close(done)
	}
	go runRotate()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("rotation did not time out")
	}
}

func TestTruncatedArchiveDetected(t *testing.T) {
	cb, dir := writeConfigDir(t)
	files, err := collectFiles(cb, dir)
	require.NoError(t, err)
	digest, err := computeDigest(files)
	require.NoError(t, err)

	backupDir := filepath.Join(dir, backupDirName)
	require.NoError(t, os.MkdirAll(backupDir, 0o700))
	require.NoError(t, publishSnapshot(backupDir, dir, digest, files))

	// Truncate the archive.
	archivePath := filepath.Join(backupDir, digest+archiveSuffix)
	data, err := os.ReadFile(archivePath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(archivePath, data[:len(data)/2], 0o600))

	assert.False(t, archiveMatchesDigest(archivePath, digest))
	snapshots, err := ListSnapshots(backupDir)
	require.NoError(t, err)
	assert.Len(t, snapshots, 0)
}

func TestSafeArchiveName(t *testing.T) {
	assert.True(t, safeArchiveName("conf.d/foo.yaml"))
	assert.True(t, safeArchiveName("datadog.yaml"))
	assert.False(t, safeArchiveName("/etc/passwd"))
	assert.False(t, safeArchiveName("../../etc/passwd"))
	assert.False(t, safeArchiveName("conf.d/../../etc/passwd"))
}

func mapArchivePaths(files []collectedFile) map[string]bool {
	m := map[string]bool{}
	for _, f := range files {
		m[f.archivePath] = true
	}
	return m
}
