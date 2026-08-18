// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package configbackupimpl implements the configbackup component interface.
//
// At every core Agent start it writes a byte-exact snapshot of the
// configuration files the Agent actually loaded into a dedicated directory
// (`<confdir>/config-backups`). The snapshot is a content-addressed archive
// plus an append-only occurrence log, so it survives restart flapping without
// writing redundant bytes. It is a provenance and audit record, not a
// disaster-recovery backup: a config experiment that fails inside the config
// constructor never reaches this hook, and the installer already keeps
// `/etc/datadog-agent` untouched for the whole experiment.
package configbackupimpl

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"github.com/DataDog/datadog-agent/comp/core/config"
	configbackup "github.com/DataDog/datadog-agent/comp/core/configbackup/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// backupDirName is the name of the backup directory inside the config directory.
	backupDirName = "config-backups"
	// startsLogName is the append-only occurrence log.
	startsLogName = "starts.jsonl"
	// lockFileName is the flock target. It never holds content.
	lockFileName = ".lock"
	// archiveSuffix is the suffix of content-addressed archives.
	archiveSuffix = ".tar.gz"
	// manifestSuffix is the suffix of the sidecar manifest.
	manifestSuffix = ".manifest.json"
	// tmpPrefix is the prefix of temporary files created during atomic publish.
	tmpPrefix = ".tmp-"
	// maxFileSize is the per-file cap for files archived. Anything larger is skipped.
	maxFileSize = 10 << 20 // 10 MiB
	// lockTimeout is the maximum time we wait for the rotation lock.
	lockTimeout = 2 * time.Second
	// deploymentIDFile is the Fleet installer state file that records the
	// deployment that produced this configuration.
	deploymentIDFile = ".deployment-id"
)

// Requires defines the dependencies of the configbackup component.
type Requires struct {
	compdef.In
	Lc             compdef.Lifecycle
	Config         config.Component
	SysprobeConfig sysprobeconfig.Component
	Log            log.Component
}

type configBackup struct {
	config         config.Component
	sysprobeConfig sysprobeconfig.Component
	log            log.Component
}

// NewComponent creates a new configbackup component.
func NewComponent(deps Requires) (configbackup.Component, error) {
	cb := &configBackup{
		config:         deps.Config,
		sysprobeConfig: deps.SysprobeConfig,
		log:            deps.Log,
	}
	deps.Lc.Append(compdef.Hook{
		OnStart: func(_ context.Context) error {
			cb.backup()
			return nil
		},
	})
	return cb, nil
}

// backup writes a snapshot of the loaded configuration files. It never fails
// startup: every error is logged at warning level.
func (cb *configBackup) backup() {
	if !cb.config.GetBool("config_backup.enabled") {
		cb.log.Debugf("config backup: disabled by config_backup.enabled")
		return
	}

	srcDir, err := resolveSrcDir(cb.config)
	if err != nil {
		cb.log.Warnf("config backup: %v", err)
		return
	}
	backupDir := backupDirFromConfig(cb.config, srcDir)

	files, err := collectFiles(cb, srcDir)
	if err != nil {
		cb.log.Warnf("config backup: failed to collect configuration files: %v", err)
		return
	}

	digest, err := computeDigest(files)
	if err != nil {
		cb.log.Warnf("config backup: failed to compute configuration digest: %v", err)
		return
	}

	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		cb.log.Warnf("config backup: failed to create backup directory %q: %v", backupDir, err)
		return
	}
	// Chmod explicitly so umask cannot widen the mode.
	if err := os.Chmod(backupDir, 0o700); err != nil {
		cb.log.Warnf("config backup: failed to set permissions on backup directory %q: %v", backupDir, err)
		return
	}

	cleanStaleTmp(backupDir)

	archivePath := filepath.Join(backupDir, digest+archiveSuffix)
	if _, err := os.Stat(archivePath); os.IsNotExist(err) {
		if err := publishSnapshot(backupDir, srcDir, digest, files); err != nil {
			cb.log.Warnf("config backup: failed to publish snapshot %s: %v", digest, err)
			// Do not append the occurrence record on a failed publish: the
			// snapshot does not exist, so the occurrence would point at nothing.
			return
		}
	}

	record := startRecord{
		Timestamp:    time.Now().UTC(),
		Digest:       digest,
		IsExperiment: isExperiment(srcDir),
		DeploymentID: readDeploymentID(srcDir),
		PID:          os.Getpid(),
		AgentVersion: version.AgentVersion,
		ConfigDir:    srcDir,
	}
	if err := appendStartRecord(backupDir, record); err != nil {
		cb.log.Warnf("config backup: failed to append start record: %v", err)
	}

	cb.rotate(backupDir, digest)
}

