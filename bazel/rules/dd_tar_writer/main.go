// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package main implements dd_tar_writer: a Bazel-compatible tar archive builder
// that produces a sidecar MD5 checksum manifest alongside the tar output.
//
// It is a drop-in replacement for rules_pkg's build_tar.py. It accepts the same
// CLI flags and the same manifest JSON format, and adds one flag:
//
//	--md5sums_output  path to write the MD5 sidecar file
//
// See REQUIREMENTS.md for the full specification.
package main

import (
	"archive/tar"
	"bufio"
	"compress/bzip2"
	"compress/gzip"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ulikunitz/xz"
)

// portableMtime is the deterministic timestamp used for reproducible builds.
// 2000-01-01 00:00:00.000 UTC — matches rules_pkg's PORTABLE_MTIME constant.
const portableMtime int64 = 946684800

// parseMode parses an octal mode string, accepting both "0o755" (Python-style)
// and "0755" / "755" (C-style) representations.
func parseMode(s string) (uint64, error) {
	s = strings.TrimPrefix(strings.ToLower(s), "0o")
	return strconv.ParseUint(s, 8, 32)
}

// Entry type constants — must stay in sync with manifest.py in rules_pkg.
const (
	entryIsFile      = "file"
	entryIsLink      = "symlink"
	entryIsRawLink   = "raw_symlink"
	entryIsDir       = "dir"
	entryIsTree      = "tree"
	entryIsEmptyFile = "empty-file"
)

// ManifestEntry matches one JSON record in a rules_pkg manifest file.
type ManifestEntry struct {
	Type       string `json:"type"`
	Dest       string `json:"dest"`
	Src        string `json:"src"`
	Mode       string `json:"mode"` // octal string, may be empty
	User       string `json:"user"`
	Group      string `json:"group"`
	UID        *int   `json:"uid"`
	GID        *int   `json:"gid"`
	Origin     string `json:"origin,omitempty"`
	Repository string `json:"repository,omitempty"`
}

// multiFlag is a flag.Value that accumulates repeated --flag occurrences.
type multiFlag []string

func (f *multiFlag) String() string { return strings.Join(*f, ", ") }
func (f *multiFlag) Set(v string) error {
	*f = append(*f, v)
	return nil
}

// Config holds all parsed CLI options.
type Config struct {
	Output            string
	ManifestPath      string
	DefaultMode       os.FileMode // meaningful only when HasDefaultMode is true
	HasDefaultMode    bool
	DefaultMtime      int64
	Tars              []string
	Debs              []string
	Directory         string // archive-path prefix, without slashes
	Compression       string // "", "gz", "bz2", "xz"
	Compressor        string // external command, e.g. "pigz -p 4"
	ModeMap           map[string]os.FileMode
	IDsMap            map[string][2]int
	NamesMap          map[string][2]string
	DefaultIDs        [2]int
	DefaultNames      [2]string
	CreateParents     bool
	AllowDupsFromDeps bool
	PreserveMode      bool
	PreserveMtime     bool
	CompressionLevel  int
	Md5sumsOutput     string
}

// md5Entry records one file's checksum for the sidecar file.
type md5Entry struct {
	archivePath string
	hexSum      string
}

// TarWriter builds a tar archive and accumulates MD5 checksums of regular files.
type TarWriter struct {
	cfg     Config
	outFile *os.File
	tw      *tar.Writer
	closers []io.Closer        // closed in order after tw is closed
	written map[string]byte    // archive path (no trailing /) → tar Typeflag
	md5s    []md5Entry
}

// — — — entry point — — —

func main() {
	expanded, err := expandArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "dd_tar_writer: %v\n", err)
		os.Exit(1)
	}
	cfg, err := parseFlags(expanded)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dd_tar_writer: %v\n", err)
		os.Exit(1)
	}
	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "dd_tar_writer: %v\n", err)
		os.Exit(1)
	}
}

// expandArgs expands @filename response-file arguments (same semantics as
// Python argparse fromfile_prefix_chars='@'). Each line in the file becomes
// one argument; blank lines are skipped.
func expandArgs(args []string) ([]string, error) {
	var out []string
	for _, arg := range args {
		if !strings.HasPrefix(arg, "@") {
			out = append(out, arg)
			continue
		}
		data, err := os.ReadFile(arg[1:])
		if err != nil {
			return nil, fmt.Errorf("reading param file %s: %w", arg[1:], err)
		}
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				out = append(out, line)
			}
		}
	}
	return out, nil
}

