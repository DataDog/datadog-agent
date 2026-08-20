// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package runtime

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// runtimeDirLogState dedupes the runtime-directory warnings, which would
// otherwise repeat once per asset compiled (secureRuntimeDir runs inside the
// per-asset compile()).
var runtimeDirLogState struct {
	sync.Mutex
	seen map[string]struct{}
}

// logRuntimeDirOnce reports whether the given reason has not yet been logged,
// recording it so subsequent identical reasons are suppressed.
func logRuntimeDirOnce(reason string) bool {
	runtimeDirLogState.Lock()
	defer runtimeDirLogState.Unlock()
	if _, ok := runtimeDirLogState.seen[reason]; ok {
		return false
	}
	if runtimeDirLogState.seen == nil {
		runtimeDirLogState.seen = make(map[string]struct{})
	}
	runtimeDirLogState.seen[reason] = struct{}{}
	return true
}

// logRuntimeDirReclaimed warns (once per reclaimed path) that a stale
// runtime-directory component was moved aside and recreated. A recurring reclaim
// points at something else recreating the directory as non-root, so it is worth
// surfacing rather than doing it silently.
func logRuntimeDirReclaimed(p string) {
	if logRuntimeDirOnce("reclaimed:" + p) {
		log.Warnf("recreated runtime compiler output directory component %s: it was not a root-owned directory", p)
	}
}

// asset represents an asset that needs its content integrity checked at runtime
type asset struct {
	filename string
	hash     string
	tm       CompilationTelemetry
}

func newAsset(filename, hash string) *asset {
	return &asset{
		filename: filename,
		hash:     hash,
		tm:       newCompilationTelemetry(),
	}
}

// CompileOptions are options used to compile eBPF programs at runtime
type CompileOptions struct {
	// AdditionalFlags are extra flags passed to clang
	AdditionalFlags []string
	// ModifyCallback is a callback function that is allowed to modify the contents before compilation
	ModifyCallback func(in io.Reader, out io.Writer) error
	// UseKernelHeaders enables the inclusion of kernel headers from the host
	UseKernelHeaders bool
}

// Compile compiles the asset to an object file, writes it to the configured output directory, and
// then opens and returns the compiled output
func (a *asset) Compile(config *ebpf.Config, additionalFlags []string) (CompiledOutput, error) {
	return a.compile(config, CompileOptions{AdditionalFlags: additionalFlags, UseKernelHeaders: true})
}

// CompileWithOptions is the same as Compile, but takes an options struct with additional choices.
func (a *asset) CompileWithOptions(config *ebpf.Config, opts CompileOptions) (CompiledOutput, error) {
	return a.compile(config, opts)
}

