// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package processmanager

// These tests change the process umask, so none of them runs in parallel.

import (
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
)

const sysprobeEntry = "datadog-agent-sysprobe.yaml"

var quarantineName = regexp.MustCompile(`^processes\.d\.untrusted-\d+-[0-9a-f]{8}$`)

// stubOwnerUID makes every path look owned by uid, except the names in overrides.
func stubOwnerUID(t *testing.T, uid uint32, overrides map[string]uint32) {
	t.Helper()
	orig := fileOwnerUID
	fileOwnerUID = func(fi fs.FileInfo) uint32 {
		if o, ok := overrides[fi.Name()]; ok {
			return o
		}
		return uid
	}
	t.Cleanup(func() { fileOwnerUID = orig })
}

func setUmask(t *testing.T, mask int) {
	t.Helper()
	orig := syscall.Umask(mask)
	t.Cleanup(func() { syscall.Umask(orig) })
}

// requireNoACL strips an inherited ACL from path and fails only if one remains. A shared CI
// runner can inherit an ACL on its temp dir, which no code change could fix.
func requireNoACL(t *testing.T, path string) {
	t.Helper()
	_ = exec.Command("/bin/chmod", "-N", path).Run()
	acl, err := hasACL(path)
	require.NoError(t, err)
	require.False(t, acl, "%s still carries an ACL after chmod -N", path)
}

// addACL grants the test user perms on path. GNU chmod rejects +a, so chmod is named by path.
func addACL(t *testing.T, path, perms string) {
	t.Helper()
	u, err := user.Current()
	require.NoError(t, err)
	out, err := exec.Command("/bin/chmod", "+a", "user:"+u.Username+" allow "+perms, path).CombinedOutput()
	require.NoError(t, err, "chmod +a: %s", out)
	acl, err := hasACL(path)
	require.NoError(t, err)
	require.True(t, acl)
}

// newInstallRoot returns a trusted install root, with every path reported as owned by root.
func newInstallRoot(t *testing.T) string {
	t.Helper()
	stubOwnerUID(t, 0, nil)
	root := filepath.Join(t.TempDir(), "datadog-agent")
	require.NoError(t, os.Mkdir(root, 0o755))
	require.NoError(t, os.Chmod(root, 0o755))
	requireNoACL(t, root)
	return root
}

func mkdirMode(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	require.NoError(t, os.Mkdir(path, mode))
	require.NoError(t, os.Chmod(path, mode))
}

func writeFileMode(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), mode))
	require.NoError(t, os.Chmod(path, mode))
}

// restoreModeOnCleanup puts path back to 0o755 so t.TempDir can remove the tree.
func restoreModeOnCleanup(t *testing.T, path string) {
	t.Helper()
	t.Cleanup(func() { _ = os.Chmod(path, 0o755) })
}

func skipIfRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores the directory mode, so the operation would succeed")
	}
}

func shippedEntry(t *testing.T) []byte {
	t.Helper()
	content, err := embedded.GetDarwinProcmgrProcess(sysprobeEntry)
	require.NoError(t, err)
	return content
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func quarantines(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	for _, name := range dirNames(t, root) {
		if strings.HasPrefix(name, "processes.d.untrusted-") {
			assert.Regexp(t, quarantineName, name)
			out = append(out, name)
		}
	}
	return out
}

// assertShippedCatalog checks processes.d holds the shipped entry, root-only and without ACLs.
func assertShippedCatalog(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, "processes.d")
	dirInfo, err := os.Lstat(dir)
	require.NoError(t, err)
	assert.True(t, dirInfo.IsDir())
	assert.Equal(t, os.FileMode(0o755), dirInfo.Mode().Perm())

	entry := filepath.Join(dir, sysprobeEntry)
	fi, err := os.Lstat(entry)
	require.NoError(t, err)
	assert.True(t, fi.Mode().IsRegular())
	assert.Equal(t, os.FileMode(0o644), fi.Mode().Perm())
	content, err := os.ReadFile(entry)
	require.NoError(t, err)
	assert.Equal(t, shippedEntry(t), content)

	for _, p := range []string{dir, entry} {
		acl, err := hasACL(p)
		require.NoError(t, err)
		assert.False(t, acl, "%s carries an ACL", p)
	}
	for _, name := range dirNames(t, dir) {
		assert.NotContains(t, name, ".tmp-", "temp file left behind")
	}
}