// parseFlags parses the CLI flags into a Config.
func parseFlags(args []string) (Config, error) {
	fset := flag.NewFlagSet("dd_tar_writer", flag.ContinueOnError)

	output := fset.String("output", "", "The output file, mandatory.")
	manifestPath := fset.String("manifest", "", "Manifest of contents to add to the layer.")
	modeStr := fset.String("mode", "", "Force the mode on added files (octal).")
	mtimeStr := fset.String("mtime", "", "Set mtime on tar entries: integer or 'portable'.")
	directory := fset.String("directory", "", "Directory prefix for all archive paths.")
	compression := fset.String("compression", "", "Compression: gz, bz2, or xz.")
	compressor := fset.String("compressor", "", "External compressor command, e.g. 'pigz -p 4'.")
	owner := fset.String("owner", "0.0", "Default numeric owner uid.gid.")
	ownerName := fset.String("owner_name", "", "Default owner name user.group.")
	stampFrom := fset.String("stamp_from", "", "File containing BUILD_TIMESTAMP.")
	createParents := fset.Bool("create_parents", false, "Auto-create implied parent directories.")
	allowDups := fset.Bool("allow_dups_from_deps", false, "Allow duplicate paths from deps.")
	preserveMode := fset.Bool("preserve_mode", false, "Preserve original file permissions.")
	preserveMtime := fset.Bool("preserve_mtime", false, "Preserve original file mtime.")
	compressionLevel := fset.Int("compression_level", -1, "Compression level (0–9 or -1 for default).")
	md5sumsOutput := fset.String("md5sums_output", "", "Path to write the MD5 sidecar file.")

	var tars, debs, modes, owners, ownerNames multiFlag
	fset.Var(&tars, "tar", "A tar file to merge (repeatable).")
	fset.Var(&debs, "deb", "A deb file to merge (repeatable).")
	fset.Var(&modes, "modes", "Per-file mode: path=0755 (repeatable).")
	fset.Var(&owners, "owners", "Per-file numeric owner: path=0.0 (repeatable).")
	fset.Var(&ownerNames, "owner_names", "Per-file owner name: path=root.root (repeatable).")

	if err := fset.Parse(args); err != nil {
		return Config{}, err
	}
	if *output == "" {
		return Config{}, fmt.Errorf("--output is required")
	}

	// --directory may be "@path/to/file" (package_dir_file indirection).
	directoryValue := *directory
	if strings.HasPrefix(directoryValue, "@") {
		data, err := os.ReadFile(directoryValue[1:])
		if err != nil {
			return Config{}, fmt.Errorf("reading directory file %s: %w", directoryValue[1:], err)
		}
		directoryValue = strings.TrimSpace(string(data))
	}

	cfg := Config{
		Output:            *output,
		ManifestPath:      *manifestPath,
		Tars:              []string(tars),
		Debs:              []string(debs),
		Directory:         strings.Trim(directoryValue, "/"),
		Compression:       strings.ToLower(*compression),
		Compressor:        *compressor,
		CreateParents:     *createParents,
		AllowDupsFromDeps: *allowDups,
		PreserveMode:      *preserveMode,
		PreserveMtime:     *preserveMtime,
		CompressionLevel:  *compressionLevel,
		Md5sumsOutput:     *md5sumsOutput,
	}

	// Default file mode (optional).
	if *modeStr != "" {
		m, err := parseMode(*modeStr)
		if err != nil {
			return Config{}, fmt.Errorf("invalid --mode %q: %w", *modeStr, err)
		}
		cfg.DefaultMode = os.FileMode(m)
		cfg.HasDefaultMode = true
	}

	// Default mtime.
	switch *mtimeStr {
	case "portable":
		cfg.DefaultMtime = portableMtime
	case "":
		cfg.DefaultMtime = 0
	default:
		t, err := strconv.ParseInt(*mtimeStr, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("invalid --mtime %q: %w", *mtimeStr, err)
		}
		cfg.DefaultMtime = t
	}
	if *stampFrom != "" {
		ts, err := readTimestampFromStampFile(*stampFrom)
		if err != nil {
			return Config{}, fmt.Errorf("reading stamp file: %w", err)
		}
		cfg.DefaultMtime = ts
	}

	// Default numeric owner (uid.gid).
	uidgid := strings.SplitN(*owner, ".", 2)
	if len(uidgid) != 2 {
		return Config{}, fmt.Errorf("invalid --owner %q: expected uid.gid", *owner)
	}
	uid, err := strconv.Atoi(uidgid[0])
	if err != nil {
		return Config{}, fmt.Errorf("invalid --owner uid: %w", err)
	}
	gid, err := strconv.Atoi(uidgid[1])
	if err != nil {
		return Config{}, fmt.Errorf("invalid --owner gid: %w", err)
	}
	cfg.DefaultIDs = [2]int{uid, gid}

	// Default owner name.
	if *ownerName != "" {
		parts := strings.SplitN(*ownerName, ".", 2)
		if len(parts) != 2 {
			return Config{}, fmt.Errorf("invalid --owner_name %q: expected user.group", *ownerName)
		}
		cfg.DefaultNames = [2]string{parts[0], parts[1]}
	}

	// Per-file mode overrides.
	cfg.ModeMap = make(map[string]os.FileMode)
	for _, entry := range modes {
		k, v, ok := strings.Cut(entry, "=")
		if !ok {
			return Config{}, fmt.Errorf("invalid --modes entry %q: expected path=octal", entry)
		}
		k = strings.TrimPrefix(k, "/")
		m, err := parseMode(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid mode in --modes %q: %w", entry, err)
		}
		cfg.ModeMap[k] = os.FileMode(m)
	}

	// Per-file numeric owner overrides.
	cfg.IDsMap = make(map[string][2]int)
	for _, entry := range owners {
		k, v, ok := strings.Cut(entry, "=")
		if !ok {
			return Config{}, fmt.Errorf("invalid --owners entry %q: expected path=uid.gid", entry)
		}
		k = strings.TrimPrefix(k, "/")
		p := strings.SplitN(v, ".", 2)
		if len(p) != 2 {
			return Config{}, fmt.Errorf("invalid owner in --owners %q", entry)
		}
		u, _ := strconv.Atoi(p[0])
		g, _ := strconv.Atoi(p[1])
		cfg.IDsMap[k] = [2]int{u, g}
	}

	// Per-file owner name overrides.
	cfg.NamesMap = make(map[string][2]string)
	for _, entry := range ownerNames {
		k, v, ok := strings.Cut(entry, "=")
		if !ok {
			return Config{}, fmt.Errorf("invalid --owner_names entry %q: expected path=user.group", entry)
		}
		k = strings.TrimPrefix(k, "/")
		p := strings.SplitN(v, ".", 2)
		if len(p) != 2 {
			return Config{}, fmt.Errorf("invalid owner_name in --owner_names %q", entry)
		}
		cfg.NamesMap[k] = [2]string{p[0], p[1]}
	}

	return cfg, nil
}

