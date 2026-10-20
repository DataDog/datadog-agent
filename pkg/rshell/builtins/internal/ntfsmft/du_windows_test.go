// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package ntfsmft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// -------------------------------------------------------------------------
// Admin-rights gate
// -------------------------------------------------------------------------

// requireAdmin skips the test unless the process is in the local Administrators
// group; Scan opens \\.\<drive>:, which requires elevation.
func requireAdmin(t *testing.T) {
	t.Helper()
	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid,
	)
	if err != nil {
		t.Skipf("AllocateAndInitializeSid failed: %v", err)
	}
	defer windows.FreeSid(sid)
	member, err := windows.Token(0).IsMember(sid)
	if err != nil {
		t.Skipf("Token.IsMember failed: %v", err)
	}
	if !member {
		t.Skip("requires Administrator privileges (raw \\.\\<drive>: open)")
	}
}

// -------------------------------------------------------------------------
// Test helpers
// -------------------------------------------------------------------------

const scanTimeout = 2 * time.Minute

type testVHDFixture struct {
	VhdPath  string
	TestRoot string
}

var (
	scanFixtureOnce    sync.Once
	scanFixtureErr     error
	scanTestRoot       string
	scanFixtureCleanup func() error
)

// TestMain cleans up a fixture only when a raw-MFT integration test initialized
// one. It must not provision a VHD itself: parser and other unit tests run
// without elevation, and DiskPart's VDS initialization can fail in that token.
func TestMain(m *testing.M) {
	code := m.Run()
	if scanFixtureCleanup != nil {
		if err := scanFixtureCleanup(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = 1
		}
	}
	os.Exit(code)
}

func configureTestVHD() (root string, cleanup func() error, err error) {
	if root := os.Getenv("RSHELL_NTFSDU_TEST_ROOT"); root != "" {
		if err := validateTestRoot(root); err != nil {
			return "", nil, err
		}
		return root, nil, nil
	}
	script, err := findTestVHDScript()
	if err != nil {
		return "", nil, err
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-File", script,
		"-Action", "Create", "-VhdDirectory", os.TempDir(), "-AsJson")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", nil, fmt.Errorf("create NTFS test VHD: %w: %s", err, out)
	}
	var fixture testVHDFixture
	if err := json.Unmarshal(out, &fixture); err != nil {
		parseErr := fmt.Errorf("parse NTFS test VHD fixture: %w: %s", err, out)
		if fixture.VhdPath != "" {
			return "", nil, errors.Join(parseErr, destroyTestVHD(script, fixture.VhdPath))
		}
		return "", nil, parseErr
	}
	if fixture.VhdPath == "" {
		return "", nil, errors.New("create NTFS test VHD returned an empty VhdPath")
	}
	destroy := func() error {
		return destroyTestVHD(script, fixture.VhdPath)
	}
	keepFixture := false
	defer func() {
		if !keepFixture {
			if cleanupErr := destroy(); cleanupErr != nil {
				err = errors.Join(err, cleanupErr)
			}
		}
	}()
	if err = validateTestRoot(fixture.TestRoot); err != nil {
		return "", nil, err
	}
	keepFixture = true
	return fixture.TestRoot, destroy, nil
}

func destroyTestVHD(script, vhdPath string) error {
	out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-File", script,
		"-Action", "Destroy", "-VhdPath", vhdPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("destroy NTFS test VHD %q: %w: %s", vhdPath, err, out)
	}
	return nil
}

func validateTestRoot(root string) error {
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("stat RSHELL_NTFSDU_TEST_ROOT %q: %w", root, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("RSHELL_NTFSDU_TEST_ROOT %q is not a directory", root)
	}
	return nil
}

// scanTempDir creates a test-owned directory on the selected NTFS test volume.
// It deliberately does not alter TMP or TEMP for the test process.
func scanTempDir(t *testing.T) string {
	t.Helper()
	requireAdmin(t)
	scanFixtureOnce.Do(func() {
		scanTestRoot, scanFixtureCleanup, scanFixtureErr = configureTestVHD()
	})
	if scanFixtureErr != nil {
		t.Skipf("cannot provision NTFS test VHD fixture: %v", scanFixtureErr)
	}
	dir, err := os.MkdirTemp(scanTestRoot, "ntfsmft-")
	if err != nil {
		t.Fatalf("mkdir test directory under %q: %v", scanTestRoot, err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("remove test directory %q: %v", dir, err)
		}
	})
	return dir
}

func findTestVHDScript() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory for NTFS test VHD script: %w", err)
	}
	for {
		script := filepath.Join(dir, "scripts", "ntfsmft-test-vhd.ps1")
		if info, err := os.Stat(script); err == nil && !info.IsDir() {
			return script, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("cannot find scripts/ntfsmft-test-vhd.ps1 from the test working directory")
		}
		dir = parent
	}
}

// scanOrSkip runs Scan(target). Requires admin (checked upfront). It skips
// unsupported raw-volume environments, but fails a timed-out scan so a slow
// volume or scanner regression cannot consume the package-wide test deadline.
func scanOrSkip(t *testing.T, target string, opts Options) *Result {
	t.Helper()
	requireAdmin(t)
	ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
	defer cancel()
	res, err := Scan(ctx, target, opts)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Scan(%q) exceeded %s", target, scanTimeout)
		}
		if isRawMFTUnsupported(err) {
			t.Skipf("raw MFT access not supported on this volume: %v", err)
		}
		t.Fatalf("Scan: %v", err)
	}
	return res
}

// isRawMFTUnsupported reports whether the error indicates the underlying
// volume / sandbox does not permit raw MFT access. Maps to:
//   - ERROR_NOT_SUPPORTED (50) — typical for container filesystem layers
//   - ERROR_INVALID_FUNCTION (1) — non-NTFS volume (e.g. ReFS)
//   - ERROR_ACCESS_DENIED (5) — sandbox blocks the volume handle
func isRawMFTUnsupported(err error) bool {
	for _, code := range []windows.Errno{
		windows.ERROR_NOT_SUPPORTED,
		windows.ERROR_INVALID_FUNCTION,
		windows.ERROR_ACCESS_DENIED,
	} {
		if errors.Is(err, code) {
			return true
		}
	}
	return false
}

// flushMetadataToDisk forces NTFS to flush the test file's $DATA, $FILE_NAME,
// and parent directory $INDEX entries to the on-disk MFT so the raw-volume
// scan can see them.
func flushMetadataToDisk(t *testing.T, path string) {
	t.Helper()
	pw, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatalf("utf16(%q): %v", path, err)
	}
	h, err := windows.CreateFile(
		pw, windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0,
	)
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	_ = windows.FlushFileBuffers(h)
}

// allocatedSize returns the on-disk allocation for a file via
// GetFileInformationByHandleEx(FileStandardInfo).
func allocatedSize(t *testing.T, path string) int64 {
	t.Helper()
	pw, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatalf("utf16(%q): %v", path, err)
	}
	h, err := windows.CreateFile(pw, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatalf("open(%q): %v", path, err)
	}
	defer windows.CloseHandle(h)

	var info struct {
		AllocationSize int64
		EndOfFile      int64
		NumberOfLinks  uint32
		DeletePending  bool
		Directory      bool
		_              [2]byte
	}
	const fileStandardInfo = 1
	if err := windows.GetFileInformationByHandleEx(h, fileStandardInfo,
		(*byte)(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		t.Fatalf("GetFileInformationByHandleEx(%q): %v", path, err)
	}
	return info.AllocationSize
}

// writeFile writes data to path, flushes metadata, and returns the on-disk
// allocated size.
func writeFile(t *testing.T, path string, data []byte) int64 {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent of %q: %v", path, err)
	}
	pw, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatalf("utf16(%q): %v", path, err)
	}
	h, err := windows.CreateFile(pw,
		windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ,
		nil, windows.CREATE_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("create(%q): %v", path, err)
	}
	if len(data) > 0 {
		var n uint32
		if err := windows.WriteFile(h, data, &n, nil); err != nil {
			windows.CloseHandle(h)
			t.Fatalf("write(%q): %v", path, err)
		}
	}
	_ = windows.FlushFileBuffers(h)
	windows.CloseHandle(h)
	flushMetadataToDisk(t, filepath.Dir(path))
	return allocatedSize(t, path)
}

func createHardLink(t *testing.T, newPath, existingPath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		t.Fatalf("mkdir parent of %q: %v", newPath, err)
	}
	npw, err := windows.UTF16PtrFromString(newPath)
	if err != nil {
		t.Fatalf("utf16(%q): %v", newPath, err)
	}
	epw, err := windows.UTF16PtrFromString(existingPath)
	if err != nil {
		t.Fatalf("utf16(%q): %v", existingPath, err)
	}
	if err := windows.CreateHardLink(npw, epw, 0); err != nil {
		t.Fatalf("CreateHardLink(%q -> %q): %v", newPath, existingPath, err)
	}
	flushMetadataToDisk(t, filepath.Dir(newPath))
}

const fsctlSetSparse = 0x000900C4
const fsctlSetCompression = 0x0009C040