func TestWriteDarwinProcmgrCatalogWritesTheEntry(t *testing.T) {
	for _, mask := range []int{0o077, 0o000} {
		t.Run(fs.FileMode(mask).String(), func(t *testing.T) {
			root := newInstallRoot(t)
			setUmask(t, mask)
			require.NoError(t, WriteDarwinProcmgrCatalog(root))
			assertShippedCatalog(t, root)
			assert.Equal(t, []string{sysprobeEntry}, dirNames(t, filepath.Join(root, "processes.d")))
		})
	}
}

func TestWriteDarwinProcmgrCatalogIsIdempotentAndReplacesStaleContent(t *testing.T) {
	root := newInstallRoot(t)
	require.NoError(t, WriteDarwinProcmgrCatalog(root))
	require.NoError(t, WriteDarwinProcmgrCatalog(root))
	assertShippedCatalog(t, root)

	writeFileMode(t, filepath.Join(root, "processes.d", sysprobeEntry), "auto_start: true\n", 0o644)
	require.NoError(t, WriteDarwinProcmgrCatalog(root))
	assertShippedCatalog(t, root)
}

func TestWriteDarwinProcmgrCatalogRefusesAnUntrustedInstallRoot(t *testing.T) {
	cases := map[string]func(t *testing.T, root string) string{
		"non-root owner": func(t *testing.T, root string) string {
			stubOwnerUID(t, 501, nil)
			return root
		},
		"mode 0777": func(t *testing.T, root string) string {
			require.NoError(t, os.Chmod(root, 0o777))
			return root
		},
		"symlink": func(t *testing.T, root string) string {
			link := filepath.Join(t.TempDir(), "link")
			require.NoError(t, os.Symlink(root, link))
			return link
		},
		"ACL": func(t *testing.T, root string) string {
			addACL(t, root, "add_file,file_inherit,directory_inherit")
			return root
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			root := newInstallRoot(t)
			target := setup(t, root)
			err := WriteDarwinProcmgrCatalog(target)
			require.ErrorIs(t, err, ErrUntrustedCatalog)
			assert.NoFileExists(t, filepath.Join(root, "processes.d", sysprobeEntry))
			assert.NoDirExists(t, filepath.Join(root, "processes.d"))
		})
	}
}

func TestWriteDarwinProcmgrCatalogMissingInstallRootIsUntrusted(t *testing.T) {
	stubOwnerUID(t, 0, nil)
	err := WriteDarwinProcmgrCatalog(filepath.Join(t.TempDir(), "missing"))
	require.ErrorIs(t, err, ErrUntrustedCatalog)
}