// run is the core logic, separated from main() for testability.
func run(cfg Config) (err error) {
	tw, err := newTarWriter(cfg)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := tw.close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	// Process manifest entries.
	if cfg.ManifestPath != "" {
		entries, err := readManifest(cfg.ManifestPath)
		if err != nil {
			return fmt.Errorf("reading manifest: %w", err)
		}
		for _, e := range entries {
			if err := tw.addManifestEntry(e); err != nil {
				return fmt.Errorf("manifest entry %q: %w", e.Dest, err)
			}
		}
	}

	// Merge --tar files.
	for _, tarPath := range cfg.Tars {
		if err := tw.mergeTar(tarPath); err != nil {
			return fmt.Errorf("merging tar %s: %w", tarPath, err)
		}
	}

	// Merge --deb files.
	for _, debPath := range cfg.Debs {
		if err := tw.mergeDeb(debPath); err != nil {
			return fmt.Errorf("merging deb %s: %w", debPath, err)
		}
	}

	// Write MD5 sidecar before closing (md5s are computed during writing).
	if cfg.Md5sumsOutput != "" {
		if err := tw.writeMd5sums(cfg.Md5sumsOutput); err != nil {
			return fmt.Errorf("writing md5sums: %w", err)
		}
	}

	return nil
}

// — — — TarWriter construction — — —

// nopCloser wraps an io.Writer with a no-op Close method.
type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

// procCloser writes to the pipe writer and, on Close, signals EOF to the
// external compressor process and waits for it to exit.
type procCloser struct {
	pw  *io.PipeWriter
	cmd *exec.Cmd
}

func (p *procCloser) Write(b []byte) (int, error) { return p.pw.Write(b) }

