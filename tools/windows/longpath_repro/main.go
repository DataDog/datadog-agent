// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build windows

// longpath_repro reproduces a failure in exec.Command when the calling
// process's working directory exceeds MAX_PATH (260 chars), even on systems
// where LongPathsEnabled=1 is set.
//
// # Root cause
//
// When a Go process calls exec.Command without setting cmd.Dir, the child
// process inherits the parent's current working directory.  Windows validates
// this during CreateProcess: if the CWD exceeds MAX_PATH, CreateProcess returns
// ERROR_INVALID_PARAMETER (87) under some condition that is still under
// investigation.
//
// What we know so far:
//   - Tests 3–7 (FAIL): no combination of manifest presence in caller or child
//     fixes the error.  cmd.exe has a genuine longPathAware manifest and still
//     fails as the child (Test 7), ruling out the child's manifest entirely.
//     A manifest injected via UpdateResourceW in the caller (Tests 5/6) also
//     fails, but it is unclear whether our injection is broken or whether the
//     caller's manifest does not control this check.
//   - Test 8 (FAIL, "directory name is invalid"): our manifest-less process
//     called CreateProcess for cmd.exe with explicit lpCurrentDirectory set to
//     the long path.  CreateProcess rejected it before cmd.exe started.
//     Different error code from Tests 3–7 (NULL lpCurrentDirectory).
//   - Test 9 (PASS): explicit short cmd.Dir overrides the inherited long CWD.
//     Confirmed fix.  Question: which short path to use in bzltestutil?
//
// Open question for bzltestutil fix: does \\?\ prefix bypass the MAX_PATH check?
//   - Test 10: exec.Command with cmd.Dir = \\?\<long path>.
//   - Test 11: CreateProcessW called directly (via syscall) with \\?\<long path>
//     as lpCurrentDirectory, bypassing any path normalisation exec.Command may apply.
//     PASS on either → bzltestutil can use \\?\+wd to preserve test CWD semantics.
//     FAIL on both  → bzltestutil must use a genuinely short path (e.g., os.TempDir()).
//
// This was observed in practice with bzltestutil (rules_go): it re-execs the
// test binary from within the runfiles tree, whose path exceeded 260 chars.
// The test binary lacked a manifest (pure Go, internal linker) or had a MinGW
// manifest without longPathAware (PIE/CGo builds via external linker), so
// CreateProcess failed.
//
// # Prerequisites
//
//   - LongPathsEnabled=1 in HKLM\SYSTEM\CurrentControlSet\Control\FileSystem
//     Confirm: fsutil behavior query LongPaths
//   - NTFS volume (Test 2 additionally requires 8.3 name generation; disabled
//     in Docker containers -- skip if GetShortPathNameW returns unchanged path)
//
// # Expected results (LongPathsEnabled=1)
//
//   - Test 1 (exec with long binary path, short CWD):              PASS
//   - Test 2 (exec with 8.3 short path, short CWD):               SKIP or FAIL
//   - Test 3 (parent no manifest, child no manifest, long CWD):   FAIL
//   - Test 4 (parent no manifest, CHILD has manifest, long CWD):  FAIL
//   - Test 5 (PARENT has manifest, child no manifest, long CWD):  FAIL
//   - Test 6 (parent has manifest, child has manifest, long CWD): FAIL
//   - Test 7 (cmd.exe as child, long CWD):                        FAIL
//   - Test 8 (explicit long cmd.Dir, no manifest):                FAIL ("directory name is invalid")
//   - Test 9 (explicit short cmd.Dir, long CWD):                  PASS
//   - Test 10 (cmd.Dir = \\?\<long path>, long CWD):             TBD
//   - Test 11 (CreateProcessW directly, \\?\<long path> as cwd): TBD
//
// # Build and run
//
//	go build -o longpath_repro.exe .
//	.\longpath_repro.exe
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	advapi32     = syscall.NewLazyDLL("advapi32.dll")
	regOpenKeyEx = advapi32.NewProc("RegOpenKeyExW")
	regQueryVal  = advapi32.NewProc("RegQueryValueExW")
	regCloseKey  = advapi32.NewProc("RegCloseKey")
)

const (
	hklmHandle    = uintptr(0x80000002) // HKEY_LOCAL_MACHINE
	keyQueryValue = 0x0001
	regDword      = 4
)