func TestWriteDarwinProcmgrCatalogQuarantinesAnUntrustedProcessesDir(t *testing.T) {
	// Each setup plants an untrusted processes.d holding evil.yaml (or is a file) and returns a
	// check that the original was not modified.
	cases := map[string]func(t *testing.T, root string) func(*testing.T, string){
		"symlink outside the root": func(t *testing.T, root string) func(*testing.T, string) {
			target := t.TempDir()
			writeFileMode(t, filepath.Join(target, "evil.yaml"), "command: /bin/evil\n", 0o644)
			require.NoError(t, os.Symlink(target, filepath.Join(root, "processes.d")))
			return func(t *testing.T, quarantine string) {
				fi, err := os.Lstat(quarantine)
				require.NoError(t, err)
				assert.NotZero(t, fi.Mode()&fs.ModeSymlink, "the link itself is moved aside")
				assert.Equal(t, []string{"evil.yaml"}, dirNames(t, target), "the symlink target is untouched")
			}
		},
		"regular file": func(t *testing.T, root string) func(*testing.T, string) {
			writeFileMode(t, filepath.Join(root, "processes.d"), "not a directory\n", 0o644)
			return func(t *testing.T, quarantine string) {
				content, err := os.ReadFile(quarantine)
				require.NoError(t, err)
				assert.Equal(t, "not a directory\n", string(content))
			}
		},
		"mode 0777": func(t *testing.T, root string) func(*testing.T, string) {
			dir := filepath.Join(root, "processes.d")
			mkdirMode(t, dir, 0o777)
			writeFileMode(t, filepath.Join(dir, "evil.yaml"), "command: /bin/evil\n", 0o644)
			return assertQuarantineHoldsEvil
		},
		"non-root owner": func(t *testing.T, root string) func(*testing.T, string) {
			stubOwnerUID(t, 0, map[string]uint32{"processes.d": 501})
			dir := filepath.Join(root, "processes.d")
			mkdirMode(t, dir, 0o755)
			writeFileMode(t, filepath.Join(dir, "evil.yaml"), "command: /bin/evil\n", 0o644)
			return assertQuarantineHoldsEvil
		},
		"ACL": func(t *testing.T, root string) func(*testing.T, string) {
			dir := filepath.Join(root, "processes.d")
			mkdirMode(t, dir, 0o755)
			writeFileMode(t, filepath.Join(dir, "evil.yaml"), "command: /bin/evil\n", 0o644)
			addACL(t, dir, "add_file,file_inherit,directory_inherit")
			return assertQuarantineHoldsEvil
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			root := newInstallRoot(t)
			check := setup(t, root)
			require.NoError(t, WriteDarwinProcmgrCatalog(root))

			assertShippedCatalog(t, root)
			assert.NoFileExists(t, filepath.Join(root, "processes.d", "evil.yaml"))
			q := quarantines(t, root)
			require.Len(t, q, 1)
			check(t, filepath.Join(root, q[0]))
		})
	}
}

func assertQuarantineHoldsEvil(t *testing.T, quarantine string) {
	t.Helper()
	assert.Equal(t, []string{"evil.yaml"}, dirNames(t, quarantine))
}

func TestWriteDarwinProcmgrCatalogKeepsEveryQuarantine(t *testing.T) {
	root := newInstallRoot(t)
	dir := filepath.Join(root, "processes.d")
	mkdirMode(t, dir, 0o777)
	require.NoError(t, WriteDarwinProcmgrCatalog(root))
	require.NoError(t, os.Chmod(dir, 0o777))
	require.NoError(t, WriteDarwinProcmgrCatalog(root))

	assert.Len(t, quarantines(t, root), 2)
	assertShippedCatalog(t, root)
}

func TestWriteDarwinProcmgrCatalogFailedQuarantineIsUntrusted(t *testing.T) {
	skipIfRoot(t)
	root := newInstallRoot(t)
	dir := filepath.Join(root, "processes.d")
	mkdirMode(t, dir, 0o777)
	require.NoError(t, os.Chmod(root, 0o555))
	restoreModeOnCleanup(t, root)

	err := WriteDarwinProcmgrCatalog(root)
	require.ErrorIs(t, err, ErrUntrustedCatalog)
	fi, err := os.Lstat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o777), fi.Mode().Perm(), "processes.d was not touched")
}

func TestWriteDarwinProcmgrCatalogReplacesTheManagedEntry(t *testing.T) {
	cases := map[string]func(t *testing.T, outside, entry string){
		"symlink": func(t *testing.T, outside, entry string) {
			require.NoError(t, os.Symlink(outside, entry))
		},
		"hard link": func(t *testing.T, outside, entry string) {
			require.NoError(t, os.Link(outside, entry))
		},
	}
	for name, plant := range cases {
		t.Run(name, func(t *testing.T) {
			root := newInstallRoot(t)
			dir := filepath.Join(root, "processes.d")
			mkdirMode(t, dir, 0o755)
			outside := filepath.Join(t.TempDir(), "outside.yaml")
			writeFileMode(t, outside, "auto_start: true\n", 0o666)
			plant(t, outside, filepath.Join(dir, sysprobeEntry))

			require.NoError(t, WriteDarwinProcmgrCatalog(root))
			assertShippedCatalog(t, root)
			content, err := os.ReadFile(outside)
			require.NoError(t, err)
			assert.Equal(t, "auto_start: true\n", string(content))
		})
	}
}