func (p *procCloser) Close() error {
	if err := p.pw.Close(); err != nil {
		return err
	}
	return p.cmd.Wait()
}

func newTarWriter(cfg Config) (*TarWriter, error) {
	outFile, err := os.Create(cfg.Output)
	if err != nil {
		return nil, err
	}

	tw := &TarWriter{
		cfg:     cfg,
		outFile: outFile,
		written: make(map[string]byte),
	}

	var w io.WriteCloser

	switch {
	case cfg.Compressor != "":
		w, err = startExternalCompressor(outFile, cfg.Compressor)
	case cfg.Compression == "gz" || cfg.Compression == "tgz":
		w, err = setupGzip(outFile, cfg.CompressionLevel, cfg.DefaultMtime)
	case cfg.Compression == "xz" || cfg.Compression == "lzma":
		w, err = setupXz(outFile)
	case cfg.Compression == "bz2" || cfg.Compression == "bzip2":
		// bzip2 write is not in stdlib; use external bzip2 command.
		w, err = startExternalCompressor(outFile, "bzip2")
	case cfg.Compression == "":
		w = nopCloser{outFile}
	default:
		outFile.Close()
		return nil, fmt.Errorf("unsupported compression: %q", cfg.Compression)
	}
	if err != nil {
		outFile.Close()
		return nil, err
	}

	tw.closers = append(tw.closers, w, outFile)
	tw.tw = tar.NewWriter(w)
	return tw, nil
}

func setupGzip(dst io.Writer, level int, mtime int64) (io.WriteCloser, error) {
	if level < 0 {
		level = gzip.DefaultCompression
	}
	gw, err := gzip.NewWriterLevel(dst, level)
	if err != nil {
		return nil, err
	}
	// Set gzip header mtime for deterministic output.
	gw.ModTime = time.Unix(mtime, 0).UTC()
	return gw, nil
}

func setupXz(dst io.Writer) (io.WriteCloser, error) {
	return xz.NewWriter(dst)
}

func startExternalCompressor(outFile *os.File, cmd string) (io.WriteCloser, error) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty compressor command")
	}
	c := exec.Command(parts[0], parts[1:]...)
	c.Stdout = outFile
	pr, pw := io.Pipe()
	c.Stdin = pr
	if err := c.Start(); err != nil {
		pr.Close()
		pw.Close()
		return nil, fmt.Errorf("starting compressor %q: %w", cmd, err)
	}
	return &procCloser{pw: pw, cmd: c}, nil
}

// close flushes and closes the tar writer, then all closers in order.
func (tw *TarWriter) close() error {
	var errs []string
	if err := tw.tw.Close(); err != nil {
		errs = append(errs, fmt.Sprintf("flushing tar: %v", err))
	}
	for _, c := range tw.closers {
		if err := c.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("closing tar writer: %s", strings.Join(errs, "; "))
	}
	return nil
}

// — — — path normalization — — —

// normalizePath cleans an archive path and applies the directory prefix.
// Equivalent to build_tar.py's TarFile.normalize_path().
func (tw *TarWriter) normalizePath(p string) string {
	p = path.Clean(p)
	p = strings.TrimPrefix(p, "./")
	p = strings.Trim(p, "/")
	if tw.cfg.Directory != "" {
		prefix := tw.cfg.Directory + "/"
		if !strings.HasPrefix(p, prefix) && p != tw.cfg.Directory {
			p = tw.cfg.Directory + "/" + p
		}
	}
	return p
}

// — — — attribute resolution — — —

// fileAttrs resolves the effective mode, ids, and names for an archive path,
// applying per-file overrides from ModeMap/IDsMap/NamesMap over the defaults.
// mode is meaningful only when hasMode is true.
func (tw *TarWriter) fileAttrs(archivePath string) (mode os.FileMode, hasMode bool, ids [2]int, names [2]string) {
	key := strings.TrimPrefix(strings.TrimRight(archivePath, "/"), "/")

	if m, ok := tw.cfg.ModeMap[key]; ok {
		mode, hasMode = m, true
	} else {
		mode, hasMode = tw.cfg.DefaultMode, tw.cfg.HasDefaultMode
	}

	ids = tw.cfg.DefaultIDs
	if i, ok := tw.cfg.IDsMap[key]; ok {
		ids = i
	}

	names = tw.cfg.DefaultNames
	if n, ok := tw.cfg.NamesMap[key]; ok {
		names = n
	}
	return
}