// resolveSrcDir returns the canonical configuration directory. It derives it
// from the config file actually used, falling back to the conf_path setting.
func resolveSrcDir(cfg config.Component) (string, error) {
	if p := cfg.ConfigFileUsed(); p != "" {
		srcDir := filepath.Dir(p)
		resolved, err := filepath.EvalSymlinks(srcDir)
		if err != nil {
			return "", fmt.Errorf("failed to resolve config directory %q: %w", srcDir, err)
		}
		return resolved, nil
	}
	// No config file used (e.g. a container configured purely through DD_*
	// environment variables). Fall back to conf_path. Do not call
	// filepath.Dir(""): it returns "." and backups would land in the process
	// working directory.
	confPath := cfg.GetString("conf_path")
	if confPath == "" {
		return "", errors.New("no configuration file used and conf_path is empty, skipping backup")
	}
	if resolved, err := filepath.EvalSymlinks(confPath); err == nil {
		return resolved, nil
	}
	return confPath, nil
}

// backupDirFromConfig returns the backup directory for a resolved config
// directory. The config_backup.directory setting is an operator override;
// when empty the directory is derived from the config directory.
func backupDirFromConfig(cfg config.Component, srcDir string) string {
	if d := cfg.GetString("config_backup.directory"); d != "" {
		return d
	}
	return filepath.Join(srcDir, backupDirName)
}

// BackupDir returns the configuration backup directory for the given config,
// resolving the config directory the same way the snapshot hook does.
func BackupDir(cfg config.Component) (string, error) {
	srcDir, err := resolveSrcDir(cfg)
	if err != nil {
		return "", err
	}
	return backupDirFromConfig(cfg, srcDir), nil
}

// collectedFile is one file (or symlink) that will be archived.
type collectedFile struct {
	archivePath string // path inside the archive
	diskPath    string // absolute path on disk
	linkTarget  string // non-empty for symlinks
	size        int64
	mode        fs.FileMode
	uid         int
	gid         int
	content     []byte // nil for symlinks
	digest      string // per-file sha256 (of content, or of the link target)
}

// collectFiles builds the sorted list of files to archive.
func collectFiles(cb *configBackup, srcDir string) ([]collectedFile, error) {
	cfg := cb.config
	var files []collectedFile

	addExplicit := func(diskPath, archivePath string) {
		if diskPath == "" {
			return
		}
		if f, ok := collectSingleFile(diskPath, archivePath); ok {
			files = append(files, f)
		}
	}

	// The main config file actually used.
	if p := cfg.ConfigFileUsed(); p != "" {
		addExplicit(p, filepath.Base(p))
	}

	// Sibling config files that live next to datadog.yaml.
	for _, name := range []string{
		"system-probe.yaml",
		"security-agent.yaml",
		"otel-config.yaml",
		"application_monitoring.yaml",
		"install_info",
		deploymentIDFile,
	} {
		addExplicit(filepath.Join(srcDir, name), name)
	}

	// Extra config files loaded by the config layer.
	for _, p := range cfg.ExtraConfigFilesUsed() {
		addExplicit(p, externalPath("extra_config_files", p, srcDir))
	}

	// Trees resolved from their settings. Any tree outside srcDir is archived
	// under the external/<setting name> prefix.
	trees := []treeSource{
		{setting: "confd_path", dir: cfg.GetString("confd_path"), extFilter: configExt},
		{setting: "additional_checksd", dir: cfg.GetString("additional_checksd")},
		{setting: "runtime_security_config.policies.dir", dir: cb.sysprobeConfig.GetString("runtime_security_config.policies.dir")},
		{setting: "compliance_config.dir", dir: cfg.GetString("compliance_config.dir")},
		{setting: "fleet_policies_dir", dir: cfg.GetString("fleet_policies_dir")},
	}
	for _, tree := range trees {
		if tree.dir == "" {
			continue
		}
		prefix := archivePrefix(srcDir, tree.dir, tree.setting)
		if err := walkTree(tree.dir, prefix, tree.extFilter, &files); err != nil {
			cb.log.Warnf("config backup: failed to walk %q: %v", tree.dir, err)
		}
	}

	sort.Slice(files, func(i, j int) bool { return files[i].archivePath < files[j].archivePath })
	return files, nil
}

// treeSource describes one directory tree to archive.
type treeSource struct {
	setting   string
	dir       string
	extFilter func(string) bool
}

