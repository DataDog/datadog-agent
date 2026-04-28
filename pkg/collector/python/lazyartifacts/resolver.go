// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package lazyartifacts materializes Python integrations from registry-hosted
// eStargz Agent images on demand.
package lazyartifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/containerd/stargz-snapshotter/estargz"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/opencontainers/go-digest"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

const (
	defaultPlatform      = "linux/amd64"
	embeddedSitePackages = "opt/datadog-agent/embedded/lib"
	pythonCheckPrefix    = "python-check"
	cacheLockStaleAfter  = 30 * time.Minute
)

// Result describes a lazily materialized capability.
type Result struct {
	Capability  string
	CacheDir    string
	ImportPath  string
	CacheHit    bool
	ImageDigest string
	Stats       Stats
}

// Stats reports how much compressed registry data was fetched while resolving
// the capability.
type Stats struct {
	RangeRequests int   `json:"range_requests"`
	RangeBytes    int64 `json:"range_bytes"`
	LayersOpened  int   `json:"layers_opened"`
}

type materializedFile struct {
	Path   string `json:"path"`
	Type   string `json:"type"`
	Size   int64  `json:"size,omitempty"`
	Layer  string `json:"layer"`
	Digest string `json:"digest,omitempty"`
}

type marker struct {
	Image       string             `json:"image"`
	ImageDigest string             `json:"image_digest"`
	Capability  string             `json:"capability"`
	ImportPath  string             `json:"import_path"`
	CreatedAt   time.Time          `json:"created_at"`
	Files       []materializedFile `json:"files"`
	Stats       Stats              `json:"stats"`
}

// EnsurePythonCheck materializes the Python integration named checkName from an
// eStargz Agent image and returns a site-packages directory that can be added to
// sys.path. This is an experimental PoC path and is intentionally disabled by
// default.
func EnsurePythonCheck(ctx context.Context, checkName string) (*Result, error) {
	cfg := pkgconfigsetup.Datadog()
	if !cfg.GetBool("python_lazy_artifacts.enabled") {
		return nil, errors.New("python lazy artifacts are disabled")
	}

	sourceImage := cfg.GetString("python_lazy_artifacts.source_image")
	if sourceImage == "" {
		return nil, errors.New("python_lazy_artifacts.source_image must be set")
	}

	cacheDir := cfg.GetString("python_lazy_artifacts.cache_dir")
	if cacheDir == "" {
		return nil, errors.New("python_lazy_artifacts.cache_dir must be set")
	}

	platform := cfg.GetString("python_lazy_artifacts.platform")
	if platform == "" {
		platform = defaultPlatform
	}

	resolver, err := newResolver(ctx, sourceImage, platform)
	if err != nil {
		return nil, err
	}

	return resolver.ensurePythonCheck(ctx, checkName, cacheDir)
}

type resolver struct {
	ref         name.Reference
	repo        name.Repository
	client      *http.Client
	imageDigest string
	layers      []v1.Descriptor
	layerCache  map[string]*layerReader
	stats       Stats
}

func newResolver(ctx context.Context, imageRef string, platform string) (*resolver, error) {
	parsedPlatform, err := parsePlatform(platform)
	if err != nil {
		return nil, err
	}

	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return nil, err
	}

	desc, err := remote.Get(ref, remote.WithContext(ctx), remote.WithPlatform(parsedPlatform), remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return nil, err
	}

	img, err := desc.Image()
	if err != nil {
		return nil, err
	}

	manifest, err := img.Manifest()
	if err != nil {
		return nil, err
	}

	client, err := registryClient(ctx, ref.Context())
	if err != nil {
		return nil, err
	}

	return &resolver{
		ref:         ref,
		repo:        ref.Context(),
		client:      client,
		imageDigest: desc.Digest.String(),
		layers:      manifest.Layers,
		layerCache:  map[string]*layerReader{},
	}, nil
}

func parsePlatform(rawPlatform string) (v1.Platform, error) {
	parts := strings.Split(rawPlatform, "/")
	if len(parts) != 2 {
		return v1.Platform{}, fmt.Errorf("platform must be os/arch, got %q", rawPlatform)
	}
	return v1.Platform{OS: parts[0], Architecture: parts[1]}, nil
}