func (a *asset) compile(config *ebpf.Config, opts CompileOptions) (CompiledOutput, error) {
	log.Debugf("starting runtime compilation of %s", a.filename)

	start := time.Now()
	a.tm.compilationEnabled = true
	defer func() {
		a.tm.compilationDuration = time.Since(start)
		a.tm.SubmitTelemetry(a.filename)
	}()

	var kernelHeaders []string
	if opts.UseKernelHeaders {
		headerOpts := headers.HeaderOptions{
			DownloadEnabled: config.EnableKernelHeaderDownload,
			Dirs:            config.KernelHeadersDirs,
			DownloadDir:     config.KernelHeadersDownloadDir,
			AptConfigDir:    config.AptConfigDir,
			YumReposDir:     config.YumReposDir,
			ZypperReposDir:  config.ZypperReposDir,
		}
		kernelHeaders = headers.GetKernelHeaders(headerOpts)
		if len(kernelHeaders) == 0 {
			a.tm.compilationResult = headerFetchErr
			return nil, errors.New("unable to find kernel headers")
		}
	}

	a.tm.compilationResult = verificationError
	outputDir := config.RuntimeCompilerOutputDir

	p := filepath.Join(config.BPFDir, "runtime", a.filename)
	f, err := os.Open(p)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", p, err)
	}
	defer f.Close()

	if err := secureRuntimeDir(outputDir); err != nil {
		// Surface the policy refusal distinctly so operators can tell "runtime
		// compilation disabled because the output directory is not under root's
		// control" apart from an ordinary compilation failure. compile() runs
		// once per asset, so dedupe by reason to avoid one warning per asset for
		// the same misconfigured directory.
		if logRuntimeDirOnce(err.Error()) {
			log.Warnf("skipping runtime compilation: %v", err)
		}
		return nil, err
	}

	diskProtectedFile, err := createProtectedFile(fmt.Sprintf("%s-%s", a.filename, a.hash), outputDir, f)
	if err != nil {
		return nil, fmt.Errorf("failed to create ram backed file from %s: %w", f.Name(), err)
	}
	defer func() {
		if err := diskProtectedFile.Close(); err != nil {
			log.Debugf("error closing protected file %s: %s", diskProtectedFile.Name(), err)
		}
	}()
	protectedFile := diskProtectedFile
	hash := a.hash

	if err = a.verify(diskProtectedFile); err != nil {
		return nil, fmt.Errorf("error reading input file: %s", err)
	}

	a.tm.compilationResult = compilationErr
	if opts.ModifyCallback != nil {
		outBuf := &bytes.Buffer{}
		// seek to the start and read all of protected file contents
		if _, err := diskProtectedFile.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("seek disk protected file: %w", err)
		}

		// run modify callback
		if err := opts.ModifyCallback(diskProtectedFile, outBuf); err != nil {
			return nil, fmt.Errorf("modify callback: %w", err)
		}
		outReader := bytes.NewReader(outBuf.Bytes())

		// update hash
		hash, err = sha256Reader(outReader)
		if err != nil {
			return nil, fmt.Errorf("hash post-modification protected file: %w", err)
		}
		if _, err := outReader.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("seek post-modification contents: %w", err)
		}

		// create new protected file with the post-modification contents
		postModifyProtectedFile, err := createProtectedFile(fmt.Sprintf("%s-%s", a.filename, hash), outputDir, outReader)
		if err != nil {
			return nil, fmt.Errorf("create post-modification protected file: %w", err)
		}
		defer func() {
			if err := postModifyProtectedFile.Close(); err != nil {
				log.Debugf("close post-modification protected file %s: %s", postModifyProtectedFile.Name(), err)
			}
		}()

		// set compilation to use post-modification contents
		protectedFile = postModifyProtectedFile
	}

	out, result, err := compileToObjectFile(protectedFile.Name(), outputDir, a.filename, hash, opts.AdditionalFlags, kernelHeaders)
	a.tm.compilationResult = result

	return out, err
}

// dedicatedDirName is the agent-owned top-level component of the runtime
// compiler cache path (the default is /var/tmp/datadog-agent/system-probe/build).
// Reclaim is confined to this subtree: a wrong-owner component is only ever
// renamed aside and deleted once we are provably inside .../datadog-agent/...,
// so a misconfigured or attacker-supplied output_dir nested under a shared,
// non-dedicated directory (e.g. /var/tmp/shared/datadog/build) is refused rather
// than causing that shared directory to be destroyed. Keep this in sync with the
// runtime_compiler_output_dir default (pkg/config/setup/system_probe_settings.go).
const dedicatedDirName = "datadog-agent"

