//go:build windows

package du

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// -------------------------------------------------------------------------
// Admin-rights gate
// -------------------------------------------------------------------------

// requireAdmin skips the test if the current process is not a member of the
// local Administrators group. Scan opens \\.\<drive>: which requires admin;
// without elevation the test cannot exercise the MFT walk. CI runs the agent
// container as ContainerAdministrator, so tests will execute there.
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

// scanOrSkip runs Scan(target). Requires admin (checked upfront).
func scanOrSkip(t *testing.T, target string, opts Options) *Result {
	t.Helper()
	requireAdmin(t)
	res, err := Scan(context.Background(), target, opts)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	return res
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

func findBucket(t *testing.T, res *Result, name string) Bucket {
	t.Helper()
	for _, b := range res.Buckets {
		if b.Name == name {
			return b
		}
	}
	names := make([]string, len(res.Buckets))
	for i, b := range res.Buckets {
		names[i] = b.Name
	}
	t.Fatalf("bucket %q not found, have: %v", name, names)
	return Bucket{}
}

// -------------------------------------------------------------------------
// Tests
// -------------------------------------------------------------------------

func TestScan_BasicDirectories(t *testing.T) {
	root := t.TempDir()

	a1 := writeFile(t, filepath.Join(root, "A", "file1.bin"), make([]byte, 4096))
	a2 := writeFile(t, filepath.Join(root, "A", "file2.bin"), make([]byte, 8192))
	b1 := writeFile(t, filepath.Join(root, "B", "file3.bin"), make([]byte, 4096))

	res := scanOrSkip(t, root, Options{})

	wantA := a1 + a2
	wantB := b1
	if got := findBucket(t, res, "A").Size; got != wantA {
		t.Errorf("bucket A = %d, want %d", got, wantA)
	}
	if got := findBucket(t, res, "B").Size; got != wantB {
		t.Errorf("bucket B = %d, want %d", got, wantB)
	}
	wantSubtree := wantA + wantB
	if res.Subtree != wantSubtree {
		t.Errorf("Subtree = %d, want %d", res.Subtree, wantSubtree)
	}
	if res.Loose != 0 {
		t.Errorf("Loose = %d, want 0", res.Loose)
	}
	if res.MultiBucketFiles != 0 {
		t.Errorf("MultiBucketFiles = %d, want 0", res.MultiBucketFiles)
	}
}

func TestScan_LooseFilesUnderTarget(t *testing.T) {
	root := t.TempDir()

	loose := writeFile(t, filepath.Join(root, "loose.bin"), make([]byte, 4096))
	a1 := writeFile(t, filepath.Join(root, "A", "x.bin"), make([]byte, 8192))

	res := scanOrSkip(t, root, Options{})

	if got := findBucket(t, res, "A").Size; got != a1 {
		t.Errorf("bucket A = %d, want %d", got, a1)
	}
	if res.Loose != loose {
		t.Errorf("Loose = %d, want %d", res.Loose, loose)
	}
	if res.Subtree != loose+a1 {
		t.Errorf("Subtree = %d, want %d", res.Subtree, loose+a1)
	}
}

func TestScan_NestedDirectories(t *testing.T) {
	root := t.TempDir()

	deep := writeFile(t, filepath.Join(root, "A", "sub1", "sub2", "deep.bin"), make([]byte, 4096))

	res := scanOrSkip(t, root, Options{})

	if got := findBucket(t, res, "A").Size; got != deep {
		t.Errorf("bucket A = %d, want %d (file in A/sub1/sub2/)", got, deep)
	}
	if res.Subtree != deep {
		t.Errorf("Subtree = %d, want %d", res.Subtree, deep)
	}
}

func TestScan_HardlinkSameBucket(t *testing.T) {
	root := t.TempDir()

	primary := filepath.Join(root, "A", "primary.bin")
	link := filepath.Join(root, "A", "secondary.bin")

	sz := writeFile(t, primary, make([]byte, 4096))
	createHardLink(t, link, primary)

	res := scanOrSkip(t, root, Options{})

	if got := findBucket(t, res, "A").Size; got != sz {
		t.Errorf("bucket A = %d, want %d (hard-linked file in same bucket)", got, sz)
	}
	if res.Subtree != sz {
		t.Errorf("Subtree = %d, want %d (dedup)", res.Subtree, sz)
	}
	if res.MultiBucketFiles != 0 {
		t.Errorf("MultiBucketFiles = %d, want 0 (single bucket)", res.MultiBucketFiles)
	}
}

func TestScan_HardlinkAcrossBuckets(t *testing.T) {
	root := t.TempDir()

	primary := filepath.Join(root, "A", "shared.bin")
	link := filepath.Join(root, "B", "shared.bin")

	sz := writeFile(t, primary, make([]byte, 4096))
	if err := os.MkdirAll(filepath.Join(root, "B"), 0o755); err != nil {
		t.Fatal(err)
	}
	createHardLink(t, link, primary)

	res := scanOrSkip(t, root, Options{})

	if got := findBucket(t, res, "A").Size; got != sz {
		t.Errorf("bucket A = %d, want %d (cross-bucket hardlink)", got, sz)
	}
	if got := findBucket(t, res, "B").Size; got != sz {
		t.Errorf("bucket B = %d, want %d (cross-bucket hardlink)", got, sz)
	}
	if res.Subtree != sz {
		t.Errorf("Subtree = %d, want %d (cross-bucket should dedup)", res.Subtree, sz)
	}
	if res.MultiBucketFiles != 1 {
		t.Errorf("MultiBucketFiles = %d, want 1", res.MultiBucketFiles)
	}
}

func TestScan_HardlinkTargetAndChild(t *testing.T) {
	root := t.TempDir()

	primary := filepath.Join(root, "loose.bin")
	link := filepath.Join(root, "A", "linked.bin")

	sz := writeFile(t, primary, make([]byte, 4096))
	createHardLink(t, link, primary)

	res := scanOrSkip(t, root, Options{})

	if got := findBucket(t, res, "A").Size; got != sz {
		t.Errorf("bucket A = %d, want %d", got, sz)
	}
	if res.Loose != sz {
		t.Errorf("Loose = %d, want %d", res.Loose, sz)
	}
	if res.Subtree != sz {
		t.Errorf("Subtree = %d, want %d", res.Subtree, sz)
	}
	if res.MultiBucketFiles != 1 {
		t.Errorf("MultiBucketFiles = %d, want 1 (target+child)", res.MultiBucketFiles)
	}
}

func TestScan_SparseFile_AllocatedNotApparent(t *testing.T) {
	root := t.TempDir()

	const virtual = 64 * 1024 * 1024
	allocated := createSparseFile(t, filepath.Join(root, "A", "sparse.bin"), virtual)

	res := scanOrSkip(t, root, Options{})

	if got := findBucket(t, res, "A").Size; got != allocated {
		t.Errorf("bucket A allocated = %d, want %d (sparse: actual on-disk)", got, allocated)
	}
	if allocated >= virtual/4 {
		t.Errorf("sparse file allocated %d is not significantly smaller than virtual %d — sparseness check failed",
			allocated, virtual)
	}
}

func TestScan_SparseFile_Apparent(t *testing.T) {
	root := t.TempDir()

	const virtual = 64 * 1024 * 1024
	createSparseFile(t, filepath.Join(root, "A", "sparse.bin"), virtual)

	res := scanOrSkip(t, root, Options{ShowApparent: true})

	if got := findBucket(t, res, "A").Size; got != virtual {
		t.Errorf("bucket A apparent = %d, want %d (apparent: virtual size)", got, virtual)
	}
}

func TestScan_CompressedFile(t *testing.T) {
	root := t.TempDir()

	const dataSize = 256 * 1024
	allocated := createCompressedFile(t, filepath.Join(root, "A", "compressed.bin"), dataSize)

	res := scanOrSkip(t, root, Options{})

	if got := findBucket(t, res, "A").Size; got != allocated {
		t.Errorf("bucket A allocated = %d, want %d (compressed)", got, allocated)
	}
	if allocated >= int64(dataSize) {
		t.Errorf("compressed file allocated %d is not smaller than data %d — compression didn't kick in",
			allocated, dataSize)
	}
}

func TestScan_ResidentSmallFile(t *testing.T) {
	root := t.TempDir()

	sz := writeFile(t, filepath.Join(root, "A", "tiny.bin"), make([]byte, 100))

	res := scanOrSkip(t, root, Options{ShowApparent: true})

	if got := findBucket(t, res, "A").Size; got != 100 {
		t.Errorf("bucket A apparent = %d, want 100 (resident $DATA)", got)
	}
	_ = sz
}

func TestScan_TargetWithNoChildDirs(t *testing.T) {
	root := t.TempDir()

	loose := writeFile(t, filepath.Join(root, "only.bin"), make([]byte, 4096))

	res := scanOrSkip(t, root, Options{})

	if len(res.Buckets) != 0 {
		t.Errorf("Buckets = %v, want empty (no child dirs)", res.Buckets)
	}
	if res.Loose != loose {
		t.Errorf("Loose = %d, want %d", res.Loose, loose)
	}
	if res.Subtree != loose {
		t.Errorf("Subtree = %d, want %d", res.Subtree, loose)
	}
}

func TestScan_EmptyTarget(t *testing.T) {
	root := t.TempDir()

	res := scanOrSkip(t, root, Options{})

	if len(res.Buckets) != 0 {
		t.Errorf("Buckets = %v, want empty", res.Buckets)
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
	root := t.TempDir()

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

	res := scanOrSkip(t, root, Options{})

	want := int64(mainBytes + adsBytes)
	if got := findBucket(t, res, "A").Size; got != want {
		t.Errorf("bucket A = %d, want %d (main+ADS sum)", got, want)
	}
}

func TestEnumerateImmediateChildren_FlagsReparsePoints(t *testing.T) {
	root := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root, "A"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "A"), filepath.Join(root, "B")); err != nil {
		t.Skipf("cannot create directory symlink: %v", err)
	}

	children, err := enumerateImmediateChildren(root + `\`)
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

func TestGetMFTIdxFromPath_DoesNotFollowReparsePoint(t *testing.T) {
	root := t.TempDir()

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
	root := t.TempDir()

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
	root := t.TempDir()

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

func TestScan_FindByExtension(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "A", "crash.dmp"), make([]byte, 4096))
	writeFile(t, filepath.Join(root, "A", "trace.etl"), make([]byte, 8192))
	writeFile(t, filepath.Join(root, "A", "ignore.txt"), make([]byte, 1024))
	writeFile(t, filepath.Join(root, "B", "other.dmp"), make([]byte, 2048))

	res := scanOrSkip(t, root, Options{
		ShowApparent: true,
		FindExt:      ".dmp,.etl",
		FindLimit:    10,
	})

	if len(res.Matched) != 3 {
		t.Fatalf("Matched len = %d, want 3; got %+v", len(res.Matched), res.Matched)
	}
	for i := 1; i < len(res.Matched); i++ {
		if res.Matched[i-1].Size < res.Matched[i].Size {
			t.Errorf("Matched not sorted descending: %+v", res.Matched)
			break
		}
	}
	if res.Matched[0].Size != 8192 {
		t.Errorf("Matched[0].Size = %d, want 8192 (trace.etl)", res.Matched[0].Size)
	}
}

func TestScan_FindByGlob(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "A", "report-2026.log"), make([]byte, 4096))
	writeFile(t, filepath.Join(root, "A", "report-old.log"), make([]byte, 2048))
	writeFile(t, filepath.Join(root, "A", "other.log"), make([]byte, 1024))

	res := scanOrSkip(t, root, Options{
		ShowApparent: true,
		FindGlobs:    []string{"report-*.log"},
		FindLimit:    10,
	})

	if len(res.Matched) != 2 {
		t.Fatalf("Matched len = %d, want 2; got %+v", len(res.Matched), res.Matched)
	}
}

func TestScan_FindLimitCapsResults(t *testing.T) {
	root := t.TempDir()

	for i, sz := range []int{1024, 2048, 4096, 8192, 16384} {
		writeFile(t, filepath.Join(root, "A", "f"+string(rune('a'+i))+".dat"), make([]byte, sz))
	}

	res := scanOrSkip(t, root, Options{
		ShowApparent: true,
		FindExt:      ".dat",
		FindLimit:    3,
	})

	if len(res.Matched) != 3 {
		t.Fatalf("Matched len = %d, want 3 (FindLimit)", len(res.Matched))
	}
	// The 3 largest should win (16384, 8192, 4096).
	wantSizes := []int64{16384, 8192, 4096}
	for i, w := range wantSizes {
		if res.Matched[i].Size != w {
			t.Errorf("Matched[%d].Size = %d, want %d", i, res.Matched[i].Size, w)
		}
	}
}

func TestScan_ExcludeSubtree(t *testing.T) {
	root := t.TempDir()

	keep := writeFile(t, filepath.Join(root, "Keep", "x.bin"), make([]byte, 4096))
	dropDir := filepath.Join(root, "Drop")
	if err := os.MkdirAll(dropDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dropDir, "y.bin"), make([]byte, 8192))

	res := scanOrSkip(t, root, Options{Exclude: []string{dropDir}})

	if res.Subtree != keep {
		t.Errorf("Subtree = %d, want %d (Drop must be excluded)", res.Subtree, keep)
	}
	for _, b := range res.Buckets {
		if b.Name == "Drop" {
			t.Errorf("Buckets includes excluded dir %q with size %d", b.Name, b.Size)
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

func TestScan_TreeDepth1MatchesBuckets(t *testing.T) {
	root := t.TempDir()

	a1 := writeFile(t, filepath.Join(root, "A", "x.bin"), make([]byte, 4096))
	a2 := writeFile(t, filepath.Join(root, "A", "sub", "y.bin"), make([]byte, 8192))
	b1 := writeFile(t, filepath.Join(root, "B", "z.bin"), make([]byte, 4096))

	res := scanOrSkip(t, root, Options{TreeDepth: 1})
	if res.Tree == nil {
		t.Fatal("Tree is nil with TreeDepth=1")
	}
	if got := findBucket(t, res, "A").Size; got != a1+a2 {
		t.Errorf("Buckets[A] = %d, want %d", got, a1+a2)
	}
	if got := findBucket(t, res, "B").Size; got != b1 {
		t.Errorf("Buckets[B] = %d, want %d", got, b1)
	}
	treeA := findTreeChild(t, res.Tree, "A")
	if treeA.Size != a1+a2 {
		t.Errorf("Tree[A] size = %d, want %d", treeA.Size, a1+a2)
	}
	if treeA.Depth != 1 {
		t.Errorf("Tree[A] depth = %d, want 1", treeA.Depth)
	}
	if res.Tree.Size != res.Subtree {
		t.Errorf("Tree.Root.Size %d != Subtree %d", res.Tree.Size, res.Subtree)
	}
}

func TestScan_TreeDepth2Cumulative(t *testing.T) {
	root := t.TempDir()

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

func TestScan_TreeLooseAccurate(t *testing.T) {
	root := t.TempDir()

	loose1 := writeFile(t, filepath.Join(root, "loose1.bin"), make([]byte, 4096))
	loose2 := writeFile(t, filepath.Join(root, "loose2.bin"), make([]byte, 4096))
	bigChild := writeFile(t, filepath.Join(root, "Big", "x.bin"), make([]byte, 65536))
	smallChild := writeFile(t, filepath.Join(root, "Small", "y.bin"), make([]byte, 4096))

	wantLoose := loose1 + loose2

	r1 := scanOrSkip(t, root, Options{TreeDepth: 2})
	if r1.Loose != wantLoose {
		t.Errorf("TreeDepth=2 Loose = %d, want %d", r1.Loose, wantLoose)
	}
	if r1.Subtree != wantLoose+bigChild+smallChild {
		t.Errorf("TreeDepth=2 Subtree = %d, want %d", r1.Subtree, wantLoose+bigChild+smallChild)
	}

	r2 := scanOrSkip(t, root, Options{TreeDepth: 2, TreeMinSize: 32 * 1024})
	if r2.Loose != wantLoose {
		t.Errorf("TreeDepth=2 TreeMinSize=32K Loose = %d, want %d (must be unaffected by TreeMinSize)", r2.Loose, wantLoose)
	}
	for _, c := range r2.Tree.Children {
		if c.Name == "Small" {
			t.Errorf("Tree.Children includes Small but its size %d < TreeMinSize 32768", c.Size)
		}
	}
}

func TestScan_TreeMinSizeOnlyAffectsTree(t *testing.T) {
	root := t.TempDir()

	loose := writeFile(t, filepath.Join(root, "loose.bin"), make([]byte, 4096))
	big := writeFile(t, filepath.Join(root, "Big", "x.bin"), make([]byte, 65536))
	small := writeFile(t, filepath.Join(root, "Small", "y.bin"), make([]byte, 4096))

	rNoFilter := scanOrSkip(t, root, Options{TreeDepth: 1})
	rFiltered := scanOrSkip(t, root, Options{TreeDepth: 1, TreeMinSize: 32 * 1024})

	if rNoFilter.Subtree != rFiltered.Subtree {
		t.Errorf("Subtree mismatch: no-filter=%d, filtered=%d", rNoFilter.Subtree, rFiltered.Subtree)
	}
	if rNoFilter.Loose != rFiltered.Loose {
		t.Errorf("Loose mismatch: no-filter=%d, filtered=%d", rNoFilter.Loose, rFiltered.Loose)
	}
	if rNoFilter.Loose != loose {
		t.Errorf("Loose = %d, want %d", rNoFilter.Loose, loose)
	}
	if rNoFilter.Subtree != loose+big+small {
		t.Errorf("Subtree = %d, want %d", rNoFilter.Subtree, loose+big+small)
	}
	if len(rFiltered.Tree.Children) != 1 {
		names := make([]string, len(rFiltered.Tree.Children))
		for i, c := range rFiltered.Tree.Children {
			names[i] = c.Name
		}
		t.Errorf("filtered Tree.Children = %v, want only [Big]", names)
	}
}

func TestScan_TreeHardlinkAcrossBuckets(t *testing.T) {
	root := t.TempDir()

	primary := filepath.Join(root, "A", "shared.bin")
	link := filepath.Join(root, "B", "shared.bin")

	sz := writeFile(t, primary, make([]byte, 4096))
	if err := os.MkdirAll(filepath.Join(root, "B"), 0o755); err != nil {
		t.Fatal(err)
	}
	createHardLink(t, link, primary)

	res := scanOrSkip(t, root, Options{TreeDepth: 1})

	if findBucket(t, res, "A").Size != sz {
		t.Errorf("Bucket A size = %d, want %d", findBucket(t, res, "A").Size, sz)
	}
	if findBucket(t, res, "B").Size != sz {
		t.Errorf("Bucket B size = %d, want %d", findBucket(t, res, "B").Size, sz)
	}
	if res.Subtree != sz {
		t.Errorf("Subtree = %d, want %d (dedup hard-linked file)", res.Subtree, sz)
	}
	if res.MultiBucketFiles != 1 {
		t.Errorf("MultiBucketFiles = %d, want 1", res.MultiBucketFiles)
	}
}

func TestScan_TreeLooseHardlinkedToBucket(t *testing.T) {
	root := t.TempDir()

	primary := filepath.Join(root, "loose.bin")
	link := filepath.Join(root, "A", "shared.bin")

	sz := writeFile(t, primary, make([]byte, 4096))
	if err := os.MkdirAll(filepath.Join(root, "A"), 0o755); err != nil {
		t.Fatal(err)
	}
	createHardLink(t, link, primary)

	res := scanOrSkip(t, root, Options{TreeDepth: 1})

	if res.Loose != sz {
		t.Errorf("Loose = %d, want %d", res.Loose, sz)
	}
	if findBucket(t, res, "A").Size != sz {
		t.Errorf("Bucket A = %d, want %d", findBucket(t, res, "A").Size, sz)
	}
	if res.Subtree != sz {
		t.Errorf("Subtree = %d, want %d (dedup)", res.Subtree, sz)
	}
	if res.MultiBucketFiles != 1 {
		t.Errorf("MultiBucketFiles = %d, want 1", res.MultiBucketFiles)
	}
}

func TestScan_TreeExcludedSubtree(t *testing.T) {
	root := t.TempDir()

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
	for _, b := range res.Buckets {
		if b.Name == "Drop" {
			t.Errorf("Buckets includes excluded dir %q", b.Name)
		}
	}
}

func TestScan_TreeBucketsDerivedFromTree(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "Big", "x.bin"), make([]byte, 65536))
	writeFile(t, filepath.Join(root, "Small", "y.bin"), make([]byte, 4096))

	res := scanOrSkip(t, root, Options{TreeDepth: 2, TreeMinSize: 32 * 1024})

	if findBucket(t, res, "Big").Size <= 0 {
		t.Errorf("Bucket Big missing or zero size")
	}
	for _, b := range res.Buckets {
		if b.Name == "Small" {
			t.Errorf("Buckets contains Small (size %d) but TreeMinSize filtered it from Tree", b.Size)
		}
	}
	for _, c := range res.Tree.Children {
		if c.Name == "Small" {
			t.Errorf("Tree.Children contains Small")
		}
	}
}