func registryClient(ctx context.Context, repo name.Repository) (*http.Client, error) {
	auth, err := authn.Resolve(ctx, authn.DefaultKeychain, repo)
	if err != nil {
		return nil, err
	}

	rt, err := transport.NewWithContext(ctx, repo.Registry, auth, remote.DefaultTransport, []string{repo.Scope(transport.PullScope)})
	if err != nil {
		return nil, err
	}

	return &http.Client{Transport: rt}, nil
}

func (r *resolver) ensurePythonCheck(ctx context.Context, checkName string, cacheDir string) (*Result, error) {
	capability := capabilityNameForPythonCheck(checkName)
	cacheKey := cacheKey(r.imageDigest, capability)
	finalDir := filepath.Join(cacheDir, cacheKey)
	markerPath := filepath.Join(finalDir, ".complete.json")

	if cached, err := readMarker(markerPath); err == nil {
		return &Result{
			Capability:  capability,
			CacheDir:    finalDir,
			ImportPath:  cached.ImportPath,
			CacheHit:    true,
			ImageDigest: cached.ImageDigest,
			Stats:       cached.Stats,
		}, nil
	}

	unlock, err := lockCache(ctx, cacheDir, cacheKey)
	if err != nil {
		return nil, err
	}
	defer unlock()

	if cached, err := readMarker(markerPath); err == nil {
		return &Result{
			Capability:  capability,
			CacheDir:    finalDir,
			ImportPath:  cached.ImportPath,
			CacheHit:    true,
			ImageDigest: cached.ImageDigest,
			Stats:       cached.Stats,
		}, nil
	}

	layer, checkPath, ent, err := r.findPythonCheck(ctx, checkName)
	if err != nil {
		return nil, err
	}

	importPath, err := pythonCheckImportPath(finalDir, checkPath, checkName)
	if err != nil {
		return nil, err
	}

	tmpDir := finalDir + fmt.Sprintf(".tmp-%d", os.Getpid())
	if err := os.RemoveAll(tmpDir); err != nil {
		return nil, err
	}
	rootDir := filepath.Join(tmpDir, "root")
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return nil, err
	}

	var files []materializedFile
	if err := materializeEntry(layer, rootDir, checkPath, ent, &files); err != nil {
		os.RemoveAll(tmpDir) //nolint:errcheck
		return nil, err
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	stats := r.accumulateStats()
	m := marker{
		Image:       r.ref.String(),
		ImageDigest: r.imageDigest,
		Capability:  capability,
		ImportPath:  importPath,
		CreatedAt:   time.Now().UTC(),
		Files:       files,
		Stats:       stats,
	}

	markerBytes, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		os.RemoveAll(tmpDir) //nolint:errcheck
		return nil, err
	}

	if err := os.WriteFile(filepath.Join(tmpDir, ".complete.json"), markerBytes, 0o644); err != nil {
		os.RemoveAll(tmpDir) //nolint:errcheck
		return nil, err
	}

	if err := os.RemoveAll(finalDir); err != nil {
		os.RemoveAll(tmpDir) //nolint:errcheck
		return nil, err
	}
	if err := os.Rename(tmpDir, finalDir); err != nil {
		os.RemoveAll(tmpDir) //nolint:errcheck
		return nil, err
	}

	return &Result{
		Capability:  capability,
		CacheDir:    finalDir,
		ImportPath:  importPath,
		CacheHit:    false,
		ImageDigest: r.imageDigest,
		Stats:       stats,
	}, nil
}