// secureRuntimeDir ensures the runtime-compiler output directory exists, is
// root-only (0700), and that it and every ancestor up to the filesystem root is
// a real directory owned by root and not writable by other users. The compiler
// writes object files here and system-probe later re-reads them to load into the
// kernel as root, so the whole path must be under root's control.
//
// It verifies the already-existing prefix of the path before creating anything,
// then creates only the missing tail one component at a time. Creation never
// follows a symlink: every existing ancestor is verified as a real (non-symlink)
// root-owned directory first, so os.Mkdir of the next component resolves only
// through verified directories and returns EEXIST rather than traversing an entry
// that appeared underneath it. This is deliberately not os.MkdirAll over the
// whole path, which would resolve - and create through - a symlinked component
// before any check ran.
//
// It rejects the directory if any ancestor is a symlink, is not a directory, is
// not owned by root, or is writable by group or other without the sticky bit
// set. The sticky-bit exception permits the default location under /var/tmp
// (root-owned, mode 1777): the sticky bit keeps other users from renaming or
// deleting the datadog-agent directory that root creates beneath it.
//
// The default parent /var/tmp is shared and writable by every user, so the
// datadog-agent directory beneath it may already exist owned by a non-root user
// (left over from a previous install, or created by another process). Rather
// than refusing in that case for the lifetime of the process - which would break
// upgraded hosts and leave runtime compilation disabled - secureRuntimeDir
// repairs the common case: a wrong-owner directory that is both below the sticky
// boundary and inside the agent's own datadog-agent subtree is renamed aside and
// the path recreated fresh as root. Only root can rename entries inside a sticky
// directory, so the repair is not subject to interference, and renaming aside
// (rather than reusing or chown-ing the existing tree) means we never write
// through whatever it previously contained.
//
// The repair is deliberately narrow (see ensureRuntimeDirChain): it happens only
// as root, only below a verified root-owned sticky directory, only inside the
// agent's own dedicated subtree (dedicatedDirName), and only for a real directory
// with the wrong owner/permissions. This last condition is what keeps a
// misconfigured or attacker-supplied output_dir nested under a shared directory
// (e.g. /var/tmp/shared/datadog/build) from causing that shared directory to be
// renamed aside and recursively deleted. A symlink, a non-directory, anything at
// or above the sticky boundary, and anything outside the datadog-agent subtree
// are all still refused, so unexpected or broken layouts fail closed rather than
// being silently rewritten.
func secureRuntimeDir(outputDir string) error {
	components, err := runtimeDirComponents(outputDir)
	if err != nil {
		return err
	}

	// A single pass verifies the existing prefix, repairs one offending
	// component if needed, and creates the missing tail. The bounded retry only
	// guards against a concurrent actor racing a component between our check and
	// our create (os.Mkdir returning EEXIST on a freshly appeared entry); it is
	// not needed in the normal single-process case.
	const maxAttempts = 3
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		retryable, err := ensureRuntimeDirChain(components)
		if err == nil {
			return nil
		}
		if !retryable {
			return err
		}
		lastErr = err
	}
	return fmt.Errorf("unable to secure compiler output directory %s after %d attempts: %w", outputDir, maxAttempts, lastErr)
}