// createSparseFile makes a fully sparse file of the given virtual size.
func createSparseFile(t *testing.T, path string, virtualSize int64) int64 {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent of %q: %v", path, err)
	}
	pw, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatalf("utf16(%q): %v", path, err)
	}
	h, err := windows.CreateFile(pw,
		windows.GENERIC_WRITE, windows.FILE_SHARE_READ,
		nil, windows.CREATE_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("create(%q): %v", path, err)
	}
	defer windows.CloseHandle(h)

	var n uint32
	if err := windows.DeviceIoControl(h, fsctlSetSparse, nil, 0, nil, 0, &n, nil); err != nil {
		t.Fatalf("FSCTL_SET_SPARSE(%q): %v", path, err)
	}

	hi := int32(virtualSize >> 32)
	if _, err := windows.SetFilePointer(h, int32(virtualSize&0xFFFFFFFF),
		&hi, windows.FILE_BEGIN); err != nil {
		t.Fatalf("SetFilePointer(%q): %v", path, err)
	}
	if err := windows.SetEndOfFile(h); err != nil {
		t.Fatalf("SetEndOfFile(%q): %v", path, err)
	}
	if err := windows.FlushFileBuffers(h); err != nil {
		t.Fatalf("FlushFileBuffers(%q): %v", path, err)
	}

	flushMetadataToDisk(t, filepath.Dir(path))
	return allocatedSize(t, path)
}

// createCompressedFile creates a file, marks it compressed, then writes
// highly-compressible data (zeros).
func createCompressedFile(t *testing.T, path string, dataSize int) int64 {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent of %q: %v", path, err)
	}
	pw, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatalf("utf16(%q): %v", path, err)
	}
	h, err := windows.CreateFile(pw,
		windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ,
		nil, windows.CREATE_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("create(%q): %v", path, err)
	}

	const compressionFormatDefault uint16 = 1
	cf := compressionFormatDefault
	var n uint32
	if err := windows.DeviceIoControl(h, fsctlSetCompression,
		(*byte)(unsafe.Pointer(&cf)), 2, nil, 0, &n, nil); err != nil {
		windows.CloseHandle(h)
		t.Fatalf("FSCTL_SET_COMPRESSION(%q): %v", path, err)
	}

	if dataSize > 0 {
		zeros := make([]byte, dataSize)
		if err := windows.WriteFile(h, zeros, &n, nil); err != nil {
			windows.CloseHandle(h)
			t.Fatalf("write(%q): %v", path, err)
		}
	}
	_ = windows.FlushFileBuffers(h)
	windows.CloseHandle(h)
	flushMetadataToDisk(t, filepath.Dir(path))
	return allocatedSize(t, path)
}

// -------------------------------------------------------------------------
// Tests
// -------------------------------------------------------------------------

func TestScan_BasicDirectories(t *testing.T) {
	root := scanTempDir(t)

	a1 := writeFile(t, filepath.Join(root, "A", "file1.bin"), make([]byte, 4096))
	a2 := writeFile(t, filepath.Join(root, "A", "file2.bin"), make([]byte, 8192))
	b1 := writeFile(t, filepath.Join(root, "B", "file3.bin"), make([]byte, 4096))

	res := scanOrSkip(t, root, Options{TreeDepth: 1})

	wantA := a1 + a2
	wantB := b1
	if got := findTreeChild(t, res.Tree, "A").Size; got != wantA {
		t.Errorf("child A = %d, want %d", got, wantA)
	}
	if got := findTreeChild(t, res.Tree, "B").Size; got != wantB {
		t.Errorf("child B = %d, want %d", got, wantB)
	}
	wantSubtree := wantA + wantB
	if res.Subtree != wantSubtree {
		t.Errorf("Subtree = %d, want %d", res.Subtree, wantSubtree)
	}
	if res.MultiParentFiles != 0 {
		t.Errorf("MultiParentFiles = %d, want 0", res.MultiParentFiles)
	}
}

// A file directly under the target counts toward the root totals but is not a
// child node (only directories are); the child directory reports only its own
// subtree.
func TestScan_FilesDirectlyUnderTarget(t *testing.T) {
	root := scanTempDir(t)

	direct := writeFile(t, filepath.Join(root, "loose.bin"), make([]byte, 4096))
	a1 := writeFile(t, filepath.Join(root, "A", "x.bin"), make([]byte, 8192))

	res := scanOrSkip(t, root, Options{TreeDepth: 1})

	if got := findTreeChild(t, res.Tree, "A").Size; got != a1 {
		t.Errorf("child A = %d, want %d (must exclude the file directly under target)", got, a1)
	}
	if res.Subtree != direct+a1 {
		t.Errorf("Subtree = %d, want %d", res.Subtree, direct+a1)
	}
	if res.Tree.Size != direct+a1 {
		t.Errorf("root Size = %d, want %d (whole subtree)", res.Tree.Size, direct+a1)
	}
}

func TestScan_NestedDirectories(t *testing.T) {
	root := scanTempDir(t)

	deep := writeFile(t, filepath.Join(root, "A", "sub1", "sub2", "deep.bin"), make([]byte, 4096))

	res := scanOrSkip(t, root, Options{TreeDepth: 1})

	if got := findTreeChild(t, res.Tree, "A").Size; got != deep {
		t.Errorf("child A = %d, want %d (file in A/sub1/sub2/)", got, deep)
	}
	if res.Subtree != deep {
		t.Errorf("Subtree = %d, want %d", res.Subtree, deep)
	}
}

func TestScan_HardlinkSameBucket(t *testing.T) {
	root := scanTempDir(t)

	primary := filepath.Join(root, "A", "primary.bin")
	link := filepath.Join(root, "A", "secondary.bin")

	sz := writeFile(t, primary, make([]byte, 4096))
	createHardLink(t, link, primary)

	res := scanOrSkip(t, root, Options{TreeDepth: 1})

	if got := findTreeChild(t, res.Tree, "A").Size; got != sz {
		t.Errorf("child A = %d, want %d (hard-linked file in same directory)", got, sz)
	}
	if res.Subtree != sz {
		t.Errorf("Subtree = %d, want %d (dedup)", res.Subtree, sz)
	}
	if res.MultiParentFiles != 0 {
		t.Errorf("MultiParentFiles = %d, want 0 (both links share one parent)", res.MultiParentFiles)
	}
}

func TestScan_HardlinkAcrossChildren(t *testing.T) {
	root := scanTempDir(t)

	primary := filepath.Join(root, "A", "shared.bin")
	link := filepath.Join(root, "B", "shared.bin")

	sz := writeFile(t, primary, make([]byte, 4096))
	if err := os.MkdirAll(filepath.Join(root, "B"), 0o755); err != nil {
		t.Fatal(err)
	}
	createHardLink(t, link, primary)

	res := scanOrSkip(t, root, Options{TreeDepth: 1})

	if got := findTreeChild(t, res.Tree, "A").Size; got != sz {
		t.Errorf("child A = %d, want %d (cross-child hardlink)", got, sz)
	}
	if got := findTreeChild(t, res.Tree, "B").Size; got != sz {
		t.Errorf("child B = %d, want %d (cross-child hardlink)", got, sz)
	}
	if res.Subtree != sz {
		t.Errorf("Subtree = %d, want %d (cross-child should dedup)", res.Subtree, sz)
	}
	if res.MultiParentFiles != 1 {
		t.Errorf("MultiParentFiles = %d, want 1", res.MultiParentFiles)
	}
}

func TestScan_HardlinkTargetAndChild(t *testing.T) {
	root := scanTempDir(t)

	primary := filepath.Join(root, "loose.bin")
	link := filepath.Join(root, "A", "linked.bin")

	sz := writeFile(t, primary, make([]byte, 4096))
	createHardLink(t, link, primary)

	res := scanOrSkip(t, root, Options{TreeDepth: 1})

	if got := findTreeChild(t, res.Tree, "A").Size; got != sz {
		t.Errorf("child A = %d, want %d", got, sz)
	}
	if res.Subtree != sz {
		t.Errorf("Subtree = %d, want %d", res.Subtree, sz)
	}
	if res.MultiParentFiles != 1 {
		t.Errorf("MultiParentFiles = %d, want 1 (target+child)", res.MultiParentFiles)
	}
}

func TestScan_SparseFile_AllocatedNotApparent(t *testing.T) {
	root := scanTempDir(t)

	const virtual = 64 * 1024 * 1024
	allocated := createSparseFile(t, filepath.Join(root, "A", "sparse.bin"), virtual)

	res := scanOrSkip(t, root, Options{TreeDepth: 1})

	if got := findTreeChild(t, res.Tree, "A").Size; got != allocated {
		t.Errorf("child A allocated = %d, want %d (sparse: actual on-disk)", got, allocated)
	}
	if allocated >= virtual/4 {
		t.Errorf("sparse file allocated %d is not significantly smaller than virtual %d — sparseness check failed",
			allocated, virtual)
	}
}