// — — — manifest reading — — —

func readManifest(manifestPath string) ([]ManifestEntry, error) {
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	// Handle Windows UTF-16 LE encoding that older Bazel versions may produce.
	if len(raw) >= 2 && raw[1] == 0x00 {
		// Crude UTF-16 LE detection (matches manifest.py logic).
		wide := make([]rune, len(raw)/2)
		for i := range wide {
			wide[i] = rune(raw[2*i]) | rune(raw[2*i+1])<<8
		}
		raw = []byte(string(wide))
	}
	var entries []ManifestEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// — — — manifest entry dispatch — — —

func (tw *TarWriter) addManifestEntry(entry ManifestEntry) error {
	nonAbsPath := strings.Trim(entry.Dest, "/")
	mode, hasMode, ids, names := tw.fileAttrs(nonAbsPath)

	// Manifest entry attributes take precedence over flags.
	if entry.Mode != "" {
		m, err := parseMode(entry.Mode)
		if err == nil {
			mode = os.FileMode(m)
			hasMode = true
		}
	}
	if entry.User != "" {
		names[0] = entry.User
		if entry.Group != "" {
			names[1] = entry.Group
		}
	}
	if entry.UID != nil {
		ids[0] = *entry.UID
		if entry.GID != nil {
			ids[1] = *entry.GID
		} else {
			ids[1] = *entry.UID
		}
	}

	switch entry.Type {
	case entryIsLink:
		return tw.addSymlink(entry.Dest, entry.Src, ids, names)
	case entryIsRawLink:
		target, err := os.Readlink(entry.Src)
		if err != nil {
			return fmt.Errorf("reading symlink %s: %w", entry.Src, err)
		}
		return tw.addSymlink(entry.Dest, target, ids, names)
	case entryIsDir:
		return tw.addDir(tw.normalizePath(entry.Dest), mode, ids, names)
	case entryIsTree:
		return tw.addTree(entry.Src, entry.Dest, mode, hasMode, ids, names)
	case entryIsEmptyFile:
		return tw.addEmptyFile(tw.normalizePath(entry.Dest), mode, hasMode, ids, names)
	case entryIsFile, "":
		return tw.addFile(entry.Src, entry.Dest, mode, hasMode, ids, names)
	default:
		return fmt.Errorf("unknown manifest entry type %q", entry.Type)
	}
}

// — — — duplicate tracking — — —

// isDuplicate returns true if the path has already been written and should be
// skipped. When AllowDupsFromDeps is set every entry is written unconditionally.
func (tw *TarWriter) isDuplicate(archivePath string, typeflag byte) bool {
	if tw.cfg.AllowDupsFromDeps {
		return false
	}
	key := strings.TrimRight(archivePath, "/")
	existing, seen := tw.written[key]
	if !seen {
		return false
	}
	if typeflag == tar.TypeDir {
		if existing != tar.TypeDir && existing != tar.TypeSymlink {
			fmt.Fprintf(os.Stderr, "directory shadows archive member %s, picking first occurrence\n", archivePath)
		}
	} else {
		fmt.Fprintf(os.Stderr, "duplicate file in archive: %s, picking first occurrence\n", archivePath)
	}
	return true
}

// markWritten records that a path has been added to the archive.
func (tw *TarWriter) markWritten(archivePath string, typeflag byte) {
	tw.written[strings.TrimRight(archivePath, "/")] = typeflag
}

// — — — parent directory auto-creation — — —

func (tw *TarWriter) conditionallyAddParents(archivePath string, ids [2]int, names [2]string, mtime int64) error {
	if !tw.cfg.CreateParents {
		return nil
	}
	// Split on the clean path, iterate over parent components.
	parts := strings.Split(strings.TrimRight(archivePath, "/"), "/")
	parentPath := ""
	for i := 0; i < len(parts)-1; i++ {
		if parentPath == "" {
			parentPath = parts[i] + "/"
		} else {
			parentPath = parentPath + parts[i] + "/"
		}
		key := strings.TrimRight(parentPath, "/")
		// Never write "." or "" as explicit tar entries (matches Python behavior).
		if key == "." || key == "" {
			continue
		}
		if _, exists := tw.written[key]; !exists {
			hdr := &tar.Header{
				Typeflag: tar.TypeDir,
				Name:     parentPath,
				Mode:     0o755,
				ModTime:  time.Unix(mtime, 0).UTC(),
				Uid:      ids[0],
				Gid:      ids[1],
				Uname:    names[0],
				Gname:    names[1],
				Format:   tar.FormatGNU,
			}
			if err := tw.tw.WriteHeader(hdr); err != nil {
				return err
			}
			tw.markWritten(parentPath, tar.TypeDir)
		}
	}
	return nil
}

// — — — individual entry writers — — —

func (tw *TarWriter) addFile(src, dest string, mode os.FileMode, hasMode bool, ids [2]int, names [2]string) error {
	dest = tw.normalizePath(dest)
	if tw.isDuplicate(dest, tar.TypeReg) {
		return nil
	}

	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return err
	}

	// Determine effective mode.
	var finalMode os.FileMode
	if tw.cfg.PreserveMode {
		finalMode = fi.Mode() & os.ModePerm
	} else if hasMode {
		finalMode = mode
	} else if fi.Mode()&0o111 != 0 {
		finalMode = 0o755
	} else {
		finalMode = 0o644
	}

	// Determine effective mtime.
	mtime := tw.cfg.DefaultMtime
	if tw.cfg.PreserveMtime {
		mtime = fi.ModTime().Unix()
	}

	if err := tw.conditionallyAddParents(dest, ids, names, mtime); err != nil {
		return err
	}

	hdr := &tar.Header{
		Typeflag: tar.TypeReg,
		Name:     dest,
		Size:     fi.Size(),
		Mode:     int64(finalMode),
		ModTime:  time.Unix(mtime, 0).UTC(),
		Uid:      ids[0],
		Gid:      ids[1],
		Uname:    names[0],
		Gname:    names[1],
		Format:   tar.FormatGNU,
	}

	hasher := md5.New()
	reader := io.TeeReader(f, hasher)

	if err := tw.tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := io.Copy(tw.tw, reader); err != nil {
		return err
	}

	tw.md5s = append(tw.md5s, md5Entry{
		archivePath: dest,
		hexSum:      hex.EncodeToString(hasher.Sum(nil)),
	})
	tw.markWritten(dest, tar.TypeReg)
	return nil
}