// configExt reports whether name should be archived from a conf.d-style tree:
// yaml/yml/json only, never .example files.
func configExt(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == ".example" {
		return false
	}
	return ext == ".yaml" || ext == ".yml" || ext == ".json"
}

// excludedNames are files that must never be archived, matched by base name.
var excludedNames = map[string]bool{
	"auth_token":   true,
	"ipc_cert.pem": true,
	backupDirName:  true,
}

// collectSingleFile reads one explicit file. It returns ok=false when the file
// does not exist or is excluded.
func collectSingleFile(diskPath, archivePath string) (collectedFile, bool) {
	if excludedNames[filepath.Base(diskPath)] {
		return collectedFile{}, false
	}
	info, err := os.Lstat(diskPath)
	if err != nil {
		return collectedFile{}, false
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		target, err := os.Readlink(diskPath)
		if err != nil {
			return collectedFile{}, false
		}
		return collectedFile{
			archivePath: archivePath,
			diskPath:    diskPath,
			linkTarget:  target,
			mode:        info.Mode(),
			digest:      digestOf([]byte("link:" + target)),
		}, true
	}
	if !info.Mode().IsRegular() {
		return collectedFile{}, false
	}
	if info.Size() > maxFileSize {
		return collectedFile{}, false
	}
	content, err := os.ReadFile(diskPath)
	if err != nil {
		return collectedFile{}, false
	}
	uid, gid := fileOwnership(info)
	return collectedFile{
		archivePath: archivePath,
		diskPath:    diskPath,
		size:        int64(len(content)),
		mode:        info.Mode(),
		uid:         uid,
		gid:         gid,
		content:     content,
		digest:      digestOf(content),
	}, true
}

// walkTree walks a directory tree, appending regular files and symlinks.
// Symlinks are recorded with their link target and never read through, so a
// symlink pointing outside the tree cannot pull foreign content into the
// archive. Devices, FIFOs and sockets are skipped.
func walkTree(root, prefix string, extFilter func(string) bool, out *[]collectedFile) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if excludedNames[filepath.Base(path)] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			*out = append(*out, collectedFile{
				archivePath: filepath.Join(prefix, rel),
				diskPath:    path,
				linkTarget:  target,
				mode:        info.Mode(),
				digest:      digestOf([]byte("link:" + target)),
			})
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if extFilter != nil && !extFilter(rel) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxFileSize {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		uid, gid := fileOwnership(info)
		*out = append(*out, collectedFile{
			archivePath: filepath.Join(prefix, rel),
			diskPath:    path,
			size:        int64(len(content)),
			mode:        info.Mode(),
			uid:         uid,
			gid:         gid,
			content:     content,
			digest:      digestOf(content),
		})
		return nil
	})
}

// externalPath returns the archive path for a file that lives outside srcDir.
// Files inside srcDir use their relative path; files outside use the full path
// under external/<setting> so distinct files cannot collide.
func externalPath(setting, diskPath, srcDir string) string {
	if rel, err := filepath.Rel(srcDir, diskPath); err == nil && !isOutside(rel) {
		return filepath.ToSlash(rel)
	}
	trimmed := strings.TrimPrefix(filepath.Clean(diskPath), string(filepath.Separator))
	return filepath.ToSlash(filepath.Join("external", setting, trimmed))
}

// archivePrefix returns the archive prefix for a tree. Trees inside srcDir use
// their relative path; trees outside use external/<setting name>.
func archivePrefix(srcDir, treeDir, setting string) string {
	canonicalSrc := canonicalPath(srcDir)
	canonicalTree := canonicalPath(treeDir)
	rel, err := filepath.Rel(canonicalSrc, canonicalTree)
	if err != nil || isOutside(rel) {
		return filepath.ToSlash(filepath.Join("external", setting))
	}
	return filepath.ToSlash(rel)
}

