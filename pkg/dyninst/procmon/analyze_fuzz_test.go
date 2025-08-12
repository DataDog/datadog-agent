//go:build linux_bpf

package procmon

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// createFuzzProcFS creates a proc filesystem structure for fuzzing
func createFuzzProcFS(t *testing.T, pid uint32, environ []byte, exeContent []byte, withExe bool, exeLinkTarget string) (string, string, func()) {
	tmpDir := t.TempDir()
	procRoot := filepath.Join(tmpDir, "proc")

	procDir := filepath.Join(procRoot, strconv.Itoa(int(pid)))
	if err := os.MkdirAll(procDir, 0o755); err != nil {
		t.Fatal(err)
	}

	environPath := filepath.Join(procDir, "environ")
	if err := os.WriteFile(environPath, environ, 0o644); err != nil {
		t.Fatal(err)
	}

	exeTarget := filepath.Join(tmpDir, "exe_target")
	if err := os.WriteFile(exeTarget, exeContent, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create exe symlink
	exeLink := filepath.Join(procDir, "exe")
	linkTarget := exeTarget
	if exeLinkTarget != "" {
		linkTarget = exeLinkTarget
	}
	if err := os.Symlink(linkTarget, exeLink); err != nil {
		t.Fatal(err)
	}

	// Create root directory for container-like scenarios
	rootDir := filepath.Join(procDir, "root")
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		t.Fatal(err)
	}

	return tmpDir, procRoot, func() {
		os.RemoveAll(tmpDir)
	}
}

func FuzzAnalyzeProcess(f *testing.F) {
	f.Add(uint32(1001), []byte("DD_DYNINST_ENABLED=true;DD_SERVICE=test-service"), []byte("fake-exe"))
	f.Add(uint32(1002), []byte("OTHER_VAR=value"), []byte("fake-exe"))
	f.Add(uint32(1003), []byte("DD_DYNINST_ENABLED=true;DD_SERVICE=git-service;DD_GIT_COMMIT_SHA=abc123def456;DD_GIT_REPOSITORY_URL=https://github.com/example/repo"), []byte("fake-exe"))

	f.Fuzz(func(t *testing.T, pid uint32, environData []byte, exeContent []byte) {
		analyzer := makeExecutableAnalyzer(10)
		exeLinkTarget := "exe"
		_, procRoot, cleanup := createFuzzProcFS(t, pid, environData, exeContent, true, exeLinkTarget)
		defer cleanup()

		procDir := filepath.Join(procRoot, strconv.Itoa(int(pid)))
		rootDir := filepath.Join(procDir, "root")

		// Create the exe symlink pointing to the container path
		exeLink := filepath.Join(procDir, "exe")
		if err := os.Remove(exeLink); err == nil {
			if err := os.Symlink(exeLinkTarget, exeLink); err != nil {
				t.Skip("failed to create exe symlink")
			}
		}

		// Create the actual executable file in the container root
		containerExePath := filepath.Join(rootDir, strings.TrimPrefix(exeLinkTarget, "/"))
		containerExeDir := filepath.Dir(containerExePath)
		if err := os.MkdirAll(containerExeDir, 0o755); err != nil {
			t.Skip("failed to create container exe dir")
		}
		if err := os.WriteFile(containerExePath, exeContent, 0o755); err != nil {
			t.Skip("failed to write container exe")
		}

		resolver := noopContainerResolver{}
		// Call the function under test
		result, err := analyzeProcess(pid, procRoot, resolver, analyzer)

		if err == nil {
			if result.exe.Path != "" {
				_ = result.exe.String()
				_ = result.exe.Key.String()
			}

			// Test the analyzer cache if it was used
			if result.exe.Path != "" {
				// Open the exe file to test cache behavior
				if exeFile, openErr := os.Open(result.exe.Path); openErr == nil {
					// Test cache hit
					_, _ = analyzer.isInteresting(exeFile, result.exe.Key)
					// Test checkFileKeyCache
					_, _ = analyzer.checkFileKeyCache(result.exe.Key)
					exeFile.Close()
				}
			}
		}

		if err != nil {
			// exercise the Error() before exit
			_ = err.Error()
			return
		}

		// Test with modified file times to exercise FileKey logic
		if result.exe.Path != "" {
			if stat, statErr := os.Stat(result.exe.Path); statErr == nil {
				if statSys, ok := stat.Sys().(*syscall.Stat_t); ok {
					newKey := FileKey{
						FileHandle: FileHandle{
							Dev: uint64(statSys.Dev),
							Ino: statSys.Ino,
						},
						LastModified: syscall.Timespec{
							Sec:  statSys.Mtim.Sec + 1,
							Nsec: statSys.Mtim.Nsec,
						},
					}
					// Exercise the String method
					_ = newKey.String()
				}
			}
		}
	})
}