// longPathsEnabled reads HKLM\SYSTEM\CurrentControlSet\Control\FileSystem\LongPathsEnabled.
// Returns (value, ok): ok=false means the key/value could not be read.
func longPathsEnabled() (uint32, bool) {
	subkey, _ := syscall.UTF16PtrFromString(`SYSTEM\CurrentControlSet\Control\FileSystem`)
	var hkey uintptr
	r, _, _ := regOpenKeyEx.Call(hklmHandle, uintptr(unsafe.Pointer(subkey)), 0, keyQueryValue, uintptr(unsafe.Pointer(&hkey)))
	if r != 0 {
		return 0, false
	}
	defer regCloseKey.Call(hkey)

	valname, _ := syscall.UTF16PtrFromString("LongPathsEnabled")
	var valType, val, size uint32
	size = 4
	r, _, _ = regQueryVal.Call(hkey, uintptr(unsafe.Pointer(valname)), 0, uintptr(unsafe.Pointer(&valType)), uintptr(unsafe.Pointer(&val)), uintptr(unsafe.Pointer(&size)))
	if r != 0 || valType != regDword {
		return 0, false
	}
	return val, true
}

func init() {
	// When re-executed as the child process, print a marker and exit.
	if len(os.Args) > 1 && os.Args[1] == "-child" {
		fmt.Println("child ok")
		os.Exit(0)
	}
	// -from-long-cwd <cwd> <child>: used by Tests 5 and 6.  THIS binary acts
	// as the parent: changes CWD to <cwd> (a long path), then execs <child> -child.
	if len(os.Args) == 4 && os.Args[1] == "-from-long-cwd" {
		if err := os.Chdir(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "Chdir: %v\n", err)
			os.Exit(1)
		}
		wd, _ := os.Getwd()
		fmt.Fprintf(os.Stderr, "parent cwd=%q (len=%d, manifest=%v)\n", wd, len(wd), peHasManifest(os.Args[0]))
		fmt.Fprintf(os.Stderr, "spawning child %q (manifest=%v)\n", os.Args[3], peHasManifest(os.Args[3]))
		out, err := exec.Command(os.Args[3], "-child").CombinedOutput()
		fmt.Printf("%s", out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
}

var (
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	findResourceW       = kernel32.NewProc("FindResourceW")
	loadLibraryExW      = kernel32.NewProc("LoadLibraryExW")
	freeLibrary         = kernel32.NewProc("FreeLibrary")
	loadResource        = kernel32.NewProc("LoadResource")
	lockResource        = kernel32.NewProc("LockResource")
	sizeofResource      = kernel32.NewProc("SizeofResource")
	beginUpdateResource = kernel32.NewProc("BeginUpdateResourceW")
	updateResource      = kernel32.NewProc("UpdateResourceW")
	endUpdateResource   = kernel32.NewProc("EndUpdateResourceW")
	createProcessW_     = kernel32.NewProc("CreateProcessW")
	waitForSingleObject = kernel32.NewProc("WaitForSingleObject")
	getExitCodeProcess  = kernel32.NewProc("GetExitCodeProcess")
	closeHandle_        = kernel32.NewProc("CloseHandle")
)

const loadLibraryAsDatafile = 0x00000002

// peHasManifest reports whether the PE binary at path has an embedded
// RT_MANIFEST resource (type 24, ID 1).
func peHasManifest(path string) bool {
	pathW, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	mod, _, _ := loadLibraryExW.Call(
		uintptr(unsafe.Pointer(pathW)), 0, loadLibraryAsDatafile)
	if mod == 0 {
		return false
	}
	defer freeLibrary.Call(mod)
	// RT_MANIFEST = 24, CREATEPROCESS_MANIFEST_RESOURCE_ID = 1
	res, _, _ := findResourceW.Call(mod, 1, 24)
	return res != 0
}

// hasManifest reports whether the running executable has an embedded RT_MANIFEST
// resource (type 24, ID 1).
//
// Go binaries built with the internal linker currently have no manifest.
// Binaries linked via MinGW (PIE / CGo builds) carry MinGW's default
// trustInfo+supportedOS manifest, which lacks longPathAware.
func hasManifest() bool {
	self, err := os.Executable()
	if err != nil {
		return false
	}
	return peHasManifest(self)
}

// readManifest reads the RT_MANIFEST resource (type 24, ID 1) from the PE
// binary at path and returns its raw XML content.
func readManifest(path string) (string, error) {
	pathW, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	mod, _, _ := loadLibraryExW.Call(uintptr(unsafe.Pointer(pathW)), 0, loadLibraryAsDatafile)
	if mod == 0 {
		return "", fmt.Errorf("LoadLibraryExW failed")
	}
	defer freeLibrary.Call(mod)
	hres, _, _ := findResourceW.Call(mod, 1, 24)
	if hres == 0 {
		return "", fmt.Errorf("manifest not found")
	}
	size, _, _ := sizeofResource.Call(mod, hres)
	if size == 0 {
		return "", fmt.Errorf("SizeofResource returned 0")
	}
	hdata, _, _ := loadResource.Call(mod, hres)
	if hdata == 0 {
		return "", fmt.Errorf("LoadResource failed")
	}
	ptr, _, _ := lockResource.Call(hdata)
	if ptr == 0 {
		return "", fmt.Errorf("LockResource failed")
	}
	return string(unsafe.Slice((*byte)(unsafe.Pointer(ptr)), size)), nil
}

// getShortPath calls GetShortPathNameW and returns the 8.3 form of path.
// If the volume does not generate 8.3 names the original path is returned
// unchanged and identical == true.
func getShortPath(longPath string) (short string, identical bool, err error) {
	lp, err := syscall.UTF16PtrFromString(longPath)
	if err != nil {
		return "", false, err
	}
	n, _ := syscall.GetShortPathName(lp, nil, 0)
	if n == 0 {
		return longPath, true, nil
	}
	buf := make([]uint16, n)
	n, err = syscall.GetShortPathName(lp, &buf[0], n)
	if err != nil {
		return "", false, err
	}
	short = syscall.UTF16ToString(buf[:n])
	return short, short == longPath, nil
}

func run(path string) bool {
	out, err := exec.Command(path, "-child").CombinedOutput()
	fmt.Printf("  output: %q\n", string(out))
	if err != nil {
		fmt.Printf("  FAIL: %v\n", err)
		return false
	}
	fmt.Println("  PASS")
	return true
}

// runFromCWD changes the process working directory to cwd, runs path with
// -child, then restores the original working directory.  cmd.Dir is
// intentionally left unset so the child inherits the parent CWD, matching
// how bzltestutil re-execs test binaries.
func runFromCWD(path, cwd string) bool {
	orig, err := os.Getwd()
	if err != nil {
		fmt.Printf("  FAIL: Getwd: %v\n", err)
		return false
	}
	if err := os.Chdir(cwd); err != nil {
		fmt.Printf("  FAIL: Chdir: %v\n", err)
		return false
	}
	defer os.Chdir(orig)
	return run(path)
}

// longPathManifest is a minimal PE manifest that sets longPathAware=true.
const longPathManifest = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <application>
    <windowsSettings>
      <longPathAware xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">true</longPathAware>
    </windowsSettings>
  </application>
</assembly>`

// withManifest copies src to a new temp file, injects a longPathAware=true
// RT_MANIFEST resource using the Win32 UpdateResource API, and returns the
// path to the patched binary.  The caller is responsible for removing it.
func withManifest(src string) (string, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	dst, err := os.CreateTemp("", "longpath_fixed_*.exe")
	if err != nil {
		return "", err
	}
	if _, err := dst.Write(data); err != nil {
		dst.Close()
		os.Remove(dst.Name())
		return "", fmt.Errorf("write: %w", err)
	}
	dst.Close()

	dstW, err := syscall.UTF16PtrFromString(dst.Name())
	if err != nil {
		os.Remove(dst.Name())
		return "", err
	}
	manifest := []byte(longPathManifest)

	// BeginUpdateResourceW: open for resource update, preserve existing resources.
	hUpdate, _, e := beginUpdateResource.Call(uintptr(unsafe.Pointer(dstW)), 0)
	if hUpdate == 0 {
		os.Remove(dst.Name())
		return "", fmt.Errorf("BeginUpdateResource: %w", e)
	}
	// UpdateResourceW: RT_MANIFEST=24, CREATEPROCESS_MANIFEST_RESOURCE_ID=1, LANG_NEUTRAL=0.
	ret, _, e := updateResource.Call(
		hUpdate, 24, 1, 0,
		uintptr(unsafe.Pointer(&manifest[0])),
		uintptr(len(manifest)))
	if ret == 0 {
		endUpdateResource.Call(hUpdate, 1) // discard
		os.Remove(dst.Name())
		return "", fmt.Errorf("UpdateResource: %w", e)
	}
	ret, _, e = endUpdateResource.Call(hUpdate, 0) // commit
	if ret == 0 {
		os.Remove(dst.Name())
		return "", fmt.Errorf("EndUpdateResource: %w", e)
	}
	if !peHasManifest(dst.Name()) {
		os.Remove(dst.Name())
		return "", fmt.Errorf("manifest injection succeeded but verification failed")
	}
	return dst.Name(), nil
}

func main() {
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "os.Executable: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("=== longpath_repro ===")
	fmt.Println()
	fmt.Println("Prerequisite: LongPathsEnabled=1")
	fmt.Println("  Verify with: fsutil behavior query LongPaths")
	fmt.Println()

	if hasManifest() {
		fmt.Println("Manifest: present (longPathAware absent unless fix applied)")
	} else {
		fmt.Println("Manifest: none (Go internal linker — PEB trick only)")
	}

	if v, ok := longPathsEnabled(); ok {
		fmt.Printf("LongPathsEnabled (registry): %d\n", v)
	} else {
		fmt.Println("LongPathsEnabled (registry): unreadable")
	}
	fmt.Println()

	// Build a path well beyond MAX_PATH by nesting short-named directories.
	dir := os.TempDir()
	for range 11 {
		dir = filepath.Join(dir, "a_very_long_dir_name")
	}
	dir = filepath.Join(dir, "subdir")
	dst := filepath.Join(dir, "child.exe")
	fmt.Printf("Long path (len=%d):\n  %s\n\n", len(dst), dst)

	defer os.RemoveAll(filepath.Join(os.TempDir(), "a_very_long_dir_name"))

	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "MkdirAll: %v\n", err)
		os.Exit(1)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ReadFile: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(dst, data, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "WriteFile: %v\n", err)
		os.Exit(1)
	}

	ok := true

	fmt.Printf("--- Test 1: direct long path (len=%d) ---\n", len(dst))
	fmt.Println("  PEB bit covers this case; expected PASS on all Windows versions.")
	if !run(dst) {
		ok = false
	}
	fmt.Println()

	short, identical, err := getShortPath(dst)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetShortPathName: %v\n", err)
		os.Exit(1)
	}

	if identical {
		fmt.Println("--- Test 2: SKIP ---")
		fmt.Println("  GetShortPathNameW returned the original path unchanged.")
		fmt.Println("  8.3 name generation is disabled on this volume.")
		fmt.Println("  Run on bare metal or a VM (not a Docker container) to reproduce.")
	} else {
		fmt.Printf("--- Test 2: 8.3 short path (len=%d, expands to %d) ---\n",
			len(short), len(dst))
		fmt.Printf("  short: %s\n", short)
		fmt.Println("  CreateProcess expands the 8.3 alias internally.")
		if !run(short) {
			ok = false
		}
	}
	fmt.Println()

	fmt.Printf("--- Test 3: short binary path, long CWD (len=%d) ---\n", len(dir))
	fmt.Println("  Matches the bzltestutil re-exec scenario: exec.Command without")
	fmt.Println("  cmd.Dir set; child inherits a >MAX_PATH working directory.")
	fmt.Println("  Without longPathAware in the binary manifest: expected FAIL.")
	fmt.Printf("  binary: %s (len=%d, manifest=%v)\n", self, len(self), peHasManifest(self))
	if !runFromCWD(self, dir) {
		ok = false
	}
	fmt.Println()

	fmt.Printf("--- Test 4: parent no manifest, CHILD has manifest, long CWD (len=%d) ---\n", len(dir))
	fixed, merr := withManifest(self)
	if merr != nil {
		fmt.Printf("  SKIP: %v\n", merr)
	} else {
		defer os.Remove(fixed)
		fmt.Printf("  fixed binary: %s (manifest=%v)\n", fixed, peHasManifest(fixed))
		if !runFromCWD(fixed, dir) {
			ok = false
		}
	}
	fmt.Println()

	fmt.Printf("--- Test 5: PARENT has manifest, child no manifest, long CWD (len=%d) ---\n", len(dir))
	fmt.Println("  Parent (fixed) chdirs to long CWD then spawns self (no manifest).")
	if merr != nil {
		fmt.Printf("  SKIP: %v\n", merr)
	} else {
		out, err5 := exec.Command(fixed, "-from-long-cwd", dir, self).CombinedOutput()
		fmt.Printf("  output: %q\n", string(out))
		if err5 != nil {
			fmt.Printf("  FAIL: %v\n", err5)
			ok = false
		} else {
			fmt.Println("  PASS")
		}
	}
	fmt.Println()

	fmt.Printf("--- Test 6: PARENT has manifest, CHILD has manifest, long CWD (len=%d) ---\n", len(dir))
	fmt.Println("  Parent (fixed) chdirs to long CWD then spawns fixed -child.")
	if merr != nil {
		fmt.Printf("  SKIP: %v\n", merr)
	} else {
		out, err6 := exec.Command(fixed, "-from-long-cwd", dir, fixed).CombinedOutput()
		fmt.Printf("  output: %q\n", string(out))
		if err6 != nil {
			fmt.Printf("  FAIL: %v\n", err6)
			ok = false
		} else {
			fmt.Println("  PASS")
		}
	}
	fmt.Println()

	fmt.Printf("--- Test 7: cmd.exe (OS binary, built-in longPathAware), long CWD (len=%d) ---\n", len(dir))
	fmt.Println("  Spawn cmd.exe /c exit 0 from the long CWD without setting cmd.Dir.")
	fmt.Println("  cmd.exe is a system binary with longPathAware=true in its PE manifest.")
	cmdExe := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	fmt.Printf("  cmd.exe manifest present: %v\n", peHasManifest(cmdExe))
	if m, err := readManifest(cmdExe); err != nil {
		fmt.Printf("  cmd.exe readManifest error: %v\n", err)
	} else {
		fmt.Printf("  cmd.exe manifest: %s\n", m)
	}
	{
		orig, _ := os.Getwd()
		if err := os.Chdir(dir); err != nil {
			fmt.Printf("  FAIL: Chdir: %v\n", err)
			ok = false
		} else {
			out, err7 := exec.Command(cmdExe, "/c", "exit 0").CombinedOutput()
			os.Chdir(orig)
			fmt.Printf("  output: %q\n", string(out))
			if err7 != nil {
				fmt.Printf("  FAIL: %v\n", err7)
				ok = false
			} else {
				fmt.Println("  PASS")
			}
		}
	}
	fmt.Println()

	fmt.Printf("--- Test 8: explicit long cmd.Dir (no manifest), long CWD (len=%d) ---\n", len(dir))
	fmt.Println("  Our process (no manifest) spawns cmd.exe with cmd.Dir set to the long path.")
	fmt.Println("  Tests whether passing an explicit long lpCurrentDirectory is also rejected.")
	fmt.Println("  Expected FAIL: caller has no manifest, path > MAX_PATH, different error code.")
	{
		bat, batErr := os.CreateTemp("", "longpath_test8_*.bat")
		if batErr != nil {
			fmt.Printf("  SKIP: CreateTemp: %v\n", batErr)
		} else {
			// Write the batch file content directly -- no Go arg quoting involved.
			fmt.Fprintf(bat, "@echo off\r\necho cwd=%%cd%%\r\n\"%s\" -child\r\n", self)
			bat.Close()
			defer os.Remove(bat.Name())
			cmd8 := exec.Command(cmdExe, "/c", bat.Name())
			cmd8.Dir = dir // cmd.exe starts with a long CWD
			out, err8 := cmd8.CombinedOutput()
			fmt.Printf("  output: %q\n", string(out))
			if err8 != nil {
				fmt.Printf("  FAIL: %v\n", err8)
				ok = false
			} else {
				fmt.Println("  PASS")
			}
		}
	}
	fmt.Println()

	fmt.Printf("--- Test 9: cmd.Dir workaround, parent has long CWD (len=%d) ---\n", len(dir))
	fmt.Println("  Process chdirs to the long CWD, then spawns self with cmd.Dir set to a short path.")
	fmt.Println("  Child gets a short CWD, bypassing the CreateProcess long-CWD check.")
	fmt.Println("  This is the proposed fix for bzltestutil: always set cmd.Dir explicitly.")
	fmt.Println("  Expected PASS.")
	{
		orig, _ := os.Getwd()
		if err := os.Chdir(dir); err != nil {
			fmt.Printf("  FAIL: Chdir: %v\n", err)
			ok = false
		} else {
			cmd9 := exec.Command(self, "-child")
			cmd9.Dir = os.TempDir() // explicit short CWD overrides inherited long CWD
			out, err9 := cmd9.CombinedOutput()
			os.Chdir(orig)
			fmt.Printf("  output: %q\n", string(out))
			if err9 != nil {
				fmt.Printf("  FAIL: %v\n", err9)
				ok = false
			} else {
				fmt.Println("  PASS")
			}
		}
	}
	fmt.Println()

	fmt.Printf("--- Test 10: cmd.Dir = \\\\?\\\\<long path>, parent has long CWD (len=%d) ---\n", len(dir))
	fmt.Println("  Same as Test 9 but cmd.Dir uses the \\\\?\\\\ extended-length path prefix.")
	fmt.Println("  If PASS: bzltestutil can set cmd.Dir = \\\\?\\\\+wd, preserving test CWD semantics.")
	fmt.Println("  If FAIL: must use a genuinely short path; test CWD semantics will change.")
	{
		orig, _ := os.Getwd()
		if err := os.Chdir(dir); err != nil {
			fmt.Printf("  FAIL: Chdir: %v\n", err)
			ok = false
		} else {
			cmd10 := exec.Command(self, "-child")
			cmd10.Dir = `\\?\` + dir // extended-length path prefix bypasses MAX_PATH for most Win32 APIs
			fmt.Printf("  cmd10.Dir (len=%d): %s\n", len(cmd10.Dir), cmd10.Dir)
			out, err10 := cmd10.CombinedOutput()
			os.Chdir(orig)
			fmt.Printf("  output: %q\n", string(out))
			if err10 != nil {
				fmt.Printf("  FAIL: %v\n", err10)
				ok = false
			} else {
				fmt.Println("  PASS")
			}
		}
	}

	fmt.Println()

	fmt.Printf("--- Test 11: CreateProcessW directly, \\\\?\\\\ prefix on cwd (len=%d) ---\n", len(dir))
	fmt.Println("  Calls CreateProcessW via syscall (not exec.Command) with \\\\?\\\\ prefix.")
	fmt.Println("  Rules out Go exec.Command path normalisation as a factor vs Test 10.")
	{
		orig, _ := os.Getwd()
		if err := os.Chdir(dir); err != nil {
			fmt.Printf("  FAIL: Chdir: %v\n", err)
			ok = false
		} else {
			extDir := `\\?\` + dir
			fmt.Printf("  lpCurrentDirectory (len=%d): %s\n", len(extDir), extDir)

			selfW, _ := syscall.UTF16PtrFromString(self)
			cmdLineW, _ := syscall.UTF16PtrFromString(`"` + self + `" -child`)
			cwdW, _ := syscall.UTF16PtrFromString(extDir)

			var si syscall.StartupInfo
			si.Cb = uint32(unsafe.Sizeof(si))
			var pi syscall.ProcessInformation

			r, _, e := createProcessW_.Call(
				uintptr(unsafe.Pointer(selfW)),
				uintptr(unsafe.Pointer(cmdLineW)),
				0, 0, // process/thread security attrs
				0, // bInheritHandles
				0, // dwCreationFlags
				0, // lpEnvironment (inherit)
				uintptr(unsafe.Pointer(cwdW)),
				uintptr(unsafe.Pointer(&si)),
				uintptr(unsafe.Pointer(&pi)),
			)
			os.Chdir(orig)
			if r == 0 {
				fmt.Printf("  FAIL: CreateProcessW: %v\n", e)
				ok = false
			} else {
				waitForSingleObject.Call(uintptr(pi.Process), 0xFFFFFFFF) // INFINITE
				var exitCode uint32
				getExitCodeProcess.Call(uintptr(pi.Process), uintptr(unsafe.Pointer(&exitCode)))
				closeHandle_.Call(uintptr(pi.Thread))
				closeHandle_.Call(uintptr(pi.Process))
				if exitCode != 0 {
					fmt.Printf("  FAIL: child exited with code %d\n", exitCode)
					ok = false
				} else {
					fmt.Println("  PASS")
				}
			}
		}
	}

	if !ok {
		os.Exit(1)
	}
}