// isOutside reports whether rel escapes the base directory.
func isOutside(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// canonicalPath resolves symlinks, falling back to the raw path on error.
func canonicalPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// computeDigest computes the content-addressed digest over the sorted
// (archive path, content) pairs.
func computeDigest(files []collectedFile) (string, error) {
	h := sha256.New()
	for _, f := range files {
		h.Write([]byte(f.archivePath))
		h.Write([]byte{0})
		if f.linkTarget != "" {
			h.Write([]byte("link:"))
			h.Write([]byte(f.linkTarget))
		} else {
			if _, err := h.Write(f.content); err != nil {
				return "", err
			}
		}
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func digestOf(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// publishSnapshot writes the archive and its manifest atomically. The manifest
// is renamed before the archive, so a visible archive always has a manifest.
func publishSnapshot(backupDir, srcDir, digest string, files []collectedFile) error {
	manifest := buildManifest(srcDir, digest, files)

	// Write and rename the manifest first.
	if err := writeManifestAtomic(backupDir, digest, manifest); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	// Write the archive to a temp file, then rename it into place.
	tmp, err := os.CreateTemp(backupDir, tmpPrefix+"*.tar.gz")
	if err != nil {
		return fmt.Errorf("failed to create temp archive: %w", err)
	}
	tmpName := tmp.Name()
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("failed to set temp archive mode: %w", err)
	}
	if err := writeTarGz(tmp, files); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("failed to write archive: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("failed to close archive: %w", err)
	}
	if err := os.Rename(tmpName, filepath.Join(backupDir, digest+archiveSuffix)); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("failed to rename archive into place: %w", err)
	}
	// Make the rename itself durable.
	if dir, err := os.Open(backupDir); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

// writeTarGz writes the archive to w. The tar writer is closed before the gzip
// writer and both errors are checked.
func writeTarGz(w io.Writer, files []collectedFile) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	for _, f := range files {
		hdr := &tar.Header{
			Name: f.archivePath,
			Mode: int64(f.mode.Perm()),
			Size: f.size,
		}
		if f.linkTarget != "" {
			hdr.Typeflag = tar.TypeSymlink
			hdr.Linkname = f.linkTarget
			hdr.Size = 0
		} else {
			hdr.Typeflag = tar.TypeReg
		}
		if err := tw.WriteHeader(hdr); err != nil {
			_ = tw.Close()
			_ = gz.Close()
			return err
		}
		if f.linkTarget == "" {
			if _, err := tw.Write(f.content); err != nil {
				_ = tw.Close()
				_ = gz.Close()
				return err
			}
		}
	}
	if err := tw.Close(); err != nil {
		_ = gz.Close()
		return err
	}
	return gz.Close()
}

// manifest describes a single configuration snapshot.
type Manifest struct {
	Timestamp    time.Time      `json:"timestamp"`
	Digest       string         `json:"digest"`
	ConfigDir    string         `json:"config_dir"`
	IsExperiment bool           `json:"is_experiment"`
	DeploymentID string         `json:"deployment_id,omitempty"`
	AgentVersion string         `json:"agent_version"`
	Hostname     string         `json:"hostname"`
	Files        []ManifestFile `json:"files"`
}

type ManifestFile struct {
	Path   string      `json:"path"`
	Size   int64       `json:"size"`
	Mode   fs.FileMode `json:"mode"`
	UID    int         `json:"uid,omitempty"`
	GID    int         `json:"gid,omitempty"`
	Digest string      `json:"digest"`
}

func buildManifest(srcDir, digest string, files []collectedFile) Manifest {
	host, err := hostname.Get(context.Background())
	if err != nil {
		host = "unknown"
	}
	m := Manifest{
		Timestamp:    time.Now().UTC(),
		Digest:       digest,
		ConfigDir:    srcDir,
		IsExperiment: isExperiment(srcDir),
		DeploymentID: readDeploymentID(srcDir),
		AgentVersion: version.AgentVersion,
		Hostname:     host,
		Files:        make([]ManifestFile, 0, len(files)),
	}
	for _, f := range files {
		m.Files = append(m.Files, ManifestFile{
			Path:   f.archivePath,
			Size:   f.size,
			Mode:   f.mode,
			UID:    f.uid,
			GID:    f.gid,
			Digest: f.digest,
		})
	}
	return m
}

// writeManifestAtomic writes the manifest to a temp file and renames it.
func writeManifestAtomic(backupDir, digest string, m Manifest) error {
	tmp, err := os.CreateTemp(backupDir, tmpPrefix+"*.manifest.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	enc := json.NewEncoder(tmp)
	if err := enc.Encode(m); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, filepath.Join(backupDir, digest+manifestSuffix))
}

// startRecord is one line of the occurrence log.
type startRecord struct {
	Timestamp    time.Time `json:"ts"`
	Digest       string    `json:"digest"`
	IsExperiment bool      `json:"is_experiment"`
	DeploymentID string    `json:"deployment_id,omitempty"`
	PID          int       `json:"pid"`
	AgentVersion string    `json:"agent_version"`
	ConfigDir    string    `json:"config_dir"`
}

// appendStartRecord appends one line to starts.jsonl with O_APPEND in a single
// write. The file is created with mode 0600.
func appendStartRecord(backupDir string, record startRecord) error {
	line, err := json.Marshal(record)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	f, err := os.OpenFile(filepath.Join(backupDir, startsLogName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	_, err = f.Write(line)
	return err
}

// cleanStaleTmp removes any leftover .tmp-* file from a previous SIGKILL.
func cleanStaleTmp(backupDir string) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), tmpPrefix) {
			_ = os.Remove(filepath.Join(backupDir, e.Name()))
		}
	}
}