func TestScan_SparseFile_Apparent(t *testing.T) {
	root := scanTempDir(t)

	const virtual = 64 * 1024 * 1024
	createSparseFile(t, filepath.Join(root, "A", "sparse.bin"), virtual)

	res := scanOrSkip(t, root, Options{ShowApparent: true, TreeDepth: 1})

	if got := findTreeChild(t, res.Tree, "A").Size; got != virtual {
		t.Errorf("child A apparent = %d, want %d (apparent: virtual size)", got, virtual)
	}
}

func TestScan_CompressedFile(t *testing.T) {
	root := scanTempDir(t)

	const dataSize = 256 * 1024
	allocated := createCompressedFile(t, filepath.Join(root, "A", "compressed.bin"), dataSize)

	res := scanOrSkip(t, root, Options{TreeDepth: 1})

	if got := findTreeChild(t, res.Tree, "A").Size; got != allocated {
		t.Errorf("child A allocated = %d, want %d (compressed)", got, allocated)
	}
	if allocated >= int64(dataSize) {
		t.Errorf("compressed file allocated %d is not smaller than data %d — compression didn't kick in",
			allocated, dataSize)
	}
}

func TestScan_ResidentSmallFile(t *testing.T) {
	root := scanTempDir(t)

	sz := writeFile(t, filepath.Join(root, "A", "tiny.bin"), make([]byte, 100))

	res := scanOrSkip(t, root, Options{ShowApparent: true, TreeDepth: 1})

	if got := findTreeChild(t, res.Tree, "A").Size; got != 100 {
		t.Errorf("child A apparent = %d, want 100 (resident $DATA)", got)
	}
	_ = sz
}

func TestScan_TargetWithNoChildDirs(t *testing.T) {
	root := scanTempDir(t)

	direct := writeFile(t, filepath.Join(root, "only.bin"), make([]byte, 4096))

	res := scanOrSkip(t, root, Options{TreeDepth: 1})

	if res.Tree == nil {
		t.Fatal("Tree is nil with TreeDepth=1")
	}
	if len(res.Tree.Children) != 0 {
		t.Errorf("Tree.Children = %+v, want empty (no child dirs)", res.Tree.Children)
	}
	if res.Tree.Files != 1 {
		t.Errorf("root Files = %d, want 1", res.Tree.Files)
	}
	if res.Subtree != direct {
		t.Errorf("Subtree = %d, want %d", res.Subtree, direct)
	}
}

func TestScan_EmptyTarget(t *testing.T) {
	root := scanTempDir(t)

	res := scanOrSkip(t, root, Options{TreeDepth: 1})

	if res.Tree == nil {
		t.Fatal("Tree is nil with TreeDepth=1")
	}
	if len(res.Tree.Children) != 0 {
		t.Errorf("Tree.Children = %+v, want empty", res.Tree.Children)
	}
	if res.Subtree != 0 {
		t.Errorf("Subtree = %d, want 0", res.Subtree)
	}
}

func writeStream(t *testing.T, path, streamName string, data []byte) {
	t.Helper()
	full := path
	if streamName != "" {
		full = path + ":" + streamName
	}
	pw, err := windows.UTF16PtrFromString(full)
	if err != nil {
		t.Fatalf("utf16(%q): %v", full, err)
	}
	h, err := windows.CreateFile(pw,
		windows.GENERIC_WRITE, windows.FILE_SHARE_READ,
		nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("create(%q): %v", full, err)
	}
	if len(data) > 0 {
		var n uint32
		if err := windows.WriteFile(h, data, &n, nil); err != nil {
			windows.CloseHandle(h)
			t.Fatalf("write(%q): %v", full, err)
		}
	}
	_ = windows.FlushFileBuffers(h)
	windows.CloseHandle(h)
}