func (tw *TarWriter) addEmptyFile(dest string, mode os.FileMode, hasMode bool, ids [2]int, names [2]string) error {
	if tw.isDuplicate(dest, tar.TypeReg) {
		return nil
	}

	var finalMode os.FileMode
	if hasMode {
		finalMode = mode
	} else {
		finalMode = 0o644
	}

	if err := tw.conditionallyAddParents(dest, ids, names, tw.cfg.DefaultMtime); err != nil {
		return err
	}

	hdr := &tar.Header{
		Typeflag: tar.TypeReg,
		Name:     dest,
		Size:     0,
		Mode:     int64(finalMode),
		ModTime:  time.Unix(tw.cfg.DefaultMtime, 0).UTC(),
		Uid:      ids[0],
		Gid:      ids[1],
		Uname:    names[0],
		Gname:    names[1],
		Format:   tar.FormatGNU,
	}
	if err := tw.tw.WriteHeader(hdr); err != nil {
		return err
	}

	// MD5 of empty content.
	hasher := md5.New()
	tw.md5s = append(tw.md5s, md5Entry{
		archivePath: dest,
		hexSum:      hex.EncodeToString(hasher.Sum(nil)),
	})
	tw.markWritten(dest, tar.TypeReg)
	return nil
}

func (tw *TarWriter) addDir(dest string, mode os.FileMode, ids [2]int, names [2]string) error {
	if !strings.HasSuffix(dest, "/") {
		dest += "/"
	}
	if tw.isDuplicate(dest, tar.TypeDir) {
		return nil
	}

	// Auto-create missing ancestor directories before writing this entry so
	// that parents always precede their children in the archive stream.
	if err := tw.conditionallyAddParents(dest, ids, names, tw.cfg.DefaultMtime); err != nil {
		return err
	}

	var finalMode os.FileMode = 0o755
	if mode != 0 {
		finalMode = mode
	}

	hdr := &tar.Header{
		Typeflag: tar.TypeDir,
		Name:     dest,
		Mode:     int64(finalMode),
		ModTime:  time.Unix(tw.cfg.DefaultMtime, 0).UTC(),
		Uid:      ids[0],
		Gid:      ids[1],
		Uname:    names[0],
		Gname:    names[1],
		Format:   tar.FormatGNU,
	}
	if err := tw.tw.WriteHeader(hdr); err != nil {
		return err
	}
	tw.markWritten(dest, tar.TypeDir)
	return nil
}

