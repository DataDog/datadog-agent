// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentbuild

import (
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
)

var pythonABIPattern = regexp.MustCompile(`^python3\.[0-9]+$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var imagePattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
var referencePattern = regexp.MustCompile(`^[a-zA-Z0-9.-]+(:[0-9]+)?/[a-zA-Z0-9/._-]+(:[a-zA-Z0-9_][a-zA-Z0-9._-]*|@sha256:[a-f0-9]{64})$`)

func ValidateReference(ref string) error {
	if !referencePattern.MatchString(ref) {
		return fmt.Errorf("expected registry-qualified image reference, got %q", ref)
	}
	return nil
}
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func DescribeFile(path string) (File, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return File{}, err
	}
	i, err := os.Lstat(path)
	if err != nil {
		return File{}, err
	}
	if !i.Mode().IsRegular() {
		return File{}, fmt.Errorf("artifact must be a regular file: %s", path)
	}
	sum, err := HashFile(path)
	return File{Path: path, SHA256: sum, Mode: uint32(i.Mode().Perm())}, err
}
func verifyFile(f File, path string) error {
	i, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !i.Mode().IsRegular() || f.Link != "" || !digestPattern.MatchString(f.SHA256) || uint32(i.Mode().Perm()) != f.Mode {
		return fmt.Errorf("invalid artifact file/mode: %s", path)
	}
	sum, err := HashFile(path)
	if err != nil {
		return err
	}
	if sum != f.SHA256 {
		return fmt.Errorf("artifact checksum mismatch: %s", path)
	}
	return nil
}
func safeRelative(path string) bool {
	return path != "" && path != "." && !filepath.IsAbs(path) && filepath.Clean(path) == path && path != ".." && !strings.HasPrefix(path, ".."+string(filepath.Separator))
}
func ValidatePrefix(prefix string) error {
	if !filepath.IsAbs(prefix) || filepath.Clean(prefix) != prefix || strings.ContainsAny(prefix, ":,\n\r") || (!strings.HasSuffix(prefix, "/dev/embedded") && prefix != "/opt/e2ectl/runtime") {
		return fmt.Errorf("unsupported embedded runtime mount prefix %q", prefix)
	}
	return nil
}
func verifyTree(t Tree) error {
	if !filepath.IsAbs(t.Root) || strings.ContainsAny(t.Root, ":,\n\r") || len(t.Files) == 0 {
		return fmt.Errorf("artifact tree requires absolute root and inventory")
	}
	root, err := filepath.EvalSymlinks(t.Root)
	if err != nil {
		return err
	}
	if root != t.Root {
		return fmt.Errorf("tree root must not traverse symlinks")
	}
	seen := map[string]bool{}
	for _, f := range t.Files {
		if !safeRelative(f.Path) || seen[f.Path] {
			return fmt.Errorf("unsafe or duplicate artifact path %q", f.Path)
		}
		seen[f.Path] = true
		path := filepath.Join(root, f.Path)
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, resolved)
		if err != nil || !safeRelative(rel) {
			return fmt.Errorf("artifact path escapes tree: %s", f.Path)
		}
		if f.Link != "" {
			link, err := os.Readlink(path)
			if err != nil || link != f.Link || filepath.IsAbs(link) {
				return fmt.Errorf("unsafe artifact symlink %s", f.Path)
			}
			continue
		}
		if err := verifyFile(f, path); err != nil {
			return err
		}
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if !seen[rel] {
			return fmt.Errorf("unrecorded runtime file %s", rel)
		}
		return nil
	})
}
func (r Result) Validate(target Target) error {
	if r.Schema != 1 {
		return fmt.Errorf("unsupported artifact schema %d", r.Schema)
	}
	if err := target.Validate(); err != nil {
		return err
	}
	if r.Target != target {
		return fmt.Errorf("artifact target %+v differs from target %+v", r.Target, target)
	}
	count := 0
	if r.Binary != nil {
		count++
	}
	if r.Image != nil {
		count++
	}
	if r.Package != nil {
		count++
	}
	if count != 1 || r.Provenance.Producer == "" {
		return fmt.Errorf("artifact requires one payload and producer")
	}
	if r.Profile != nil {
		if err := r.Profile.Validate(); err != nil {
			return err
		}
		if !digestPattern.MatchString(r.Provenance.SourceSHA256) {
			return fmt.Errorf("producer profile requires observed source-content evidence")
		}
		switch {
		case r.Image != nil && r.Provenance.Producer == "invoke-image":
			if !imagePattern.MatchString(r.Provenance.BaseIdentity) {
				return fmt.Errorf("unsupported image producer profile evidence")
			}
		case r.Binary != nil && r.Provenance.Producer == "invoke-binary":
			if r.Provenance.Options["runtimeLayout"] != "bazel-embedded-absolute-prefix" || len(r.Profile.Roles) != 1 || r.Profile.Roles[0] != receivers.CoreAgent {
				return fmt.Errorf("binary profile requires the native core-source runtime contract")
			}
		default:
			return fmt.Errorf("unsupported producer profile evidence")
		}
	}
	if b := r.Binary; b != nil {
		if b.RuntimeImageID != "" && (!imagePattern.MatchString(b.RuntimeImageID) || !pythonABIPattern.MatchString(b.PythonABI) || b.PythonPath != "/opt/datadog-agent/embedded/lib/"+b.PythonABI+"/site-packages" || b.OSVersion == "") {
			return fmt.Errorf("invalid runtime image dependency receipt")
		}
		if !filepath.IsAbs(b.Executable.Path) {
			return fmt.Errorf("executable path must be absolute")
		}
		if err := verifyFile(b.Executable, b.Executable.Path); err != nil {
			return err
		}
		if b.Executable.Mode&0111 == 0 {
			return fmt.Errorf("Agent binary is not executable")
		}
		f, err := elf.Open(b.Executable.Path)
		if err != nil {
			return fmt.Errorf("Agent must be ELF: %w", err)
		}
		defer f.Close()
		if (target.Arch == "amd64" && f.Machine != elf.EM_X86_64) || (target.Arch == "arm64" && f.Machine != elf.EM_AARCH64) {
			return fmt.Errorf("ELF architecture mismatch")
		}
		if err := ValidatePrefix(b.RuntimePrefix); err != nil {
			return err
		}
		if err := verifyTree(b.Runtime); err != nil {
			return err
		}
		return verifyTree(b.Assets)
	}
	if i := r.Image; i != nil {
		if err := ValidateReference(i.Reference); err != nil {
			return err
		}
		if !imagePattern.MatchString(i.ID) {
			return fmt.Errorf("image requires actual Docker image ID")
		}
		if i.Delivered != "" {
			return ValidateReference(i.Delivered)
		}
		return nil
	}
	p := r.Package
	if p.Format != "deb" || filepath.Ext(p.File.Path) != ".deb" || p.Name != "datadog-agent" || p.Version == "" || len(p.Roles) == 0 || p.Roles[0] != "agent" || !filepath.IsAbs(p.File.Path) {
		return fmt.Errorf("only exact datadog-agent DEB packages are supported")
	}
	if err := validatePackageRoles(p.Roles); err != nil {
		return err
	}
	return verifyFile(p.File, p.File.Path)
}
func Read(path string) (Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()
	var r Result
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	if err = d.Decode(&r); err != nil {
		return r, err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return r, fmt.Errorf("trailing artifact data")
	}
	return r, nil
}
func Write(path string, r Result) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".result-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func copyArtifact(src, dst string, mode uint32) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(mode))
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	closeErr := out.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Chmod(dst, os.FileMode(mode))
}
func stageTree(t Tree, dst string) (Tree, error) {
	if err := os.MkdirAll(dst, 0700); err != nil {
		return Tree{}, err
	}
	for _, f := range t.Files {
		path := filepath.Join(dst, f.Path)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return Tree{}, err
		}
		var err error
		if f.Link != "" {
			err = os.Symlink(f.Link, path)
		} else {
			err = copyArtifact(filepath.Join(t.Root, f.Path), path, f.Mode)
		}
		if err != nil {
			return Tree{}, err
		}
	}
	t.Root = dst
	return t, nil
}

// Stage creates a new immutable generation; never overwrites installed pins.
func Stage(r Result, dir string) (Result, error) {
	if err := r.Validate(r.Target); err != nil {
		return r, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return r, err
	}
	gen, err := os.MkdirTemp(dir, "generation-")
	if err != nil {
		return r, err
	}
	if r.Binary != nil {
		b := *r.Binary
		r.Binary = &b
		dst := filepath.Join(gen, "agent")
		if err = copyArtifact(b.Executable.Path, dst, b.Executable.Mode); err != nil {
			return r, err
		}
		b.Executable.Path = dst
		if b.Runtime, err = stageTree(b.Runtime, filepath.Join(gen, "runtime")); err != nil {
			return r, err
		}
		if b.Assets, err = stageTree(b.Assets, filepath.Join(gen, "assets")); err != nil {
			return r, err
		}
	}
	if r.Package != nil {
		p := *r.Package
		r.Package = &p
		dst := filepath.Join(gen, "agent.deb")
		if err = copyArtifact(p.File.Path, dst, p.File.Mode); err != nil {
			return r, err
		}
		p.File.Path = dst
	}
	if err = r.Validate(r.Target); err != nil {
		return r, err
	}
	return r, Write(filepath.Join(gen, "result.json"), r)
}
func samePackage(a, b *Package) bool {
	return reflect.DeepEqual(a, b)
}