func TestScan_FileWithAlternateDataStream(t *testing.T) {
	root := scanTempDir(t)

	const mainBytes = 8192
	const adsBytes = 154
	mainPath := filepath.Join(root, "A", "downloaded.bin")

	if err := os.MkdirAll(filepath.Dir(mainPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeStream(t, mainPath, "", make([]byte, mainBytes))
	writeStream(t, mainPath, "Zone.Identifier", make([]byte, adsBytes))
	flushMetadataToDisk(t, filepath.Dir(mainPath))

	if got := allocatedSize(t, mainPath); got != mainBytes {
		t.Fatalf("setup: unnamed stream allocated = %d, want %d", got, mainBytes)
	}

	res := scanOrSkip(t, root, Options{TreeDepth: 1})

	want := int64(mainBytes + adsBytes)
	if got := findTreeChild(t, res.Tree, "A").Size; got != want {
		t.Errorf("child A = %d, want %d (main+ADS sum)", got, want)
	}
}

func TestEnumerateImmediateChildren_FlagsReparsePoints(t *testing.T) {
	root := scanTempDir(t)

	if err := os.MkdirAll(filepath.Join(root, "A"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "A"), filepath.Join(root, "B")); err != nil {
		t.Skipf("cannot create directory symlink: %v", err)
	}

	children, err := enumerateImmediateChildren(context.Background(), root+`\`)
	if err != nil {
		t.Fatalf("enumerateImmediateChildren: %v", err)
	}

	got := map[string]bool{}
	for _, c := range children {
		got[c.name] = c.reparse
	}
	if r, ok := got["A"]; !ok || r {
		t.Errorf("child A: reparse=%v ok=%v, want false/true", r, ok)
	}
	if r, ok := got["B"]; !ok || !r {
		t.Errorf("child B: reparse=%v ok=%v, want true/true", r, ok)
	}
}

func TestEnumerateImmediateChildrenHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := enumerateImmediateChildren(ctx, t.TempDir()+`\`)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("enumerateImmediateChildren error = %v, want context.Canceled", err)
	}
}

func TestResolveScopeIndicesDepthZeroSkipsChildEnumeration(t *testing.T) {
	called := false
	s := &scanState{
		opts:         Options{TreeDepth: 0},
		res:          &Result{},
		abs:          `C:\target\`,
		targetIdx:    42,
		volumeSerial: 7,
		enumerateChildren: func(context.Context, string) ([]childInfo, error) {
			called = true
			return nil, errors.New("child enumeration must be skipped at depth zero")
		},
	}
	if err := s.resolveScopeIndices(context.Background()); err != nil {
		t.Fatalf("resolveScopeIndices: %v", err)
	}
	if called {
		t.Fatal("depth-zero scan called child enumeration")
	}
	if len(s.children) != 0 || len(s.bucketByIdx) != 0 {
		t.Fatalf("depth-zero children/buckets = %#v/%#v, want empty", s.children, s.bucketByIdx)
	}
}

func TestGetMFTIdxFromPath_DoesNotFollowReparsePoint(t *testing.T) {
	root := scanTempDir(t)

	if err := os.MkdirAll(filepath.Join(root, "A"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "A"), filepath.Join(root, "B")); err != nil {
		t.Skipf("cannot create directory symlink: %v", err)
	}

	idxA, err := getMFTIdxFromPath(filepath.Join(root, "A"))
	if err != nil {
		t.Fatalf("idx A: %v", err)
	}
	idxB, err := getMFTIdxFromPath(filepath.Join(root, "B"))
	if err != nil {
		t.Fatalf("idx B: %v", err)
	}
	if idxA == idxB {
		t.Errorf("idx A == idx B (%d) — CreateFile is following the reparse point", idxA)
	}
}

func TestScan_DriveRootFromDeepCwd(t *testing.T) {
	sub := t.TempDir()
	if len(sub) < 3 || sub[1] != ':' {
		t.Skipf("temp dir %q is not on a Windows drive", sub)
	}
	driveRoot := sub[:2] + `\`

	t.Chdir(sub)

	idx, err := getMFTIdxFromPath(driveRoot)
	if err != nil {
		t.Fatalf("getMFTIdxFromPath(%q): %v", driveRoot, err)
	}
	if idx != rootDirMFTIndex {
		t.Errorf("getMFTIdxFromPath(%q) = %d, want %d (NTFS volume root) — cwd %q leaked into resolution",
			driveRoot, idx, rootDirMFTIndex, sub)
	}
}

// -------------------------------------------------------------------------
// Top-N files / extensions / find predicates
// -------------------------------------------------------------------------

func TestScan_TopFilesAndExtensions(t *testing.T) {
	root := scanTempDir(t)

	writeFile(t, filepath.Join(root, "A", "small.txt"), make([]byte, 1024))
	writeFile(t, filepath.Join(root, "A", "medium.log"), make([]byte, 16*1024))
	writeFile(t, filepath.Join(root, "B", "large.dat"), make([]byte, 64*1024))
	writeFile(t, filepath.Join(root, "B", "extra.log"), make([]byte, 8*1024))

	res := scanOrSkip(t, root, Options{
		ShowApparent:  true,
		TopFiles:      10,
		TopExtensions: 10,
	})

	if len(res.TopFiles) < 4 {
		t.Fatalf("TopFiles len = %d, want >= 4", len(res.TopFiles))
	}
	// Sorted descending by size.
	for i := 1; i < len(res.TopFiles); i++ {
		if res.TopFiles[i-1].Size < res.TopFiles[i].Size {
			t.Errorf("TopFiles not sorted descending: %+v", res.TopFiles)
			break
		}
	}
	if res.TopFiles[0].Size != 64*1024 {
		t.Errorf("TopFiles[0].Size = %d, want %d (large.dat)", res.TopFiles[0].Size, 64*1024)
	}

	extByName := map[string]ExtensionEntry{}
	for _, e := range res.TopExtensions {
		extByName[e.Ext] = e
	}
	if e := extByName["dat"]; e.Size != 64*1024 || e.Count != 1 {
		t.Errorf("ext dat = %+v, want size=%d count=1", e, 64*1024)
	}
	if e := extByName["log"]; e.Size != 24*1024 || e.Count != 2 {
		t.Errorf("ext log = %+v, want size=%d count=2", e, 24*1024)
	}
	if e := extByName["txt"]; e.Size != 1024 || e.Count != 1 {
		t.Errorf("ext txt = %+v, want size=1024 count=1", e)
	}
}

func TestScan_TopFilesMinFileSize(t *testing.T) {
	root := scanTempDir(t)

	writeFile(t, filepath.Join(root, "A", "tiny.bin"), make([]byte, 100))
	writeFile(t, filepath.Join(root, "A", "big.bin"), make([]byte, 32*1024))

	res := scanOrSkip(t, root, Options{
		ShowApparent: true,
		TopFiles:     10,
		MinFileSize:  16 * 1024,
	})

	if len(res.TopFiles) != 1 {
		t.Fatalf("TopFiles len = %d, want 1 (only big.bin qualifies)", len(res.TopFiles))
	}
	if res.TopFiles[0].Size != 32*1024 {
		t.Errorf("TopFiles[0].Size = %d, want %d", res.TopFiles[0].Size, 32*1024)
	}
}

// MinFileSize drops extensions whose AGGREGATED total is below the threshold
// from TopExtensions (this is an aggregate filter, not a per-file one: many
// small files of one extension can still sum above the floor).
func TestScan_TopExtMinFileSize(t *testing.T) {
	root := scanTempDir(t)

	// .log aggregates to 20 KiB (below the 32 KiB floor); .dat is 64 KiB (above).
	writeFile(t, filepath.Join(root, "A", "a.log"), make([]byte, 10*1024))
	writeFile(t, filepath.Join(root, "A", "b.log"), make([]byte, 10*1024))
	writeFile(t, filepath.Join(root, "A", "big.dat"), make([]byte, 64*1024))

	res := scanOrSkip(t, root, Options{
		ShowApparent:  true,
		TopExtensions: 10,
		MinFileSize:   32 * 1024,
	})

	byExt := map[string]ExtensionEntry{}
	for _, e := range res.TopExtensions {
		byExt[e.Ext] = e
	}
	if e, ok := byExt["dat"]; !ok || e.Size != 64*1024 {
		t.Errorf("ext dat = %+v (ok=%v), want size=%d (above floor)", e, ok, 64*1024)
	}
	if e, ok := byExt["log"]; ok {
		t.Errorf("ext log present with size %d but 20 KiB aggregate is below the 32 KiB floor", e.Size)
	}
}

func TestScan_FindByExtension(t *testing.T) {
	root := scanTempDir(t)

	writeFile(t, filepath.Join(root, "A", "crash.dmp"), make([]byte, 4096))
	writeFile(t, filepath.Join(root, "A", "trace.etl"), make([]byte, 8192))
	writeFile(t, filepath.Join(root, "A", "ignore.txt"), make([]byte, 1024))
	writeFile(t, filepath.Join(root, "B", "other.dmp"), make([]byte, 2048))

	res := scanOrSkip(t, root, Options{
		ShowApparent: true,
		Finds: []FindQuery{
			{Type: "ext", Value: ".dmp,.etl", Limit: 10},
		},
	})

	if len(res.FindResults) != 1 {
		t.Fatalf("FindResults len = %d, want 1", len(res.FindResults))
	}
	matches := res.FindResults[0].Matches
	if len(matches) != 3 {
		t.Fatalf("matches len = %d, want 3; got %+v", len(matches), matches)
	}
	for i := 1; i < len(matches); i++ {
		if matches[i-1].Size < matches[i].Size {
			t.Errorf("matches not sorted descending: %+v", matches)
			break
		}
	}
	if matches[0].Size != 8192 {
		t.Errorf("matches[0].Size = %d, want 8192 (trace.etl)", matches[0].Size)
	}
}

// MinFileSize floors --find results the same way it floors the top-files list:
// matches strictly smaller than the threshold are excluded, so --find surfaces
// only large hits (the "find large .dmp files" use case).
func TestScan_FindRespectsMinFileSize(t *testing.T) {
	root := scanTempDir(t)

	writeFile(t, filepath.Join(root, "A", "tiny.dmp"), make([]byte, 4096))
	writeFile(t, filepath.Join(root, "A", "big.dmp"), make([]byte, 64*1024))

	res := scanOrSkip(t, root, Options{
		ShowApparent: true,
		MinFileSize:  16 * 1024,
		Finds: []FindQuery{
			{Type: "ext", Value: ".dmp", Limit: 10},
		},
	})

	if len(res.FindResults) != 1 {
		t.Fatalf("FindResults len = %d, want 1", len(res.FindResults))
	}
	matches := res.FindResults[0].Matches
	if len(matches) != 1 {
		t.Fatalf("matches len = %d, want 1 (tiny.dmp is below --min); got %+v", len(matches), matches)
	}
	if matches[0].Size != 64*1024 {
		t.Errorf("matches[0].Size = %d, want %d (big.dmp)", matches[0].Size, 64*1024)
	}
}

func TestScan_FindByGlob(t *testing.T) {
	root := scanTempDir(t)

	writeFile(t, filepath.Join(root, "A", "report-2026.log"), make([]byte, 4096))
	writeFile(t, filepath.Join(root, "A", "report-old.log"), make([]byte, 2048))
	writeFile(t, filepath.Join(root, "A", "other.log"), make([]byte, 1024))

	res := scanOrSkip(t, root, Options{
		ShowApparent: true,
		Finds: []FindQuery{
			{Type: "glob", Value: "report-*.log", Limit: 10},
		},
	})

	if len(res.FindResults) != 1 {
		t.Fatalf("FindResults len = %d, want 1", len(res.FindResults))
	}
	if got := len(res.FindResults[0].Matches); got != 2 {
		t.Fatalf("matches len = %d, want 2; got %+v", got, res.FindResults[0].Matches)
	}
}

func TestScan_FindLimitCapsResults(t *testing.T) {
	root := scanTempDir(t)

	for i, sz := range []int{1024, 2048, 4096, 8192, 16384} {
		writeFile(t, filepath.Join(root, "A", "f"+string(rune('a'+i))+".dat"), make([]byte, sz))
	}

	res := scanOrSkip(t, root, Options{
		ShowApparent: true,
		Finds: []FindQuery{
			{Type: "ext", Value: ".dat", Limit: 3},
		},
	})

	if len(res.FindResults) != 1 {
		t.Fatalf("FindResults len = %d, want 1", len(res.FindResults))
	}
	matches := res.FindResults[0].Matches
	if len(matches) != 3 {
		t.Fatalf("matches len = %d, want 3 (per-query Limit)", len(matches))
	}
	// The 3 largest should win (16384, 8192, 4096).
	wantSizes := []int64{16384, 8192, 4096}
	for i, w := range wantSizes {
		if matches[i].Size != w {
			t.Errorf("matches[%d].Size = %d, want %d", i, matches[i].Size, w)
		}
	}
}

func TestValidateNTFSLayout(t *testing.T) {
	// modern-default NTFS layout: 4 KiB cluster, 1 KiB record, 512 sector.
	ok := ntfsVolumeData{
		TotalClusters:             100,
		BytesPerSector:            512,
		BytesPerCluster:           4096,
		BytesPerFileRecordSegment: 1024,
	}
	if err := validateNTFSLayout(&ok); err != nil {
		t.Errorf("ok layout rejected: %v", err)
	}

	// equal-size cluster and record (legacy small-volume layout) — still OK.
	okEq := ok
	okEq.BytesPerCluster = 1024
	if err := validateNTFSLayout(&okEq); err != nil {
		t.Errorf("cluster==record layout rejected: %v", err)
	}

	// cluster < record — the codex-flagged case.
	bad := ok
	bad.BytesPerCluster = 512
	if err := validateNTFSLayout(&bad); err == nil {
		t.Error("cluster < record was accepted; expected rejection")
	}

	// 4Kn drives report BytesPerSector=4096; the MSTP stride is still
	// 512 per the NTFS spec, so this configuration must NOT be rejected.
	ok4kn := ok
	ok4kn.BytesPerSector = 4096
	if err := validateNTFSLayout(&ok4kn); err != nil {
		t.Errorf("4Kn (BytesPerSector=4096) layout rejected: %v", err)
	}

	// record size not a multiple of 512.
	badRec := ok
	badRec.BytesPerFileRecordSegment = 1000
	if err := validateNTFSLayout(&badRec); err == nil {
		t.Error("non-512-multiple record size was accepted; expected rejection")
	}

	// zeroed fields that the validator must reject.
	for _, field := range []string{"cluster", "record"} {
		z := ok
		switch field {
		case "cluster":
			z.BytesPerCluster = 0
		case "record":
			z.BytesPerFileRecordSegment = 0
		}
		if err := validateNTFSLayout(&z); err == nil {
			t.Errorf("zero %s was accepted; expected rejection", field)
		}
	}
}

func TestScan_FindMultipleQueriesIndependentLimits(t *testing.T) {
	root := scanTempDir(t)

	// Two .dmp files (sizes 100, 200) and two .log files (sizes 50, 75).
	// If the matcher shared a single heap, the 100/200 dmps could evict
	// the smaller logs and leave the log query empty.
	writeFile(t, filepath.Join(root, "a.dmp"), make([]byte, 100))
	writeFile(t, filepath.Join(root, "b.dmp"), make([]byte, 200))
	writeFile(t, filepath.Join(root, "c.log"), make([]byte, 50))
	writeFile(t, filepath.Join(root, "d.log"), make([]byte, 75))

	res := scanOrSkip(t, root, Options{
		ShowApparent: true,
		Finds: []FindQuery{
			{Type: "ext", Value: ".dmp", Limit: 10, Label: "dumps"},
			{Type: "ext", Value: ".log", Limit: 10, Label: "logs"},
		},
	})

	if len(res.FindResults) != 2 {
		t.Fatalf("FindResults len = %d, want 2", len(res.FindResults))
	}
	dmps := res.FindResults[0]
	logs := res.FindResults[1]
	if dmps.Query.Label != "dumps" || logs.Query.Label != "logs" {
		t.Errorf("labels misordered: got %q, %q", dmps.Query.Label, logs.Query.Label)
	}
	if len(dmps.Matches) != 2 {
		t.Errorf("dumps matches = %d, want 2", len(dmps.Matches))
	}
	if len(logs.Matches) != 2 {
		t.Errorf("logs matches = %d, want 2 (must not be starved by larger dumps)", len(logs.Matches))
	}
}

func TestScan_FindLongExtensionNotTruncated(t *testing.T) {
	root := scanTempDir(t)

	// .crdownload (10 chars) and .application (11 chars) — both > 8 chars,
	// which would be silently truncated by the old 8-byte buffer.
	writeFile(t, filepath.Join(root, "partial.crdownload"), make([]byte, 4096))
	writeFile(t, filepath.Join(root, "installer.application"), make([]byte, 8192))
	writeFile(t, filepath.Join(root, "decoy.crdown"), make([]byte, 100))

	res := scanOrSkip(t, root, Options{
		ShowApparent: true,
		Finds: []FindQuery{
			{Type: "ext", Value: ".crdownload"},
			{Type: "ext", Value: ".application"},
		},
	})

	if len(res.FindResults) != 2 {
		t.Fatalf("FindResults len = %d, want 2", len(res.FindResults))
	}
	if got := len(res.FindResults[0].Matches); got != 1 {
		t.Errorf("crdownload matches = %d, want 1 (the decoy.crdown must NOT match)", got)
	}
	if got := len(res.FindResults[1].Matches); got != 1 {
		t.Errorf("application matches = %d, want 1", got)
	}
}

// TestIsLocalDrivePath pins the path-class gate that keeps a caller-supplied
// path away from CreateFile. It needs no elevation because it is a pure string
// check. The UNC cases are the security-critical ones: handing \\host\share to
// CreateFile makes the SMB client authenticate to a caller-named host.
func TestIsLocalDrivePath(t *testing.T) {
	reject := []string{
		`\\attacker\share\x`,   // UNC
		`\\ATTACKER\share`,     // UNC, no trailing path
		`\\?\C:\Windows`,       // DOS device path (local, still refused)
		`\\.\C:\Windows`,       // DOS device path
		`\\?\UNC\server\share`, // device UNC
		`\\.\UNC\server\share`, // device UNC
		`\\?\Volume{b75e2c83-0000-0000-0000-602f00000000}\x`, // volume GUID
		`\\.\Volume{b75e2c83-0000-0000-0000-602f00000000}\x`, // volume GUID
		`\\.\CON`, `\\.\NUL`, `\\.\LPT1`, // legacy DOS devices, post-Abs form
		`\Windows`,   // rooted, no drive
		`relative\x`, // relative
		`C:`,         // bare drive, no separator
		`C:x`,        // drive-relative, not fully qualified
		`1:\x`,       // not a letter
		`:\x`,        // no letter
		``,           // empty
		`\`, `\\`, `//`,
	}
	for _, p := range reject {
		if isLocalDrivePath(p) {
			t.Errorf("isLocalDrivePath(%q) = true, want false", p)
		}
	}

	accept := []string{
		`C:\`,
		`C:\Windows`,
		`C:\Windows\System32\drivers`,
		`C:/Windows`, // forward slash is a legal Windows separator
		`c:\windows`, // lowercase drive letter
		`Z:\x`,
	}
	for _, p := range accept {
		if !isLocalDrivePath(p) {
			t.Errorf("isLocalDrivePath(%q) = false, want true", p)
		}
	}
}

// TestNormalizeTargetRejectsUnsafePaths pins the whole target gate, not just the
// isLocalDrivePath half: a relative path is refused outright rather than resolved
// against the host process cwd, and the path classes that must never reach
// CreateFile stay refused. A forward-slash UNC and a bare legacy DOS device name
// are the interesting ones — both are non-obvious spellings of a rejected class.
func TestNormalizeTargetRejectsUnsafePaths(t *testing.T) {
	for _, p := range []string{
		`sub\dir`, `.`, `..`, ``, // relative: would otherwise consult the process cwd
		`C:x`,                        // drive-relative: the per-drive current directory
		`CON`, `NUL`, `LPT1`, `COM1`, // legacy DOS devices
		`//attacker/share/x`, `\\attacker\share\x`,
		`\\?\C:\Windows`,
		`\Windows`, // rooted, no drive
	} {
		s := scanState{res: &Result{}}
		if err := s.normalizeTarget(p); err == nil {
			t.Errorf("normalizeTarget(%q) = nil, want an error (normalized to %q)", p, s.abs)
		}
	}
}

// TestNormalizeTargetNormalizes pins what an accepted target is turned into:
// cleaned, drive upcased, trailing separator present.
func TestNormalizeTargetNormalizes(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`c:\windows`, `C:\windows\`},
		{`C:\Windows\`, `C:\Windows\`},
		{`C:\`, `C:\`},
		{`C:\Windows\..\Windows\System32`, `C:\Windows\System32\`},
		{`C:/Windows`, `C:\Windows\`},
	} {
		s := scanState{res: &Result{}}
		if err := s.normalizeTarget(tc.in); err != nil {
			t.Errorf("normalizeTarget(%q): %v", tc.in, err)
			continue
		}
		if s.abs != tc.want {
			t.Errorf("normalizeTarget(%q) = %q, want %q", tc.in, s.abs, tc.want)
		}
	}
}

// TestScan_ExcludeUNCRejected verifies a UNC --exclude fails the scan instead of
// reaching CreateFile. The host is unroutable (RFC 5737 TEST-NET-1) so that a
// regression which does reach the SMB client surfaces as a timeout rather than
// quietly succeeding.
func TestScan_ExcludeUNCRejected(t *testing.T) {
	root := scanTempDir(t)
	writeFile(t, filepath.Join(root, "a.bin"), make([]byte, 4096))

	requireAdmin(t)
	_, err := Scan(context.Background(), root, Options{
		TreeDepth: 1,
		Exclude:   []string{`\\192.0.2.1\share\x`},
	})
	if err == nil {
		t.Fatal("Scan accepted a UNC --exclude; want an error")
	}
	if isRawMFTUnsupported(err) {
		t.Skipf("raw MFT access not supported on this volume: %v", err)
	}
	if !strings.Contains(err.Error(), "local drive-letter") {
		t.Errorf("Scan error = %v, want it to name the local-drive-letter restriction", err)
	}
}

// TestScan_ExcludeRelativeRejected pins that a relative exclude is an error and
// not resolved against the host process cwd. It is rejected rather than skipped
// like a missing exclude: a relative path is a caller contract violation, and the
// subtree it names is unknowable here, so silently ignoring it would leave the
// caller believing something was excluded.
func TestScan_ExcludeRelativeRejected(t *testing.T) {
	root := scanTempDir(t)
	writeFile(t, filepath.Join(root, "a.bin"), make([]byte, 4096))

	requireAdmin(t)
	_, err := Scan(context.Background(), root, Options{
		TreeDepth: 1,
		Exclude:   []string{`sub\dir`},
	})
	if err == nil {
		t.Fatal("Scan accepted a relative --exclude; want an error")
	}
	if isRawMFTUnsupported(err) {
		t.Skipf("raw MFT access not supported on this volume: %v", err)
	}
	if !strings.Contains(err.Error(), "absolute path") {
		t.Errorf("Scan error = %v, want it to name the absolute-path requirement", err)
	}
}

// TestScan_ExcludeOnUnmountedDriveIsIgnored pins how an exclude naming a drive
// letter that holds no volume behaves: it is an unresolvable path, so it falls
// under the same rule as any other missing exclude and is skipped.
//
// There is deliberately no drive-LETTER comparison anymore. It is not a valid
// proxy for "same volume" once reparse points are involved — an exclude typed
// under a junction resolves onto the target's volume while carrying a different
// letter, and one typed with the resolved letter carries a letter the target's own
// path never had. The volume serial is the real test.
func TestScan_ExcludeOnUnmountedDriveIsIgnored(t *testing.T) {
	root := scanTempDir(t)
	if !isLocalDrivePath(root) {
		t.Skipf("temp dir %q is not a local drive path", root)
	}
	want := writeFile(t, filepath.Join(root, "a.bin"), make([]byte, 4096))

	// Pick a letter with no volume behind it, so the path cannot resolve at all.
	unmounted := byte(0)
	for c := byte('X'); c >= 'D'; c-- {
		if _, err := os.Stat(string(c) + `:\`); err != nil {
			unmounted = c
			break
		}
	}
	if unmounted == 0 {
		t.Skip("every drive letter D:-X: is mounted; cannot construct an unresolvable path")
	}

	res := scanOrSkip(t, root, Options{
		TreeDepth: 1,
		Exclude:   []string{string(unmounted) + `:\somewhere`},
	})
	if res.Subtree != want {
		t.Errorf("Subtree = %d, want %d (an unresolvable exclude must not change totals)",
			res.Subtree, want)
	}
}

// TestScan_ExcludeOtherVolumeRejected verifies that an existing exclude on a
// different local volume cannot be interpreted as an MFT record on the scanned
// volume. The disposable VHD is the scan volume; t.TempDir is the runner volume.
// An intermediate junction must be resolved before the same serial check.
func TestScan_ExcludeOtherVolumeRejected(t *testing.T) {
	root := scanTempDir(t)
	foreign := t.TempDir()
	foreignChild := filepath.Join(foreign, "child")
	if err := os.MkdirAll(foreignChild, 0o755); err != nil {
		t.Fatalf("mkdir foreign directory %q: %v", foreignChild, err)
	}

	_, targetSerial, _, err := resolvePathLocation(root)
	if err != nil {
		t.Fatalf("resolve VHD root %q: %v", root, err)
	}
	_, foreignSerial, _, err := resolvePathLocation(foreignChild)
	if err != nil {
		t.Fatalf("resolve foreign directory %q: %v", foreignChild, err)
	}
	if targetSerial == foreignSerial {
		t.Skipf("VHD root %q and foreign directory %q are on the same volume", root, foreign)
	}

	assertRejected := func(t *testing.T, exclude string) {
		t.Helper()
		_, err := Scan(context.Background(), root, Options{TreeDepth: 0, Exclude: []string{exclude}})
		if err == nil {
			t.Fatalf("Scan accepted cross-volume exclude %q", exclude)
		}
		if isRawMFTUnsupported(err) {
			t.Skipf("raw MFT access not supported on VHD: %v", err)
		}
		if !strings.Contains(err.Error(), "not on the scanned volume") {
			t.Fatalf("Scan(%q, exclude %q) error = %v, want volume-mismatch error", root, exclude, err)
		}
	}

	t.Run("direct", func(t *testing.T) {
		assertRejected(t, foreignChild)
	})

	t.Run("through intermediate junction", func(t *testing.T) {
		link := filepath.Join(root, "foreign")
		mkJunction(t, link, foreign)
		assertRejected(t, filepath.Join(link, "child"))
	})
}

// TestScan_ExcludeMissingPathIsIgnored pins the deliberate asymmetry: a
// nonexistent exclude must NOT fail the scan, because pre-emptively excluding a
// path that may or may not exist yet is a supported use. A UNC or wrong-drive
// exclude is an error; a merely absent one is skipped.
func TestScan_ExcludeMissingPathIsIgnored(t *testing.T) {
	root := scanTempDir(t)
	want := writeFile(t, filepath.Join(root, "a.bin"), make([]byte, 4096))

	res := scanOrSkip(t, root, Options{
		TreeDepth: 1,
		Exclude:   []string{filepath.Join(root, "does-not-exist", "nor-this")},
	})
	if res.Subtree != want {
		t.Errorf("Subtree = %d, want %d (a missing exclude must not change totals)", res.Subtree, want)
	}
}

func TestScan_ExcludeSubtree(t *testing.T) {
	root := scanTempDir(t)

	keep := writeFile(t, filepath.Join(root, "Keep", "x.bin"), make([]byte, 4096))
	dropDir := filepath.Join(root, "Drop")
	if err := os.MkdirAll(dropDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dropDir, "y.bin"), make([]byte, 8192))

	res := scanOrSkip(t, root, Options{TreeDepth: 1, Exclude: []string{dropDir}})

	if res.Subtree != keep {
		t.Errorf("Subtree = %d, want %d (Drop must be excluded)", res.Subtree, keep)
	}
	for _, c := range res.Tree.Children {
		if c.Name == "Drop" {
			t.Errorf("Tree.Children includes excluded dir %q with size %d", c.Name, c.Size)
		}
	}
}

// -------------------------------------------------------------------------
// Depth-N tree-mode tests
// -------------------------------------------------------------------------

func findTreeChild(t *testing.T, node *TreeNode, name string) *TreeNode {
	t.Helper()
	for _, c := range node.Children {
		if c.Name == name {
			return c
		}
	}
	names := make([]string, len(node.Children))
	for i, c := range node.Children {
		names[i] = c.Name
	}
	t.Fatalf("tree child %q not found under %q, have: %v", name, node.Name, names)
	return nil
}

// Depth 1 (the fast path) returns the root plus its immediate children, each
// carrying its whole-subtree total, and the root's Size equals Subtree.
func TestScan_TreeDepth1(t *testing.T) {
	root := scanTempDir(t)

	a1 := writeFile(t, filepath.Join(root, "A", "x.bin"), make([]byte, 4096))
	a2 := writeFile(t, filepath.Join(root, "A", "sub", "y.bin"), make([]byte, 8192))
	b1 := writeFile(t, filepath.Join(root, "B", "z.bin"), make([]byte, 4096))

	res := scanOrSkip(t, root, Options{TreeDepth: 1})
	if res.Tree == nil {
		t.Fatal("Tree is nil with TreeDepth=1")
	}
	if res.Tree.Depth != 0 {
		t.Errorf("root depth = %d, want 0", res.Tree.Depth)
	}
	treeA := findTreeChild(t, res.Tree, "A")
	if treeA.Size != a1+a2 {
		t.Errorf("Tree[A] size = %d, want %d (cumulative, incl. A/sub)", treeA.Size, a1+a2)
	}
	if treeA.Depth != 1 {
		t.Errorf("Tree[A] depth = %d, want 1", treeA.Depth)
	}
	if got := findTreeChild(t, res.Tree, "B").Size; got != b1 {
		t.Errorf("Tree[B] size = %d, want %d", got, b1)
	}
	if res.Tree.Size != res.Subtree {
		t.Errorf("root Size %d != Subtree %d", res.Tree.Size, res.Subtree)
	}
}

func TestScan_TreeDepth2Cumulative(t *testing.T) {
	root := scanTempDir(t)

	a := writeFile(t, filepath.Join(root, "A", "x.bin"), make([]byte, 4096))
	y := writeFile(t, filepath.Join(root, "A", "sub", "y.bin"), make([]byte, 8192))
	z := writeFile(t, filepath.Join(root, "A", "sub", "deeper", "z.bin"), make([]byte, 16384))

	res := scanOrSkip(t, root, Options{TreeDepth: 2})

	want := a + y + z
	treeA := findTreeChild(t, res.Tree, "A")
	if treeA.Size != want {
		t.Errorf("Tree[A] cumulative = %d, want %d", treeA.Size, want)
	}
	subNode := findTreeChild(t, treeA, "sub")
	if subNode.Depth != 2 {
		t.Errorf("Tree[A/sub] depth = %d, want 2", subNode.Depth)
	}
	if subNode.Size != y+z {
		t.Errorf("Tree[A/sub] cumulative = %d, want %d (y + z)", subNode.Size, y+z)
	}
	for _, c := range subNode.Children {
		if c.Name == "deeper" {
			t.Errorf("Tree[A/sub] has child %q at depth %d but TreeDepth=2", c.Name, c.Depth)
		}
	}
}

// Files directly under the target roll into the root total (there is no longer
// a separate "loose" bucket), and TreeMinSize filters displayed children
// without changing any total.
func TestScan_TreeRootTotalsIncludeDirectFiles(t *testing.T) {
	root := scanTempDir(t)

	direct1 := writeFile(t, filepath.Join(root, "loose1.bin"), make([]byte, 4096))
	direct2 := writeFile(t, filepath.Join(root, "loose2.bin"), make([]byte, 4096))
	bigChild := writeFile(t, filepath.Join(root, "Big", "x.bin"), make([]byte, 65536))
	smallChild := writeFile(t, filepath.Join(root, "Small", "y.bin"), make([]byte, 4096))

	wantSubtree := direct1 + direct2 + bigChild + smallChild

	r1 := scanOrSkip(t, root, Options{TreeDepth: 2})
	if r1.Subtree != wantSubtree {
		t.Errorf("TreeDepth=2 Subtree = %d, want %d", r1.Subtree, wantSubtree)
	}
	if r1.Tree.Size != wantSubtree {
		t.Errorf("root Size = %d, want %d (incl. files directly under target)", r1.Tree.Size, wantSubtree)
	}
	if r1.Tree.Files != 4 {
		t.Errorf("root Files = %d, want 4 (2 direct + 2 in children)", r1.Tree.Files)
	}

	r2 := scanOrSkip(t, root, Options{TreeDepth: 2, TreeMinSize: 32 * 1024})
	if r2.Subtree != wantSubtree {
		t.Errorf("TreeMinSize=32K Subtree = %d, want %d (must be unaffected by TreeMinSize)", r2.Subtree, wantSubtree)
	}
	for _, c := range r2.Tree.Children {
		if c.Name == "Small" {
			t.Errorf("Tree.Children includes Small but its size %d < TreeMinSize 32768", c.Size)
		}
	}
}

func TestScan_TreeMinSizeOnlyAffectsTree(t *testing.T) {
	root := scanTempDir(t)

	direct := writeFile(t, filepath.Join(root, "loose.bin"), make([]byte, 4096))
	big := writeFile(t, filepath.Join(root, "Big", "x.bin"), make([]byte, 65536))
	small := writeFile(t, filepath.Join(root, "Small", "y.bin"), make([]byte, 4096))

	rNoFilter := scanOrSkip(t, root, Options{TreeDepth: 1})
	rFiltered := scanOrSkip(t, root, Options{TreeDepth: 1, TreeMinSize: 32 * 1024})

	if rNoFilter.Subtree != rFiltered.Subtree {
		t.Errorf("Subtree mismatch: no-filter=%d, filtered=%d", rNoFilter.Subtree, rFiltered.Subtree)
	}
	if rNoFilter.Subtree != direct+big+small {
		t.Errorf("Subtree = %d, want %d", rNoFilter.Subtree, direct+big+small)
	}
	if len(rFiltered.Tree.Children) != 1 {
		names := make([]string, len(rFiltered.Tree.Children))
		for i, c := range rFiltered.Tree.Children {
			names[i] = c.Name
		}
		t.Errorf("filtered Tree.Children = %v, want only [Big]", names)
	}
}

func TestScan_TreeHardlinkAcrossChildren(t *testing.T) {
	root := scanTempDir(t)

	primary := filepath.Join(root, "A", "shared.bin")
	link := filepath.Join(root, "B", "shared.bin")

	sz := writeFile(t, primary, make([]byte, 4096))
	if err := os.MkdirAll(filepath.Join(root, "B"), 0o755); err != nil {
		t.Fatal(err)
	}
	createHardLink(t, link, primary)

	res := scanOrSkip(t, root, Options{TreeDepth: 1})

	if got := findTreeChild(t, res.Tree, "A").Size; got != sz {
		t.Errorf("child A size = %d, want %d", got, sz)
	}
	if got := findTreeChild(t, res.Tree, "B").Size; got != sz {
		t.Errorf("child B size = %d, want %d", got, sz)
	}
	if res.Subtree != sz {
		t.Errorf("Subtree = %d, want %d (dedup hard-linked file)", res.Subtree, sz)
	}
	if res.MultiParentFiles != 1 {
		t.Errorf("MultiParentFiles = %d, want 1", res.MultiParentFiles)
	}
}

// A file directly under the target that is also hard-linked into a child dir is
// deduped in the root total, attributed to the child, and counted as
// multi-parent (target + child are two distinct in-scope parents).
func TestScan_TreeDirectFileHardlinkedToChild(t *testing.T) {
	root := scanTempDir(t)

	primary := filepath.Join(root, "loose.bin")
	link := filepath.Join(root, "A", "shared.bin")

	sz := writeFile(t, primary, make([]byte, 4096))
	if err := os.MkdirAll(filepath.Join(root, "A"), 0o755); err != nil {
		t.Fatal(err)
	}
	createHardLink(t, link, primary)

	res := scanOrSkip(t, root, Options{TreeDepth: 1})

	if got := findTreeChild(t, res.Tree, "A").Size; got != sz {
		t.Errorf("child A = %d, want %d", got, sz)
	}
	if res.Subtree != sz {
		t.Errorf("Subtree = %d, want %d (dedup)", res.Subtree, sz)
	}
	if res.MultiParentFiles != 1 {
		t.Errorf("MultiParentFiles = %d, want 1", res.MultiParentFiles)
	}
}

func TestScan_TreeExcludedSubtree(t *testing.T) {
	root := scanTempDir(t)

	keep := writeFile(t, filepath.Join(root, "Keep", "x.bin"), make([]byte, 4096))
	dropDir := filepath.Join(root, "Drop")
	if err := os.MkdirAll(dropDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dropDir, "y.bin"), make([]byte, 8192))

	res := scanOrSkip(t, root, Options{TreeDepth: 1, Exclude: []string{dropDir}})

	if res.Subtree != keep {
		t.Errorf("Subtree = %d, want %d (Drop must be excluded)", res.Subtree, keep)
	}
	for _, c := range res.Tree.Children {
		if c.Name == "Drop" {
			t.Errorf("Tree.Children includes excluded dir %q", c.Name)
		}
	}
}

// TreeMinSize filters depth-1 children out of the tree at depth >= 2 as well.
func TestScan_TreeMinSizeFiltersChildrenAtDepth2(t *testing.T) {
	root := scanTempDir(t)

	writeFile(t, filepath.Join(root, "Big", "x.bin"), make([]byte, 65536))
	writeFile(t, filepath.Join(root, "Small", "y.bin"), make([]byte, 4096))

	res := scanOrSkip(t, root, Options{TreeDepth: 2, TreeMinSize: 32 * 1024})

	if findTreeChild(t, res.Tree, "Big").Size <= 0 {
		t.Errorf("child Big missing or zero size")
	}
	for _, c := range res.Tree.Children {
		if c.Name == "Small" {
			t.Errorf("Tree.Children contains Small (size %d) but TreeMinSize should filter it", c.Size)
		}
	}
}

// TestStreamPipelinedReportsReadErrors verifies that a raw-volume ReadFile
// failure is counted rather than silently swallowed: a dropped chunk would
// undercount every folder / top-file total, so the caller must be able to detect
// and report it. The pipeline recovers and continues instead of aborting. An
// invalid handle makes every chunk read fail deterministically, so this needs no
// elevation or real volume.
func TestStreamPipelinedReportsReadErrors(t *testing.T) {
	const recordSize = 1024
	// Two chunk-sized extents so we also confirm the second failed chunk is
	// counted after the first (recover-and-continue, not abort).
	const chunkBytes = 4096 * recordSize
	extents := []extent{{byteOffset: 0, byteLength: 2 * chunkBytes}}
	var called int
	parsed, _, readErrs, skipped := streamPipelined(
		context.Background(),
		windows.InvalidHandle,
		extents,
		recordSize,
		modeAll,
		func(idx uint64, e *mftEntry, baseRef uint64) { called++ },
	)
	if readErrs != 2 {
		t.Errorf("readErrs = %d, want 2 (both chunks unreadable)", readErrs)
	}
	if skipped != 2*4096 {
		t.Errorf("skipped = %d, want %d (all records in both chunks)", skipped, 2*4096)
	}
	if parsed != 0 {
		t.Errorf("parsed = %d, want 0 (nothing was readable)", parsed)
	}
	if called != 0 {
		t.Errorf("callback invoked %d times despite the chunk reads failing; expected 0", called)
	}
}

// TestStreamPipelinedUnmappedExtentsAreNotReadErrors verifies an unmapped range
// is skipped without being blamed on I/O: the loss is reported from the extent map
// instead (Result.UnmappedMFTRecords), so readErrs must stay clean. The record
// index must still advance across the gap, which is what keeps later records'
// indices correct.
//
// An invalid handle is safe here because an unmapped extent is never read.
func TestStreamPipelinedUnmappedExtentsAreNotReadErrors(t *testing.T) {
	const recordSize = 1024
	extents := []extent{{byteOffset: unmappedExtent, byteLength: 8 * recordSize}}
	var called int
	parsed, errs, readErrs, skipped := streamPipelined(
		context.Background(),
		windows.InvalidHandle,
		extents,
		recordSize,
		modeAll,
		func(idx uint64, e *mftEntry, baseRef uint64) { called++ },
	)
	if readErrs != 0 {
		t.Errorf("readErrs = %d, want 0 (an unmapped range is not an I/O failure)", readErrs)
	}
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0 (counted from the extent map, not here)", skipped)
	}
	if parsed != 0 || errs != 0 || called != 0 {
		t.Errorf("parsed=%d errs=%d called=%d, want all 0 (nothing to read)", parsed, errs, called)
	}
}

// TestOpenFileByIdVolumeHintAccessMask guards the access mask
// resolveCandidatePaths requests for its hVolumeHint handle. Microsoft documents no
// requirement for that handle and their own sample uses GENERIC_READ, so the only
// evidence that a narrower mask works is a live check.
//
// It matters because a failed root open makes every top-file path degrade to
// "?\<basename>", and asking for fewer rights makes that open more likely to
// succeed. The other masks are logged for diagnosis if this ever regresses.
func TestOpenFileByIdVolumeHintAccessMask(t *testing.T) {
	requireAdmin(t)

	dir := t.TempDir()
	if !isLocalDrivePath(dir) {
		t.Skipf("temp dir %q is not a local drive path", dir)
	}
	probe := filepath.Join(dir, "probe.bin")
	writeFile(t, probe, make([]byte, 64))

	pw, err := windows.UTF16PtrFromString(probe)
	if err != nil {
		t.Fatal(err)
	}
	hf, err := windows.CreateFile(pw, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatalf("open probe: %v", err)
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(hf, &info); err != nil {
		windows.CloseHandle(hf)
		t.Fatalf("GetFileInformationByHandle: %v", err)
	}
	windows.CloseHandle(hf)
	fileID := uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow)

	rootW, err := windows.UTF16PtrFromString(dir[:2] + `\`)
	if err != nil {
		t.Fatal(err)
	}

	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	openByID := kernel32.NewProc("OpenFileById")

	// tryHint opens the volume root with desiredAccess, then reports whether
	// OpenFileById resolves fileID through that hint handle.
	tryHint := func(desiredAccess uint32) (opened bool, callErr, hintErr error) {
		hRoot, err := windows.CreateFile(rootW, desiredAccess,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
			nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
		if err != nil {
			return false, nil, err
		}
		defer windows.CloseHandle(hRoot)

		fid := fileIDDescriptor{
			Size:   uint32(unsafe.Sizeof(fileIDDescriptor{})),
			Type:   0,
			FileID: fileID,
		}
		hr, _, e := openByID.Call(
			uintptr(hRoot),
			uintptr(unsafe.Pointer(&fid)),
			uintptr(windows.FILE_READ_ATTRIBUTES),
			uintptr(windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE),
			0,
			uintptr(windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT),
		)
		if h := windows.Handle(hr); hr == 0 || h == windows.InvalidHandle {
			return false, e, nil
		}
		windows.CloseHandle(windows.Handle(hr))
		return true, nil, nil
	}

	for _, tc := range []struct {
		name          string
		desiredAccess uint32
		shipped       bool
	}{
		{"access 0", 0, true},
		{"FILE_READ_ATTRIBUTES", windows.FILE_READ_ATTRIBUTES, false},
		{"GENERIC_READ", windows.GENERIC_READ, false},
	} {
		opened, callErr, hintErr := tryHint(tc.desiredAccess)
		switch {
		case hintErr != nil:
			t.Logf("%-22s hint open FAILED: %v", tc.name, hintErr)
		case opened:
			t.Logf("%-22s OpenFileById OK", tc.name)
		default:
			t.Logf("%-22s OpenFileById FAILED: %v", tc.name, callErr)
		}
		if tc.shipped && !opened {
			t.Errorf("resolveCandidatePaths opens the volume hint with desired access %#x, "+
				"but OpenFileById rejected it here (hintErr=%v callErr=%v); top-file paths "+
				"would degrade to \"?\\<basename>\"", tc.desiredAccess, hintErr, callErr)
		}
	}
}

// mkJunction creates a directory junction at link pointing at target, skipping the
// test if the OS refuses. mklink is used because creating a junction otherwise
// needs FSCTL_SET_REPARSE_POINT, which this package has no reason to wrap.
func mkJunction(t *testing.T, link, target string) {
	t.Helper()
	out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil {
		t.Skipf("cannot create junction %q -> %q: %v (%s)", link, target, err, out)
	}
}

// longPath expands any DOS 8.3 components in path. GetFinalPathNameByHandle,
// used by resolvePathLocation, returns this canonical spelling, while t.TempDir
// can use an 8.3 component on CI.
func longPath(t *testing.T, path string) string {
	t.Helper()
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatalf("UTF16PtrFromString(%q): %v", path, err)
	}
	buf := make([]uint16, 32768)
	n, err := windows.GetLongPathName(p, &buf[0], uint32(len(buf)))
	if err != nil {
		t.Fatalf("GetLongPathName(%q): %v", path, err)
	}
	if n >= uint32(len(buf)) {
		t.Fatalf("long path for %q exceeds %d chars", path, len(buf))
	}
	return windows.UTF16ToString(buf[:n])
}

// TestScan_TargetThroughSameVolumeJunction covers the path shape that motivated
// deriving the scanned volume from the RESOLVED target rather than the drive letter
// in the operand.
//
// A junction in an INTERMEDIATE component is always traversed — FILE_FLAG_OPEN_REPARSE_POINT
// suppresses that only for the final component — so `<root>\link\real` resolves to
// `<root>\real\real`. Here the junction stays on one volume, so the resolved drive
// matches and the scan must simply work; the value of the test is that it pins the
// resolve-then-open ordering, which is what stops a cross-volume junction from
// looking indices up in the wrong MFT.
func TestScan_TargetThroughSameVolumeJunction(t *testing.T) {
	root := scanTempDir(t)
	if !isLocalDrivePath(root) {
		t.Skipf("temp dir %q is not a local drive path", root)
	}
	// <root>\real\inner\a.bin, reached via <root>\link\inner
	inner := filepath.Join(root, "real", "inner")
	want := writeFile(t, filepath.Join(inner, "a.bin"), make([]byte, 8192))
	mkJunction(t, filepath.Join(root, "link"), filepath.Join(root, "real"))

	res := scanOrSkip(t, filepath.Join(root, "link", "inner"), Options{TreeDepth: 1})
	if res.Subtree != want {
		t.Errorf("Subtree = %d, want %d (scan through an intermediate junction)", res.Subtree, want)
	}
}

// TestScan_TargetIsJunctionReportsTheLinkItself pins that naming a reparse point
// directly does NOT dereference it, matching du's default for command-line
// operands. The junction has no children in this volume's MFT (parent references
// are physical), so its subtree is its own record allocation, not the destination's
// contents.
func TestScan_TargetIsJunctionReportsTheLinkItself(t *testing.T) {
	root := scanTempDir(t)
	if !isLocalDrivePath(root) {
		t.Skipf("temp dir %q is not a local drive path", root)
	}
	realDir := filepath.Join(root, "real")
	big := writeFile(t, filepath.Join(realDir, "big.bin"), make([]byte, 64*1024))
	link := filepath.Join(root, "link")
	mkJunction(t, link, realDir)

	res := scanOrSkip(t, link, Options{TreeDepth: 1})
	if res.Subtree >= big {
		t.Errorf("Subtree = %d, want well under %d: naming a junction must not pull in its destination",
			res.Subtree, big)
	}
}

// TestResolvePathLocationReparseSemantics pins the two properties the scanned
// volume is derived from. They are what make deriving it from the resolved path
// correct, and neither is stated outright in the Win32 docs.
//
//   - An INTERMEDIATE reparse point is traversed, so the resolved path (and hence
//     the volume) can differ from the operand's drive letter. This is the case that
//     made the scan read the wrong MFT.
//   - The FINAL component is NOT traversed, so naming a reparse point yields the
//     link itself — which is what keeps operand dereferencing a separate feature.
func TestResolvePathLocationReparseSemantics(t *testing.T) {
	root := longPath(t, t.TempDir())
	if !isLocalDrivePath(root) {
		t.Skipf("temp dir %q is not a local drive path", root)
	}
	realDir := filepath.Join(root, "real")
	inner := filepath.Join(realDir, "inner")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	mkJunction(t, link, realDir)

	t.Run("intermediate junction is traversed", func(t *testing.T) {
		viaLink, _, resolved, err := resolvePathLocation(filepath.Join(link, "inner"))
		if err != nil {
			t.Fatalf("resolve via link: %v", err)
		}
		direct, _, _, err := resolvePathLocation(inner)
		if err != nil {
			t.Fatalf("resolve direct: %v", err)
		}
		if viaLink != direct {
			t.Errorf("idx via link = %d, direct = %d; want equal (junction not traversed?)",
				viaLink, direct)
		}
		if clean := stripExtendedPathPrefix(resolved); !strings.EqualFold(clean, inner) {
			t.Errorf("resolved = %q, want %q — the resolved path is what the scanned volume is taken from",
				clean, inner)
		}
	})

	t.Run("final component is not traversed", func(t *testing.T) {
		linkIdx, _, resolved, err := resolvePathLocation(link)
		if err != nil {
			t.Fatalf("resolve link: %v", err)
		}
		realIdx, _, _, err := resolvePathLocation(realDir)
		if err != nil {
			t.Fatalf("resolve real: %v", err)
		}
		if linkIdx == realIdx {
			t.Errorf("idx of %q equals idx of its destination; the final component was dereferenced", link)
		}
		if clean := stripExtendedPathPrefix(resolved); !strings.EqualFold(clean, link) {
			t.Errorf("resolved = %q, want %q (the link itself)", clean, link)
		}
	})
}