func (tw *TarWriter) addSymlink(dest, target string, ids [2]int, names [2]string) error {
	// Preserve leading "./" if present (matches Python behavior).
	if !strings.HasPrefix(dest, "./") {
		dest = tw.normalizePath(dest)
	}
	if tw.isDuplicate(dest, tar.TypeSymlink) {
		return nil
	}

	if err := tw.conditionallyAddParents(dest, ids, names, tw.cfg.DefaultMtime); err != nil {
		return err
	}

	hdr := &tar.Header{
		Typeflag: tar.TypeSymlink,
		Name:     dest,
		Linkname: target,
		Mode:     0o777,
		ModTime:  time.Unix(tw.cfg.DefaultMtime, 0).UTC(),
		Uid:      ids[0],
		Gid:      ids[1],
		Uname:    names[0],
		Gname:    names[1],
		Format:   tar.FormatGNU,
	}
	if err := tw.tw.WriteHeader(hdr); err != nil {
		return err
	}
	// Symlinks are NOT added to md5s.
	tw.markWritten(dest, tar.TypeSymlink)
	return nil
}

func (tw *TarWriter) addTree(srcDir, destPath string, mode os.FileMode, hasMode bool, ids [2]int, names [2]string) error {
	srcDir = filepath.Clean(srcDir)

	// Build the archive destination prefix.
	dest := strings.Trim(destPath, "/")
	if tw.cfg.Directory != "" && !strings.HasPrefix(dest, tw.cfg.Directory+"/") {
		dest = tw.cfg.Directory + "/" + dest
	}
	dest = path.Clean(dest)
	if dest == "." {
		dest = ""
	}

	// Collect entries sorted for determinism (Python sorts within each dir level).
	type treeEntry struct {
		fullPath string
		relPath  string // / separated
		isDir    bool
	}
	var entries []treeEntry
	err := filepath.Walk(srcDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(srcDir, p)
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		entries = append(entries, treeEntry{
			fullPath: p,
			relPath:  rel,
			isDir:    info.IsDir(),
		})
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].relPath < entries[j].relPath })

	for _, e := range entries {
		var archivePath string
		if dest != "" {
			archivePath = dest + "/" + e.relPath
		} else {
			archivePath = e.relPath
		}
		archivePath = path.Clean(archivePath)

		if e.isDir {
			if err := tw.addDir(archivePath+"/", 0o755, ids, names); err != nil {
				return err
			}
		} else {
			if err := tw.addFile(e.fullPath, archivePath, mode, hasMode, ids, names); err != nil {
				return err
			}
		}
	}
	return nil
}

// — — — tar merging — — —

func (tw *TarWriter) mergeTar(tarPath string) error {
	tr, cleanup, err := openTarReader(tarPath)
	if err != nil {
		return err
	}
	defer cleanup()
	return tw.mergeTarReader(tr)
}

func (tw *TarWriter) mergeTarReader(tr *tar.Reader) error {
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		// Apply directory prefix.
		if tw.cfg.Directory != "" {
			hdr.Name = path.Clean(tw.cfg.Directory + "/" + hdr.Name)
			if hdr.Typeflag == tar.TypeDir && !strings.HasSuffix(hdr.Name, "/") {
				hdr.Name += "/"
			}
		}

		// Strip owner names (numeric only, matching Python behavior).
		hdr.Uname = ""
		hdr.Gname = ""

		// Override mtime unless preserving.
		if !tw.cfg.PreserveMtime {
			hdr.ModTime = time.Unix(tw.cfg.DefaultMtime, 0).UTC()
		}

		if err := tw.conditionallyAddParents(hdr.Name, [2]int{hdr.Uid, hdr.Gid}, [2]string{}, tw.cfg.DefaultMtime); err != nil {
			return err
		}

		if tw.isDuplicate(hdr.Name, hdr.Typeflag) {
			continue
		}

		isRegular := hdr.Typeflag == tar.TypeReg || hdr.Typeflag == tar.TypeRegA
		if isRegular {
			hasher := md5.New()
			reader := io.TeeReader(tr, hasher)

			if err := tw.tw.WriteHeader(hdr); err != nil {
				return err
			}
			if _, err := io.Copy(tw.tw, reader); err != nil {
				return err
			}

			archivePath := strings.TrimRight(hdr.Name, "/")
			tw.md5s = append(tw.md5s, md5Entry{
				archivePath: archivePath,
				hexSum:      hex.EncodeToString(hasher.Sum(nil)),
			})
		} else {
			if err := tw.tw.WriteHeader(hdr); err != nil {
				return err
			}
			if _, err := io.Copy(tw.tw, tr); err != nil {
				return err
			}
		}
		tw.markWritten(hdr.Name, hdr.Typeflag)
	}
}