// runtimeDirComponents returns the absolute form of outputDir and every ancestor
// up to the filesystem root, ordered root-first: index 0 is the filesystem root
// ("/") and the last element is the leaf output directory.
func runtimeDirComponents(outputDir string) ([]string, error) {
	abs, err := filepath.Abs(outputDir)
	if err != nil {
		return nil, fmt.Errorf("unable to resolve compiler output directory %s: %w", outputDir, err)
	}
	var components []string
	for p := filepath.Clean(abs); ; {
		components = append(components, p)
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
	// Collected leaf-first; reverse so the walk below can range root -> leaf.
	slices.Reverse(components)
	return components, nil
}

// ensureRuntimeDirChain walks the components root -> leaf, enforcing
// verifyDirComponent on each one that already exists, creating each one that does
// not, and reclaiming (rename-aside + delete + recreate) a repairable wrong-owner
// component. Verifying the existing prefix before creating the missing tail is
// what makes creation symlink-safe: os.Mkdir of a component only ever resolves
// through ancestors already verified as real (non-symlink) root-owned
// directories.
//
// It returns whether the failure it hit (if any) is worth retrying: a race on
// creation (os.Mkdir returning EEXIST because something appeared underneath a
// verified parent) is retryable; a policy refusal or a failed reclaim is
// terminal.
//
// A component is reclaimable only when all of the following hold, so the repair
// stays conservative and never damages unrelated directories:
//   - we are running as root (system-probe's normal state); a non-root process
//     must not start renaming directories it happens not to own,
//   - the component lies strictly below a verified root-owned sticky directory
//     (a shared, writable-by-anyone parent such as /var/tmp, whose sticky bit
//     means only root can rename the entries beneath it),
//   - the component lies inside the agent's own dedicated subtree (at or below a
//     component named dedicatedDirName). Without this, the first insecure
//     ancestor on a custom output_dir could be a shared directory unrelated to
//     the agent (e.g. /var/tmp/shared), which reclaim would then rename aside and
//     recursively delete along with its contents, and
//   - the component is itself a real directory that merely has the wrong owner or
//     permissions. A symlink or non-directory at a path component is unexpected
//     rather than a routine leftover, so it is still refused (fail-closed)
//     instead of being rewritten.
func ensureRuntimeDirChain(components []string) (retryable bool, err error) {
	// Walk root -> leaf so we know a component sits below a verified sticky
	// boundary, and inside the agent's dedicated subtree, before we decide
	// whether its failure is reclaimable.
	belowStickyBoundary := false
	inDedicatedSubtree := false
	for i, p := range components {
		// A component named dedicatedDirName marks the start of the agent's own
		// subtree; it and anything deeper may be repaired. Set this before the
		// verify below so the datadog-agent component itself is repairable.
		if filepath.Base(p) == dedicatedDirName {
			inDedicatedSubtree = true
		}

		info, lerr := os.Lstat(p)
		if os.IsNotExist(lerr) {
			// This component and everything below it are the missing tail. Every
			// existing ancestor was verified above as a real (non-symlink)
			// root-owned directory, so os.Mkdir resolves only through verified
			// directories and will not create through a symlink at p.
			if mErr := os.Mkdir(p, 0700); mErr != nil {
				// A racing actor may have created p first; re-walking re-checks it.
				return true, fmt.Errorf("unable to create compiler output directory component %s: %w", p, mErr)
			}
			continue
		}
		if lerr != nil {
			return false, fmt.Errorf("unable to verify compiler output directory component %s: %w", p, lerr)
		}

		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return false, fmt.Errorf("unable to read ownership of compiler output directory component %s", p)
		}
		// The last component is the leaf output directory, which is held to the
		// stricter no-group/other-write rule (see verifyDirComponent).
		if verr := verifyDirComponent(p, info.Mode(), stat.Uid, i == len(components)-1); verr != nil {
			// Reclaim only a real, wrong-owner/permission directory that is below
			// the sticky boundary, inside the agent's dedicated subtree, and only
			// as root. info.Mode().IsDir() is false for a symlink or a regular
			// file, so those remain fail-closed; requiring the dedicated subtree
			// keeps reclaim from ever deleting a shared, non-agent directory.
			healable := belowStickyBoundary && inDedicatedSubtree && os.Geteuid() == 0 && info.Mode().IsDir()
			if !healable {
				return false, verr
			}
			if rErr := reclaimDirComponent(p); rErr != nil {
				return false, fmt.Errorf("%w (attempted reclaim failed: %v)", verr, rErr)
			}
			logRuntimeDirReclaimed(p)
			// Recreate this component fresh; deeper components were removed with
			// the reclaimed subtree and are created as the walk continues.
			if mErr := os.Mkdir(p, 0700); mErr != nil {
				return true, fmt.Errorf("unable to recreate compiler output directory component %s after reclaim: %w", p, mErr)
			}
			continue
		}
		// This component is verified good. If it is a sticky directory it marks
		// the shared-parent boundary; anything deeper may be repaired.
		if info.Mode()&os.ModeSticky != 0 {
			belowStickyBoundary = true
		}
	}
	return false, nil
}

