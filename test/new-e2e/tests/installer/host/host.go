// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package host provides a way to interact with an e2e remote host and capture its state.
package host

import (
	"fmt"
	"io/fs"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Host is a remote host environment.
type Host struct {
	t              *testing.T
	remote         *components.RemoteHost
	os             e2eos.Descriptor
	arch           e2eos.Architecture
	systemdVersion int
	pkgManager     string
}

// Option is an option to configure a Host.
type Option func(*testing.T, *Host)

// New creates a new Host.
func New(t *testing.T, remote *components.RemoteHost, os e2eos.Descriptor, arch e2eos.Architecture, opts ...Option) *Host {
	host := &Host{
		t:      t,
		remote: remote,
		os:     os,
		arch:   arch,
	}
	for _, opt := range opts {
		opt(t, host)
	}
	host.uploadFixtures()
	host.setSystemdVersion()
	if _, err := host.remote.Execute("command -v dpkg-query"); err == nil {
		host.pkgManager = "dpkg"
	} else if _, err := host.remote.Execute("command -v rpm"); err == nil {
		host.pkgManager = "rpm"
	} else {
		t.Fatal("no package manager found")
	}
	return host
}

// GetPkgManager returns the package manager of the host.
func (h *Host) GetPkgManager() string {
	return h.pkgManager
}

func (h *Host) setSystemdVersion() {
	strVersion := strings.TrimSpace(h.remote.MustExecute("systemctl --version | head -n1 | awk '{print $2}'"))
	version, err := strconv.Atoi(strVersion)
	require.NoError(h.t, err)
	h.systemdVersion = version
}

// InstallDocker installs Docker on the host if it is not already installed.
func (h *Host) InstallDocker() {
	if _, err := h.remote.Execute("command -v docker"); err == nil {
		return
	}

	switch h.os.Flavor {
	case e2eos.AmazonLinux:
		h.remote.MustExecute(`sudo sh -c "yum -y install docker && systemctl start docker"`)
	default:
		h.remote.MustExecute("curl -fsSL https://get.docker.com | sudo sh")
	}
}

// GetDockerRuntimePath returns the runtime path of a docker runtime
func (h *Host) GetDockerRuntimePath(runtime string) string {
	var cmd string
	switch h.os.Flavor {
	case e2eos.AmazonLinux:
		cmd = "sudo docker system info --format '{{ (index .Runtimes \"%s\").Path }}'"
	default:
		cmd = "sudo docker system info --format '{{ (index .Runtimes \"%s\").Runtime.Path }}'"
	}
	return strings.TrimSpace(h.remote.MustExecute(fmt.Sprintf(cmd, runtime)))
}

// Run executes a command on the host.
func (h *Host) Run(command string, env ...string) string {
	envVars := make(map[string]string)
	for _, e := range env {
		parts := strings.Split(e, "=")
		envVars[parts[0]] = parts[1]
	}
	return h.remote.MustExecute(command, client.WithEnvVariables(envVars))
}

// FileExists checks if a file exists on the host.
func (h *Host) FileExists(path string) (bool, error) {
	return h.remote.FileExists(path)
}

// ReadFile reads a file from the host.
func (h *Host) ReadFile(path string) ([]byte, error) {
	return h.remote.ReadFile(path)
}

// DeletePath deletes a path on the host.
func (h *Host) DeletePath(path string) {
	h.remote.MustExecute(fmt.Sprintf("sudo ls %s", path))
	h.remote.MustExecute(fmt.Sprintf("sudo rm -rf %s", path))
}

// WaitForUnitActive waits for a systemd unit to be active
func (h *Host) WaitForUnitActive(units ...string) {
	for _, unit := range units {
		_, err := h.remote.Execute(fmt.Sprintf("timeout=30; unit=%s; while ! systemctl is-active --quiet $unit && [ $timeout -gt 0 ]; do sleep 1; ((timeout--)); done; [ $timeout -ne 0 ]", unit))
		require.NoError(h.t, err, "unit %s did not become active", unit)
	}
}

// BootstraperVersion returns the version of the bootstraper on the host.
func (h *Host) BootstraperVersion() string {
	return strings.TrimSpace(h.remote.MustExecute("sudo datadog-bootstrap version"))
}

// InstallerVersion returns the version of the installer on the host.
func (h *Host) InstallerVersion() string {
	return strings.TrimSpace(h.remote.MustExecute("sudo datadog-installer version"))
}

// AssertPackageInstalledByInstaller checks if a package is installed by the installer on the host.
func (h *Host) AssertPackageInstalledByInstaller(pkgs ...string) {
	for _, pkg := range pkgs {
		h.remote.MustExecute("sudo datadog-installer is-installed " + pkg)
	}
}

// AssertPackageInstalledByPackageManager checks if a package is installed by the package manager on the host.
func (h *Host) AssertPackageInstalledByPackageManager(pkgs ...string) {
	for _, pkg := range pkgs {
		switch h.pkgManager {
		case "dpkg":
			h.remote.MustExecute("dpkg-query -l " + pkg)
		case "rpm":
			h.remote.MustExecute("rpm -q " + pkg)
		default:
			h.t.Fatal("unsupported package manager")
		}
	}
}

// AssertPackageNotInstalledByPackageManager checks if a package is not installed by the package manager on the host.
func (h *Host) AssertPackageNotInstalledByPackageManager(pkgs ...string) {
	for _, pkg := range pkgs {
		switch h.pkgManager {
		case "dpkg":
			h.remote.MustExecute("! dpkg-query -l " + pkg)
		case "rpm":
			h.remote.MustExecute("! rpm -q " + pkg)
		default:
			h.t.Fatal("unsupported package manager")
		}
	}
}

// State returns the state of the host.
func (h *Host) State() State {
	return State{
		t:      h.t,
		Users:  h.users(),
		Groups: h.groups(),
		FS:     h.fs(),
		Units:  h.getSystemdUnitInfo(),
	}
}

func (h *Host) users() []user.User {
	output := h.remote.MustExecute("sudo getent passwd")
	lines := strings.Split(output, "\n")
	var users []user.User
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, ":")
		assert.Len(h.t, parts, 7)
		users = append(users, user.User{
			Username: parts[0],
			Uid:      parts[2],
			Gid:      parts[3],
			Name:     parts[4],
			HomeDir:  parts[5],
		})
	}
	sort.Slice(users, func(i, j int) bool {
		return users[i].Uid < users[j].Uid
	})
	return users
}