func TestWriteDarwinProcmgrCatalogLeftoverManagedEntryIsUntrusted(t *testing.T) {
	t.Run("symlink", func(t *testing.T) {
		skipIfRoot(t)
		root := newInstallRoot(t)
		dir := filepath.Join(root, "processes.d")
		mkdirMode(t, dir, 0o755)
		outside := filepath.Join(t.TempDir(), "outside.yaml")
		writeFileMode(t, outside, "auto_start: true\n", 0o644)
		entry := filepath.Join(dir, sysprobeEntry)
		require.NoError(t, os.Symlink(outside, entry))
		require.NoError(t, os.Chmod(dir, 0o555))
		restoreModeOnCleanup(t, dir)

		err := WriteDarwinProcmgrCatalog(root)
		require.ErrorIs(t, err, ErrUntrustedCatalog)
		fi, err := os.Lstat(entry)
		require.NoError(t, err)
		assert.NotZero(t, fi.Mode()&fs.ModeSymlink, "the symlink remains")
	})
	t.Run("stale auto_start true", func(t *testing.T) {
		skipIfRoot(t)
		root := newInstallRoot(t)
		dir := filepath.Join(root, "processes.d")
		mkdirMode(t, dir, 0o755)
		entry := filepath.Join(dir, sysprobeEntry)
		writeFileMode(t, entry, "auto_start: true\n", 0o644)
		require.NoError(t, os.Chmod(dir, 0o555))
		restoreModeOnCleanup(t, dir)

		err := WriteDarwinProcmgrCatalog(root)
		require.ErrorIs(t, err, ErrUntrustedCatalog)
		content, err := os.ReadFile(entry)
		require.NoError(t, err)
		assert.Equal(t, "auto_start: true\n", string(content))
	})
}

func TestWriteDarwinProcmgrCatalogMissingEntryThatCannotBeCreatedIsOrdinary(t *testing.T) {
	skipIfRoot(t)
	root := newInstallRoot(t)
	dir := filepath.Join(root, "processes.d")
	mkdirMode(t, dir, 0o555)
	restoreModeOnCleanup(t, dir)

	err := WriteDarwinProcmgrCatalog(root)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrUntrustedCatalog)
	assert.Empty(t, dirNames(t, dir))
}

func TestWriteDarwinProcmgrCatalogRefusesAnUntrustedSibling(t *testing.T) {
	for _, sibling := range []string{"custom.yaml", "custom.yml", ".custom.yaml", ".custom.yml"} {
		cases := map[string]func(t *testing.T, path string){
			"mode 0666": func(t *testing.T, path string) {
				writeFileMode(t, path, "command: /bin/custom\n", 0o666)
			},
			"non-root owner": func(t *testing.T, path string) {
				stubOwnerUID(t, 0, map[string]uint32{sibling: 501})
				writeFileMode(t, path, "command: /bin/custom\n", 0o644)
			},
			"ACL": func(t *testing.T, path string) {
				writeFileMode(t, path, "command: /bin/custom\n", 0o644)
				addACL(t, path, "write")
			},
		}
		for name, plant := range cases {
			t.Run(sibling+"/"+name, func(t *testing.T) {
				root := newInstallRoot(t)
				dir := filepath.Join(root, "processes.d")
				mkdirMode(t, dir, 0o755)
				plant(t, filepath.Join(dir, sibling))
				before := dirNames(t, dir)

				err := WriteDarwinProcmgrCatalog(root)
				require.ErrorIs(t, err, ErrUntrustedCatalog)
				assert.Equal(t, before, dirNames(t, dir), "processes.d is unchanged")
				assert.NoFileExists(t, filepath.Join(dir, sysprobeEntry))
				content, err := os.ReadFile(filepath.Join(dir, sibling))
				require.NoError(t, err)
				assert.Equal(t, "command: /bin/custom\n", string(content))
				assert.Empty(t, quarantines(t, root))
			})
		}
	}
}