// reclaimDirComponent moves a stale path component aside and removes it. The
// caller must only invoke this for a component that ensureRuntimeDirChain
// reported as repairable (below a verified root-owned sticky directory and inside
// the agent's dedicated datadog-agent subtree), which guarantees the parent is
// root-owned so root can rename the entry regardless of who owns it. Renaming
// aside (never reusing or chown-ing the existing entry) ensures we do not follow
// a symlink or reuse whatever the entry previously contained; even if the
// component's non-root owner races us (they may rename their own entry in a
// sticky parent), os.Rename does not dereference a symlink at p and we recreate
// the path fresh afterwards, so the worst case is a transient error, never
// loading or writing through an attacker-controlled path.
func reclaimDirComponent(p string) error {
	// os.Rename does not follow a symlink at p: a symlinked component is moved as
	// the link itself. The aside name is in the same (root-owned) parent so the
	// rename stays within one directory. Use the root-namespace PID: os.Getpid
	// returns a namespaced PID inside a container, which is not unique across the
	// host and could collide with a concurrent reclaim from another namespace.
	pid, err := kernel.RootNSPID()
	if err != nil {
		pid = os.Getpid()
	}
	aside := fmt.Sprintf("%s.reclaimed-%d-%d", p, pid, time.Now().UnixNano())
	if err := os.Rename(p, aside); err != nil {
		return fmt.Errorf("unable to move aside stale component %s: %w", p, err)
	}
	// Best-effort cleanup of the moved-aside tree. os.RemoveAll uses O_NOFOLLOW
	// openat internally, so it will not traverse symlinks beneath it.
	if err := os.RemoveAll(aside); err != nil {
		log.Debugf("unable to remove reclaimed component %s: %s", aside, err)
	}
	return nil
}

// verifyDirComponent enforces the runtime-directory policy on a single path
// component, described by the mode returned from Lstat and its owning uid: it
// must be a real (non-symlink) directory owned by root and not writable by other
// users. It is kept separate from the filesystem walk above so the policy can be
// unit-tested with synthetic values, without needing to build a root-owned
// directory tree.
//
// The group/other-writable-but-sticky exception applies to ancestors only. It
// exists for the shared parent the default lives under (/var/tmp is 1777), which
// the agent does not own and cannot expect to tighten. The leaf output directory
// is different: it is where object files are written and re-read under
// predictable names, so a group/other-writable leaf would let another user
// interpose an entry at one of those names between our check and the compiler's
// write. The leaf must therefore have no group/other write bits regardless of the
// sticky bit; isLeaf selects that stricter rule.
func verifyDirComponent(p string, mode os.FileMode, uid uint32, isLeaf bool) error {
	if mode&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to use compiler output directory: %s is a symlink", p)
	}
	if !mode.IsDir() {
		return fmt.Errorf("refusing to use compiler output directory: %s is not a directory", p)
	}
	if uid != 0 {
		return fmt.Errorf("refusing to use compiler output directory: %s is not owned by root (uid=%d)", p, uid)
	}
	if mode.Perm()&0022 != 0 && (isLeaf || mode&os.ModeSticky == 0) {
		return fmt.Errorf("refusing to use compiler output directory: %s is writable by non-root (mode=%#o)", p, mode.Perm())
	}
	return nil
}

// creates a ram backed file from the given reader. The file is made immutable
func createProtectedFile(name, runtimeDir string, source io.Reader) (ProtectedFile, error) {
	protectedFile, err := NewProtectedFile(name, runtimeDir, source)
	if err != nil {
		return nil, fmt.Errorf("failed to create protected file: %w", err)
	}

	return protectedFile, err
}

// verify reads the asset from the reader and verifies the content hash matches what is expected.
func (a *asset) verify(source ProtectedFile) error {
	sum, err := sha256Reader(source)
	if err != nil {
		return fmt.Errorf("hash file %s: %w", source.Name(), err)
	}
	if sum != a.hash {
		return errors.New("file content hash does not match expected value")
	}
	return nil
}

func sha256Reader(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// GetTelemetry returns the compilation telemetry for this asset
func (a *asset) GetTelemetry() CompilationTelemetry {
	return a.tm
}