func (h *Host) groups() []user.Group {
	output := h.remote.MustExecute("sudo getent group")
	lines := strings.Split(output, "\n")
	var groups []user.Group
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, ":")
		assert.Len(h.t, parts, 4)
		groups = append(groups, user.Group{
			Name: parts[0],
			Gid:  parts[2],
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Gid < groups[j].Gid
	})
	return groups
}

func (h *Host) fs() map[string]FileInfo {
	ignoreDirs := []string{
		"/proc",
		"/sys",
		"/dev",
		"/run/utmp",
		"/tmp",
	}
	cmd := "sudo find / "
	for _, dir := range ignoreDirs {
		cmd += fmt.Sprintf("-path '%s' -prune -o ", dir)
	}
	cmd += `-printf '%p\\|//%s\\|//%TY-%Tm-%Td %TH:%TM:%TS\\|//%f\\|//%m\\|//%u\\|//%g\\|//%y\\|//%l\n' 2>/dev/null`
	output := h.remote.MustExecute(cmd + " || true")
	lines := strings.Split(output, "\n")

	fileInfos := make(map[string]FileInfo)
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\\|//")
		assert.Len(h.t, parts, 9)

		path := parts[0]
		size, _ := strconv.ParseInt(parts[1], 10, 64)
		modTime, _ := time.Parse("2006-01-02 15:04:05", parts[2])
		name := parts[3]
		mode, _ := strconv.ParseUint(parts[4], 8, 32)
		user := parts[5]
		group := parts[6]
		fileType := parts[7]
		isDir := fileType == "d"
		isSymlink := fileType == "l"
		link := parts[8]

		fileInfos[path] = FileInfo{
			name:      name,
			size:      size,
			perms:     fs.FileMode(mode).Perm(),
			modTime:   modTime,
			isDir:     isDir,
			isSymlink: isSymlink,
			link:      link,
			user:      user,
			group:     group,
		}
	}
	return fileInfos
}

