// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package ntp

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetNTPServersFromFileNotExist(t *testing.T) {
	_, err := getNTPServersFromFiles([]string{"file1", "file2"})
	assert.EqualError(t, err, "Cannot find NTP server in file1, file2")
}

func createTempFile(t *testing.T, content string, callback func(filename string)) {
	file, err := os.CreateTemp("", "")

	filename := file.Name()
	defer os.Remove(filename)
	assert.NoError(t, err)

	os.WriteFile(filename, []byte(content), 0)
	callback(filename)
}

func TestGetNTPServersFromFile(t *testing.T) {
	config := `
		# --- GENERAL CONFIGURATION ---
		server  aaa.bbb.ccc.ddd
		 server  127.127.1.0
		#server  127.0.0.1
		fudge   127.127.1.0 stratum 10
		`
	createTempFile(t, config, func(f1 string) {
		servers, err := getNTPServersFromFiles([]string{f1})
		assert.NoError(t, err)
		sort.Strings(servers)
		assert.Equal(t, []string{"127.127.1.0", "aaa.bbb.ccc.ddd"}, servers)
	})
}

func TestGetNTPServersFromFileTwoConfigs(t *testing.T) {
	config1 := "server  aaa.bbb.ccc.ddd"
	config2 := "server  127.0.0.1"

	createTempFile(t, config1, func(f1 string) {
		createTempFile(t, config2, func(f2 string) {
			servers, err := getNTPServersFromFiles([]string{f1, f2})
			assert.NoError(t, err)
			sort.Strings(servers)
			assert.Equal(t, []string{"127.0.0.1", "aaa.bbb.ccc.ddd"}, servers)
		})
	})
}

func TestGetNTPPoolsFromChronyConfig(t *testing.T) {
	config := `
pool  127.0.0.1
pool  aaa.bbb.ccc.ddd
`
	createTempFile(t, config, func(f1 string) {
		servers, err := getNTPServersFromFiles([]string{f1})
		assert.NoError(t, err)
		sort.Strings(servers)
		assert.Equal(t, []string{"127.0.0.1", "aaa.bbb.ccc.ddd"}, servers)
	})
}

func TestGetNTPPoolsAndServersFromChronyConfig(t *testing.T) {
	config := `
server  127.0.0.1
pool  aaa.bbb.ccc.ddd
`

	createTempFile(t, config, func(f1 string) {
		servers, err := getNTPServersFromFiles([]string{f1})
		assert.NoError(t, err)
		sort.Strings(servers)
		assert.Equal(t, []string{"127.0.0.1", "aaa.bbb.ccc.ddd"}, servers)
	})
}

func TestGetNTPPeersFromChronyConfig(t *testing.T) {
	config := `
peer  127.0.0.1
peer  aaa.bbb.ccc.ddd
`
	createTempFile(t, config, func(f1 string) {
		servers, err := getNTPServersFromFiles([]string{f1})
		assert.NoError(t, err)
		sort.Strings(servers)
		assert.Equal(t, []string{"127.0.0.1", "aaa.bbb.ccc.ddd"}, servers)
	})
}

func TestGetNTPPoolsAndServersAndPeersFromChronyConfig(t *testing.T) {
	config := `
peer  127.0.0.1
server  aaa.bbb.ccc.ddd
pool 10.0.0.1
`

	createTempFile(t, config, func(f1 string) {
		servers, err := getNTPServersFromFiles([]string{f1})
		assert.NoError(t, err)
		sort.Strings(servers)
		assert.Equal(t, []string{"10.0.0.1", "127.0.0.1", "aaa.bbb.ccc.ddd"}, servers)
	})
}

func TestGetNTPServersFromFileNoDuplicate(t *testing.T) {
	config := `
server  aaa.bbb.ccc.ddd
server  aaa.bbb.ccc.ddd
`
	createTempFile(t, config, func(f1 string) {
		servers, err := getNTPServersFromFiles([]string{f1})
		assert.NoError(t, err)
		assert.Equal(t, []string{"aaa.bbb.ccc.ddd"}, servers)
	})
}

func TestGetNTPServersFromFileNoServer(t *testing.T) {
	createTempFile(t, "", func(f1 string) {
		servers, err := getNTPServersFromFiles([]string{f1})
		assert.Error(t, err)
		assert.Equal(t, []string(nil), servers)
	})
}

func TestGetNTPServersFromTimesyncdConfig(t *testing.T) {
	config := `[Time]
NTP=time1.example.com time2.example.com
FallbackNTP=0.pool.ntp.org 1.pool.ntp.org
`
	createTempFile(t, config, func(f1 string) {
		servers, err := getNTPServersFromFiles([]string{f1})
		assert.NoError(t, err)
		sort.Strings(servers)
		assert.Equal(t, []string{"0.pool.ntp.org", "1.pool.ntp.org", "time1.example.com", "time2.example.com"}, servers)
	})
}

func TestGetNTPServersFromTimesyncdConfigEdgeCases(t *testing.T) {
	config := `# vendor defaults
[Time]
#NTP=
NTP=time1.example.com  time2.example.com   # trailing comment
FallbackNTP=
`
	createTempFile(t, config, func(f1 string) {
		servers, err := getNTPServersFromFiles([]string{f1})
		assert.NoError(t, err)
		sort.Strings(servers)
		assert.Equal(t, []string{"time1.example.com", "time2.example.com"}, servers)
	})
}

func TestGetLocalDefinedNTPServersIncludesTimesyncdPath(t *testing.T) {
	_, err := getLocalDefinedNTPServers()
	if err == nil {
		t.Skip("a real ntp/chrony/timesyncd config exists on this host")
	}
	assert.Contains(t, err.Error(), "/etc/systemd/timesyncd.conf")
}

// withTimesyncdDropInDirs swaps the package-level drop-in dir list for the
// duration of the test and restores it on cleanup.
func withTimesyncdDropInDirs(t *testing.T, dirs []string) {
	orig := timesyncdDropInDirs
	timesyncdDropInDirs = dirs
	t.Cleanup(func() { timesyncdDropInDirs = orig })
}

func TestGetLocalDefinedNTPServersReadsTimesyncdDropIn(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "cloud-init.conf"),
		[]byte("[Time]\nNTP=dropin-host.example\n"), 0644)
	assert.NoError(t, err)

	withTimesyncdDropInDirs(t, []string{dir})

	servers, err := getLocalDefinedNTPServers()
	assert.NoError(t, err)
	assert.Contains(t, servers, "dropin-host.example")
}

// TestTimesyncdDropInDirsMatchSystemdDocs guards the production constant. The
// behavioral tests swap this var out for a t.TempDir() path, so a typo in any
// of these strings would not be caught by them. Pinning the list here forces
// any change to be deliberate.
func TestTimesyncdDropInDirsMatchSystemdDocs(t *testing.T) {
	assert.Equal(t, []string{
		"/etc/systemd/timesyncd.conf.d",
		"/run/systemd/timesyncd.conf.d",
		"/usr/local/lib/systemd/timesyncd.conf.d",
		"/usr/lib/systemd/timesyncd.conf.d",
	}, timesyncdDropInDirs)
}