// mergeDeb extracts the data.tar.* payload from a Debian .deb (ar archive)
// and merges it into the output tar.
func (tw *TarWriter) mergeDeb(debPath string) error {
	f, err := os.Open(debPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Verify ar magic.
	magic := make([]byte, 8)
	if _, err := io.ReadFull(f, magic); err != nil {
		return fmt.Errorf("%s: cannot read ar magic: %w", debPath, err)
	}
	if string(magic) != "!<arch>\n" {
		return fmt.Errorf("%s: not a valid ar archive", debPath)
	}

	// Scan ar members for data.tar.*.
	const arHdrSize = 60
	for {
		hdr := make([]byte, arHdrSize)
		n, err := io.ReadFull(f, hdr)
		if err == io.EOF || n == 0 {
			break
		}
		if err != nil {
			return fmt.Errorf("%s: reading ar header: %w", debPath, err)
		}

		name := strings.TrimRight(string(hdr[0:16]), " ")
		sizeStr := strings.TrimRight(string(hdr[48:58]), " ")
		size, err := strconv.ParseInt(sizeStr, 10, 64)
		if err != nil {
			return fmt.Errorf("%s: invalid ar member size: %w", debPath, err)
		}

		if !strings.HasPrefix(name, "data.") {
			skip := size
			if size%2 != 0 {
				skip++
			}
			if _, err := f.Seek(skip, io.SeekCurrent); err != nil {
				return err
			}
			continue
		}

		// Found the data member; open a tar reader over it.
		lr := io.LimitReader(f, size)
		tr, cleanup, err := openTarReaderFromStream(lr, name)
		if err != nil {
			return fmt.Errorf("%s: opening data member: %w", debPath, err)
		}
		defer cleanup()
		return tw.mergeTarReader(tr)
	}
	return fmt.Errorf("%s: no data.tar.* member found", debPath)
}

// — — — tar reader helpers — — —

// openTarReader opens a (possibly compressed) tar file by extension.
func openTarReader(tarPath string) (*tar.Reader, func(), error) {
	f, err := os.Open(tarPath)
	if err != nil {
		return nil, nil, err
	}
	tr, cleanup, err := openTarReaderFromStream(f, tarPath)
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	outer := cleanup
	return tr, func() { outer(); f.Close() }, nil
}

// openTarReaderFromStream wraps r with a decompressor inferred from name's extension.
func openTarReaderFromStream(r io.Reader, name string) (*tar.Reader, func(), error) {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		gr, err := gzip.NewReader(r)
		if err != nil {
			return nil, nil, err
		}
		return tar.NewReader(gr), func() { gr.Close() }, nil
	case strings.HasSuffix(lower, ".tar.xz") || strings.HasSuffix(lower, ".txz"):
		xr, err := xz.NewReader(r)
		if err != nil {
			return nil, nil, err
		}
		return tar.NewReader(xr), func() {}, nil
	case strings.HasSuffix(lower, ".tar.bz2") || strings.HasSuffix(lower, ".tbz2"):
		br := bzip2.NewReader(r)
		return tar.NewReader(br), func() {}, nil
	default:
		return tar.NewReader(r), func() {}, nil
	}
}

// — — — MD5 sidecar output — — —

func (tw *TarWriter) writeMd5sums(outPath string) error {
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, entry := range tw.md5s {
		// Two-space separator matches md5sum(1) output format.
		fmt.Fprintf(w, "%s  %s\n", entry.hexSum, entry.archivePath)
	}
	return w.Flush()
}

// — — — stamp file helper — — —

// readTimestampFromStampFile reads BUILD_TIMESTAMP from a Bazel volatile
// status file. Returns 0 if the key is not present.
func readTimestampFromStampFile(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		k, v, ok := strings.Cut(line, " ")
		if ok && k == "BUILD_TIMESTAMP" {
			ts, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid BUILD_TIMESTAMP %q: %w", v, err)
			}
			return ts, nil
		}
	}
	return 0, nil
}