func TestWriteDarwinProcmgrCatalogKeepsATrustedSibling(t *testing.T) {
	root := newInstallRoot(t)
	dir := filepath.Join(root, "processes.d")
	mkdirMode(t, dir, 0o755)
	writeFileMode(t, filepath.Join(dir, "custom.yaml"), "command: /bin/custom\n", 0o644)

	require.NoError(t, WriteDarwinProcmgrCatalog(root))
	assertShippedCatalog(t, root)
	assert.Equal(t, []string{"custom.yaml", sysprobeEntry}, dirNames(t, dir))
}

func TestWriteDarwinProcmgrCatalogIgnoresNonYAMLDotfiles(t *testing.T) {
	root := newInstallRoot(t)
	dir := filepath.Join(root, "processes.d")
	mkdirMode(t, dir, 0o755)
	writeFileMode(t, filepath.Join(dir, ".hidden.txt"), "notes\n", 0o666)

	require.NoError(t, WriteDarwinProcmgrCatalog(root))
	assertShippedCatalog(t, root)
	assert.Equal(t, []string{".hidden.txt", sysprobeEntry}, dirNames(t, dir))
}

func TestRemoveDarwinProcmgrCatalog(t *testing.T) {
	t.Run("keeps a foreign entry and the directory", func(t *testing.T) {
		root := newInstallRoot(t)
		require.NoError(t, WriteDarwinProcmgrCatalog(root))
		dir := filepath.Join(root, "processes.d")
		writeFileMode(t, filepath.Join(dir, "custom.yaml"), "command: /bin/custom\n", 0o644)

		require.NoError(t, RemoveDarwinProcmgrCatalog(root))
		assert.Equal(t, []string{"custom.yaml"}, dirNames(t, dir))
	})
	t.Run("removes the directory it emptied", func(t *testing.T) {
		root := newInstallRoot(t)
		require.NoError(t, WriteDarwinProcmgrCatalog(root))

		require.NoError(t, RemoveDarwinProcmgrCatalog(root))
		assert.NoDirExists(t, filepath.Join(root, "processes.d"))
	})
	t.Run("missing entry", func(t *testing.T) {
		root := newInstallRoot(t)
		dir := filepath.Join(root, "processes.d")
		mkdirMode(t, dir, 0o755)
		writeFileMode(t, filepath.Join(dir, "custom.yaml"), "command: /bin/custom\n", 0o644)

		require.NoError(t, RemoveDarwinProcmgrCatalog(root))
		assert.Equal(t, []string{"custom.yaml"}, dirNames(t, dir))
	})
	t.Run("missing directory", func(t *testing.T) {
		root := newInstallRoot(t)
		require.NoError(t, RemoveDarwinProcmgrCatalog(root))
	})
	t.Run("missing install root", func(t *testing.T) {
		stubOwnerUID(t, 0, nil)
		require.NoError(t, RemoveDarwinProcmgrCatalog(filepath.Join(t.TempDir(), "missing")))
	})
	t.Run("symlinked processes.d", func(t *testing.T) {
		root := newInstallRoot(t)
		target := t.TempDir()
		writeFileMode(t, filepath.Join(target, sysprobeEntry), "auto_start: false\n", 0o644)
		require.NoError(t, os.Symlink(target, filepath.Join(root, "processes.d")))

		require.NoError(t, RemoveDarwinProcmgrCatalog(root))
		assert.FileExists(t, filepath.Join(target, sysprobeEntry))
	})
	t.Run("untrusted processes.d", func(t *testing.T) {
		root := newInstallRoot(t)
		dir := filepath.Join(root, "processes.d")
		mkdirMode(t, dir, 0o777)
		writeFileMode(t, filepath.Join(dir, sysprobeEntry), "auto_start: false\n", 0o644)

		require.NoError(t, RemoveDarwinProcmgrCatalog(root))
		assert.FileExists(t, filepath.Join(dir, sysprobeEntry))
	})
}