// rotate trims the occurrence log and evicts old snapshots under an exclusive
// file lock. On lock timeout it skips rotation only — never the snapshot.
func (cb *configBackup) rotate(backupDir, currentDigest string) {
	lock := flock.New(filepath.Join(backupDir, lockFileName))
	ctx, cancel := context.WithTimeout(context.Background(), lockTimeout)
	defer cancel()
	locked, err := lock.TryLockContext(ctx, 100*time.Millisecond)
	if err != nil {
		cb.log.Warnf("config backup: failed to acquire rotation lock: %v", err)
		return
	}
	if !locked {
		cb.log.Warnf("config backup: timed out waiting for rotation lock, skipping rotation")
		return
	}
	defer func() { _ = lock.Unlock() }()

	maxSnapshots := cb.config.GetInt("config_backup.max_snapshots")
	if maxSnapshots <= 0 {
		maxSnapshots = 10
	}
	maxStarts := cb.config.GetInt("config_backup.max_starts_logged")
	if maxStarts <= 0 {
		maxStarts = 1000
	}

	records, err := readStartRecords(backupDir)
	if err != nil {
		cb.log.Warnf("config backup: failed to read start log for rotation: %v", err)
		return
	}

	// Trim the occurrence log to the last maxStarts lines.
	if len(records) > maxStarts {
		records = records[len(records)-maxStarts:]
		if err := rewriteStartRecords(backupDir, records); err != nil {
			cb.log.Warnf("config backup: failed to trim start log: %v", err)
		}
	}

	evictSnapshots(backupDir, records, currentDigest, maxSnapshots)
}

// evictSnapshots removes the oldest snapshots beyond maxSnapshots, by
// last-seen time from the occurrence log. It never evicts the current
// configuration or its most recent distinct predecessor.
func evictSnapshots(backupDir string, records []startRecord, currentDigest string, maxSnapshots int) {
	lastSeen := map[string]time.Time{}
	var order []string
	for _, r := range records {
		if _, ok := lastSeen[r.Digest]; !ok {
			order = append(order, r.Digest)
		}
		lastSeen[r.Digest] = r.Timestamp
	}

	protected := map[string]bool{currentDigest: true}
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].Digest != currentDigest {
			protected[records[i].Digest] = true
			break
		}
	}

	type candidate struct {
		digest   string
		lastSeen time.Time
	}
	var candidates []candidate
	for _, digest := range order {
		if protected[digest] {
			continue
		}
		if _, err := os.Stat(filepath.Join(backupDir, digest+archiveSuffix)); err == nil {
			candidates = append(candidates, candidate{digest: digest, lastSeen: lastSeen[digest]})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].lastSeen.Before(candidates[j].lastSeen) })

	total := countArchives(backupDir)
	toEvict := total - maxSnapshots
	if toEvict <= 0 {
		return
	}
	for i := 0; i < toEvict && i < len(candidates); i++ {
		_ = os.Remove(filepath.Join(backupDir, candidates[i].digest+archiveSuffix))
		_ = os.Remove(filepath.Join(backupDir, candidates[i].digest+manifestSuffix))
	}
}

func countArchives(backupDir string) int {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), archiveSuffix) {
			n++
		}
	}
	return n
}

// readStartRecords reads all occurrence records, oldest first.
func readStartRecords(backupDir string) ([]startRecord, error) {
	data, err := os.ReadFile(filepath.Join(backupDir, startsLogName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var records []startRecord
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var r startRecord
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			continue
		}
		records = append(records, r)
	}
	return records, nil
}

// rewriteStartRecords rewrites starts.jsonl with the given records.
func rewriteStartRecords(backupDir string, records []startRecord) error {
	tmp, err := os.CreateTemp(backupDir, tmpPrefix+"*.jsonl")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	enc := json.NewEncoder(tmp)
	for _, r := range records {
		if err := enc.Encode(r); err != nil {
			tmp.Close()
			os.Remove(tmpName)
			return err
		}
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, filepath.Join(backupDir, startsLogName))
}