func (h *Host) getSystemdUnitInfo() map[string]SystemdUnitInfo {
	// Retrieve the status of all units
	output := h.remote.MustExecute("sudo systemctl list-units --all --no-legend --no-pager")
	output = strings.ReplaceAll(output, "●", "") // Remove the bullet point
	unitsOutput := strings.Split(string(output), "\n")
	units := make(map[string]SystemdUnitInfo)

	// Retrieve the enabled state of unit files
	enabledOutput := h.remote.MustExecute("sudo systemctl list-unit-files --no-legend --no-pager")
	enabledOutput = strings.ReplaceAll(enabledOutput, "●", "") // Remove the bullet point
	enabledLines := strings.Split(string(enabledOutput), "\n")
	enabledMap := make(map[string]string)
	for _, line := range enabledLines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		enabledMap[fields[0]] = fields[1] // Map full unit name to enabled status
	}

	// Parse active state and match with enabled state
	for _, line := range unitsOutput {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		name := fields[0] // Full unit name with extension
		loadState := LoadState(fields[1])
		active := fields[2]
		subState := SubState(fields[3])

		enabled, exists := enabledMap[name]
		if !exists {
			enabled = "unknown" // Handle cases where the unit file is not listed
		}

		units[name] = SystemdUnitInfo{
			Name:      name,
			Active:    active,
			SubState:  subState,
			LoadState: loadState,
			Enabled:   enabled,
		}
	}

	return units
}

// LoadState is the load state of a systemd unit.
type LoadState string

// SubState is the sub state of a systemd unit.
type SubState string

const (
	// Loaded is the load state of a systemd unit.
	Loaded LoadState = "loaded"
	// NotLoaded is the load state of a systemd unit.
	NotLoaded LoadState = "not-found"
	// Masked is the load state of a systemd unit.
	Masked LoadState = "masked"
	// Error is the load state of a systemd unit.
	Error LoadState = "error"

	// Running is the sub state of a systemd unit.
	Running SubState = "running"
	// Dead is the sub state of a systemd unit.
	Dead SubState = "dead"
)

// SystemdUnitInfo is the info of a systemd unit.
type SystemdUnitInfo struct {
	Name      string
	Active    string
	Enabled   string
	SubState  SubState
	LoadState LoadState
}

// FileInfo struct mimics os.FileInfo
type FileInfo struct {
	name      string
	size      int64
	perms     fs.FileMode
	modTime   time.Time
	isDir     bool
	isSymlink bool
	link      string
	user      string
	group     string
}

// State is the state of a remote host.
type State struct {
	t      *testing.T
	Users  []user.User
	Groups []user.Group
	FS     map[string]FileInfo
	Units  map[string]SystemdUnitInfo
}

// AssertUserExists asserts that a user exists on the host.
func (s *State) AssertUserExists(userName string) {
	for _, user := range s.Users {
		if user.Username == userName {
			return
		}
	}
	assert.Fail(s.t, "user does not exist", userName)
}

// AssertGroupExists asserts that a group exists on the host.
func (s *State) AssertGroupExists(groupName string) {
	for _, group := range s.Groups {
		if group.Name == groupName {
			return
		}
	}
	assert.Fail(s.t, "group does not exist", groupName)
}

// AssertUserHasGroup asserts that a user has a group on the host.
func (s *State) AssertUserHasGroup(userName, groupName string) {
	for _, user := range s.Users {
		if user.Username == userName {
			for _, group := range s.Groups {
				if group.Name == groupName {
					if user.Gid == group.Gid {
						return
					}
				}
			}
		}
	}
	assert.Fail(s.t, "user does not have group", userName, groupName)
}

// evalSymlinkPath resolves the absolute path, resolving symlinks
func evalSymlinkPath(path string, fs map[string]FileInfo) string {
	// Normalize the path to clean up any .. or .
	path = filepath.Clean(path)

	// Split the path into components
	parts := strings.Split(path, "/")
	resolvedPath := "/"

	for _, part := range parts {
		if part == "" || part == "." {
			// Ignore empty part or current directory marker
			continue
		}

		// Append the current part to the resolved path
		nextPath := filepath.Join(resolvedPath, part)
		nextPath = filepath.Clean(nextPath) // Clean to ensure no trailing slashes

		// Check if the current path component is a symlink
		if fileInfo, exists := fs[nextPath]; exists && fileInfo.isSymlink {
			// Resolve the symlink
			symlinkTarget := fileInfo.link
			// Handle recursive symlink resolution
			symlinkTarget = evalSymlinkPath(symlinkTarget, fs)
			// Update the resolvedPath to be the target of the symlink
			resolvedPath = symlinkTarget
		} else {
			// Not a symlink, or doesn't exist in fs; move to next component
			resolvedPath = nextPath
		}

		// Ensure the path ends correctly for the next iteration
		if !strings.HasSuffix(resolvedPath, "/") && len(resolvedPath) > 1 {
			resolvedPath += "/"
		}
	}

	return filepath.Clean(resolvedPath)
}