func readMarker(markerPath string) (*marker, error) {
	f, err := os.Open(markerPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cached marker
	if err := json.NewDecoder(f).Decode(&cached); err != nil {
		return nil, err
	}
	if cached.ImportPath == "" || cached.ImageDigest == "" {
		return nil, errors.New("incomplete lazy artifact marker")
	}
	return &cached, nil
}

func lockCache(ctx context.Context, cacheDir string, cacheKey string) (func(), error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, err
	}

	lockDir := filepath.Join(cacheDir, cacheKey+".lock")
	for {
		err := os.Mkdir(lockDir, 0o700)
		if err == nil {
			return func() {
				os.Remove(lockDir) //nolint:errcheck
			}, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}

		if info, statErr := os.Stat(lockDir); statErr == nil && time.Since(info.ModTime()) > cacheLockStaleAfter {
			os.Remove(lockDir) //nolint:errcheck
			continue
		}

		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (r *resolver) findPythonCheck(ctx context.Context, checkName string) (*layerReader, string, *estargz.TOCEntry, error) {
	for i := len(r.layers) - 1; i >= 0; i-- {
		layer, err := r.openLayer(ctx, r.layers[i])
		if err != nil {
			return nil, "", nil, err
		}

		checkPath, ent, found := findPythonCheckInLayer(layer, checkName)
		if found {
			return layer, checkPath, ent, nil
		}
	}

	return nil, "", nil, fmt.Errorf("python check %q not found in eStargz image", checkName)
}

func findPythonCheckInLayer(layer *layerReader, checkName string) (string, *estargz.TOCEntry, bool) {
	lib, ok := layer.stargz.Lookup(embeddedSitePackages)
	if !ok || lib.Type != "dir" {
		return "", nil, false
	}

	var checkPath string
	var checkEntry *estargz.TOCEntry
	lib.ForeachChild(func(base string, child *estargz.TOCEntry) bool {
		if child.Type != "dir" || !strings.HasPrefix(base, "python") {
			return true
		}

		candidate := path.Join(embeddedSitePackages, base, "site-packages", "datadog_checks", checkName)
		ent, ok := layer.stargz.Lookup(candidate)
		if !ok || ent.Type != "dir" {
			return true
		}

		checkPath = candidate
		checkEntry = ent
		return false
	})

	return checkPath, checkEntry, checkEntry != nil
}

type layerReader struct {
	desc     v1.Descriptor
	blob     *registryBlobReaderAt
	stargz   *estargz.Reader
	verifier estargz.TOCEntryVerifier
}

func (r *resolver) openLayer(ctx context.Context, desc v1.Descriptor) (*layerReader, error) {
	key := desc.Digest.String()
	if lr, ok := r.layerCache[key]; ok {
		return lr, nil
	}

	blob := &registryBlobReaderAt{
		ctx:    ctx,
		client: r.client,
		repo:   r.repo,
		digest: desc.Digest,
		size:   desc.Size,
	}

	reader, err := estargz.Open(io.NewSectionReader(blob, 0, desc.Size))
	if err != nil {
		return nil, fmt.Errorf("open eStargz layer %s: %w", desc.Digest, err)
	}

	tocDigestRaw := desc.Annotations[estargz.TOCJSONDigestAnnotation]
	if tocDigestRaw == "" {
		return nil, fmt.Errorf("layer %s is missing %s", desc.Digest, estargz.TOCJSONDigestAnnotation)
	}
	tocDigest, err := digest.Parse(tocDigestRaw)
	if err != nil {
		return nil, fmt.Errorf("parse TOC digest annotation for layer %s: %w", desc.Digest, err)
	}

	verifier, err := reader.VerifyTOC(tocDigest)
	if err != nil {
		return nil, fmt.Errorf("verify TOC for layer %s: %w", desc.Digest, err)
	}

	lr := &layerReader{desc: desc, blob: blob, stargz: reader, verifier: verifier}
	r.layerCache[key] = lr
	r.stats.LayersOpened++
	return lr, nil
}

func (r *resolver) accumulateStats() Stats {
	out := r.stats
	for _, lr := range r.layerCache {
		reqs, bytes := lr.blob.stats()
		out.RangeRequests += reqs
		out.RangeBytes += bytes
	}
	return out
}

type registryBlobReaderAt struct {
	ctx    context.Context
	client *http.Client
	repo   name.Repository
	digest v1.Hash
	size   int64

	mu            sync.Mutex
	rangeRequests int
	rangeBytes    int64
}

func (r *registryBlobReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if off < 0 || off >= r.size {
		return 0, io.EOF
	}

	want := len(p)
	end := off + int64(want) - 1
	if end >= r.size {
		end = r.size - 1
		want = int(end - off + 1)
		p = p[:want]
	}

	u := url.URL{
		Scheme: r.repo.Registry.Scheme(),
		Host:   r.repo.Registry.RegistryStr(),
		Path:   fmt.Sprintf("/v2/%s/blobs/%s", r.repo.RepositoryStr(), r.digest.String()),
	}
	req, err := http.NewRequestWithContext(r.ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", off, end))

	resp, err := r.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent {
		return 0, fmt.Errorf("range GET %s returned %s", u.String(), resp.Status)
	}

	n, err := io.ReadFull(resp.Body, p)
	r.mu.Lock()
	r.rangeRequests++
	r.rangeBytes += int64(n)
	r.mu.Unlock()
	if err != nil {
		return n, err
	}
	if n != want {
		return n, io.ErrUnexpectedEOF
	}
	return n, nil
}

func (r *registryBlobReaderAt) stats() (int, int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rangeRequests, r.rangeBytes
}

func materializeEntry(layer *layerReader, rootDir string, imagePath string, ent *estargz.TOCEntry, files *[]materializedFile) error {
	switch ent.Type {
	case "dir":
		if err := mkdirSafe(rootDir, imagePath, os.FileMode(ent.Mode)&0o777); err != nil {
			return err
		}
		*files = append(*files, materializedFile{Path: "/" + imagePath, Type: "dir", Layer: layer.desc.Digest.String()})

		var walkErr error
		ent.ForeachChild(func(base string, child *estargz.TOCEntry) bool {
			walkErr = materializeEntry(layer, rootDir, path.Join(imagePath, base), child, files)
			return walkErr == nil
		})
		return walkErr
	case "reg":
		return materializeRegularFile(layer, rootDir, imagePath, ent, files)
	case "symlink":
		return materializeSymlink(layer, rootDir, imagePath, ent, files)
	default:
		return fmt.Errorf("unsupported entry type %q at %q", ent.Type, imagePath)
	}
}

func materializeRegularFile(layer *layerReader, rootDir string, imagePath string, ent *estargz.TOCEntry, files *[]materializedFile) error {
	dest, err := safeDest(rootDir, imagePath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	tmp := dest + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fileMode(ent.Mode))
	if err != nil {
		return err
	}

	h := sha256.New()
	n, copyErr := copyVerifiedChunks(layer, imagePath, ent, io.MultiWriter(out, h))
	closeErr := out.Close()
	if copyErr != nil {
		os.Remove(tmp) //nolint:errcheck
		return copyErr
	}
	if closeErr != nil {
		os.Remove(tmp) //nolint:errcheck
		return closeErr
	}
	if n != ent.Size {
		os.Remove(tmp) //nolint:errcheck
		return fmt.Errorf("short materialization for %q: copied %d bytes, expected %d", imagePath, n, ent.Size)
	}

	actualDigest := "sha256:" + hex.EncodeToString(h.Sum(nil))
	if ent.Digest != "" && actualDigest != ent.Digest {
		os.Remove(tmp) //nolint:errcheck
		return fmt.Errorf("file digest mismatch for %q: got %s, want %s", imagePath, actualDigest, ent.Digest)
	}

	if err := os.Chmod(tmp, fileMode(ent.Mode)); err != nil {
		os.Remove(tmp) //nolint:errcheck
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp) //nolint:errcheck
		return err
	}

	*files = append(*files, materializedFile{
		Path:   "/" + imagePath,
		Type:   "reg",
		Size:   ent.Size,
		Layer:  layer.desc.Digest.String(),
		Digest: actualDigest,
	})
	return nil
}

func copyVerifiedChunks(layer *layerReader, imagePath string, ent *estargz.TOCEntry, out io.Writer) (int64, error) {
	src, err := layer.stargz.OpenFile(imagePath)
	if err != nil {
		return 0, err
	}

	var copied int64
	for copied < ent.Size {
		chunk, ok := layer.stargz.ChunkEntryForOffset(imagePath, copied)
		if !ok {
			return copied, fmt.Errorf("no chunk for %q at offset %d", imagePath, copied)
		}

		size := chunk.ChunkSize
		if size <= 0 || copied+size > ent.Size {
			size = ent.Size - copied
		}
		buf := make([]byte, size)

		n, readErr := src.ReadAt(buf, copied)
		if n > 0 {
			if err := verifyChunk(layer, chunk, buf[:n]); err != nil {
				return copied, err
			}
			written, writeErr := out.Write(buf[:n])
			copied += int64(written)
			if writeErr != nil {
				return copied, writeErr
			}
			if written != n {
				return copied, io.ErrShortWrite
			}
		}
		if readErr != nil {
			if readErr == io.EOF && copied == ent.Size {
				break
			}
			return copied, readErr
		}
	}

	return copied, nil
}

func verifyChunk(layer *layerReader, chunk *estargz.TOCEntry, data []byte) error {
	v, err := layer.verifier.Verifier(chunk)
	if err != nil {
		return fmt.Errorf("chunk verifier for offset %d: %w", chunk.Offset, err)
	}
	if _, err := v.Write(data); err != nil {
		return fmt.Errorf("write chunk verifier for offset %d: %w", chunk.Offset, err)
	}
	if !v.Verified() {
		return fmt.Errorf("chunk digest mismatch at offset %d", chunk.Offset)
	}
	return nil
}

func materializeSymlink(layer *layerReader, rootDir string, imagePath string, ent *estargz.TOCEntry, files *[]materializedFile) error {
	if filepath.IsAbs(ent.LinkName) || strings.Contains(filepath.Clean(ent.LinkName), "..") {
		return fmt.Errorf("unsafe symlink %q -> %q", imagePath, ent.LinkName)
	}

	dest, err := safeDest(rootDir, imagePath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	if err := os.Symlink(ent.LinkName, dest); err != nil {
		return err
	}

	*files = append(*files, materializedFile{Path: "/" + imagePath, Type: "symlink", Layer: layer.desc.Digest.String()})
	return nil
}

func mkdirSafe(rootDir string, imagePath string, mode os.FileMode) error {
	dest, err := safeDest(rootDir, imagePath)
	if err != nil {
		return err
	}
	if mode == 0 {
		mode = 0o755
	}
	return os.MkdirAll(dest, mode)
}

func safeDest(rootDir string, imagePath string) (string, error) {
	clean := cleanImagePath(imagePath)
	if clean == "." || clean == "" {
		return "", errors.New("refusing to materialize root")
	}

	dest := filepath.Join(rootDir, filepath.FromSlash(clean))
	rel, err := filepath.Rel(rootDir, dest)
	if err != nil {
		return "", err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", fmt.Errorf("path escapes root: %q", imagePath)
	}
	return dest, nil
}

func cleanImagePath(p string) string {
	return strings.TrimPrefix(path.Clean("/"+p), "/")
}

func fileMode(mode int64) os.FileMode {
	m := os.FileMode(mode) & 0o777
	if m == 0 {
		return 0o644
	}
	return m
}

func pythonCheckImportPath(finalDir string, checkPath string, checkName string) (string, error) {
	suffix := path.Join("datadog_checks", checkName)
	if !strings.HasSuffix(checkPath, suffix) {
		return "", fmt.Errorf("python check path %q does not end with %q", checkPath, suffix)
	}

	sitePackages := strings.TrimSuffix(checkPath, suffix)
	sitePackages = strings.TrimSuffix(sitePackages, "/")
	if sitePackages == "" {
		return "", fmt.Errorf("unable to derive site-packages path from %q", checkPath)
	}

	return filepath.Join(finalDir, "root", filepath.FromSlash(sitePackages)), nil
}

func capabilityNameForPythonCheck(checkName string) string {
	return pythonCheckPrefix + ":" + checkName
}

func cacheKey(imageDigest string, capabilityName string) string {
	sum := sha256.Sum256([]byte(imageDigest + "\x00" + capabilityName))
	safeCap := strings.NewReplacer("/", "_", ":", "_").Replace(capabilityName)
	return safeCap + "-" + hex.EncodeToString(sum[:])[:16]
}