// AssertDirExists asserts that a directory exists on the host with the given perms, user, and group.
func (s *State) AssertDirExists(path string, perms fs.FileMode, user string, group string) {
	path = evalSymlinkPath(path, s.FS)
	fileInfo, ok := s.FS[path]
	assert.True(s.t, ok, "dir %v does not exist", path)
	assert.True(s.t, fileInfo.isDir, "%v is not a directory", path)
	assert.Equal(s.t, perms, fileInfo.perms, "%v has unexpected perms", path)
	assert.Equal(s.t, user, fileInfo.user, "%v has unexpected user", path)
	assert.Equal(s.t, group, fileInfo.group, "%v has unexpected group", path)
}

// AssertPathDoesNotExist asserts that a path does not exist on the host.
func (s *State) AssertPathDoesNotExist(path string) {
	path = evalSymlinkPath(path, s.FS)
	_, ok := s.FS[path]
	assert.False(s.t, ok, "something exists at path", path)
}

// AssertFileExists asserts that a file exists on the host with the given perms, user, and group.
func (s *State) AssertFileExists(path string, perms fs.FileMode, user string, group string) {
	path = evalSymlinkPath(path, s.FS)
	fileInfo, ok := s.FS[path]
	assert.True(s.t, ok, "file %v does not exist", path)
	assert.False(s.t, fileInfo.isDir, "%v is not a file", path)
	assert.Equal(s.t, perms, fileInfo.perms, "%v has unexpected perms", path)
	assert.Equal(s.t, user, fileInfo.user, "%v has unexpected user", path)
	assert.Equal(s.t, group, fileInfo.group, "%v has unexpected group", path)
}

// AssertSymlinkExists asserts that a symlink exists on the host with the given target, user, and group.
func (s *State) AssertSymlinkExists(path string, target string, user string, group string) {
	fileInfo, ok := s.FS[path]
	assert.True(s.t, ok, "syminlk %v does not exist", path)
	assert.True(s.t, fileInfo.isSymlink, "%v is not a symlink", path)
	assert.Equal(s.t, target, fileInfo.link, "%v has unexpected target", path)
	assert.Equal(s.t, user, fileInfo.user, "%v has unexpected user", path)
	assert.Equal(s.t, group, fileInfo.group, "%v has unexpected group", path)
}

// AssertUnitsLoaded asserts that units are enabled on the host.
func (s *State) AssertUnitsLoaded(names ...string) {
	for _, name := range names {
		unit, ok := s.Units[name]
		assert.True(s.t, ok, "unit %v is not loaded", name)
		assert.Equal(s.t, Loaded, unit.LoadState, "unit %v is not loaded", name)
	}
}

// AssertUnitsEnabled asserts that a systemd unit is not loaded.
func (s *State) AssertUnitsEnabled(names ...string) {
	for _, name := range names {
		unit, ok := s.Units[name]
		assert.True(s.t, ok, "unit %v is not enabled", name)
		assert.Equal(s.t, "enabled", unit.Enabled, "unit %v is not enabled", name)
	}
}

// AssertUnitsRunning asserts that a systemd unit is running.
func (s *State) AssertUnitsRunning(names ...string) {
	for _, name := range names {
		unit, ok := s.Units[name]
		assert.True(s.t, ok, "unit %v is not running", name)
		assert.Equal(s.t, Running, unit.SubState, "unit %v is not running", name)
	}
}

// AssertUnitsNotLoaded asserts that a systemd unit is not loaded.
func (s *State) AssertUnitsNotLoaded(names ...string) {
	for _, name := range names {
		_, ok := s.Units[name]
		assert.True(s.t, !ok, "unit %v is loaded", name)
	}
}

// AssertUnitsNotEnabled asserts that a systemd unit is not enabled
func (s *State) AssertUnitsNotEnabled(names ...string) {
	for _, name := range names {
		unit, ok := s.Units[name]
		assert.True(s.t, ok, "unit %v is enabled", name)
		assert.Equal(s.t, "disabled", unit.Enabled, "unit %v is enabled", name)
	}
}

// AssertUnitsDead asserts that a systemd unit is not running.
func (s *State) AssertUnitsDead(names ...string) {
	for _, name := range names {
		unit, ok := s.Units[name]
		assert.True(s.t, ok, "unit %v is not running", name)
		assert.Equal(s.t, Dead, unit.SubState, "unit %v is not running", name)
	}
}
