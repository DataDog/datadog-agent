package so

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func procBashReadPidMapsPathname(t *testing.T, pid int, cmdlineExtra ...string) []string {
	cmdline := fmt.Sprintf("awk '{print $6}' /proc/%d/maps | sort -u | grep -v '^\\['", pid)
	if len(cmdlineExtra) > 0 {
		cmdline = cmdline + cmdlineExtra[0]
	}
	cmd := exec.Command("bash", "-c", cmdline)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("exec Command failed '%s' %v", cmd.String(), err)
	}

	libs := []string{}
	scanner := bufio.NewScanner(bytes.NewBuffer(out))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 0 {
			libs = append(libs, scanner.Text())
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal("reading output Command error", err)
	}
	return libs
}

func TestProcSelfNoFilter(t *testing.T) {
	pid := os.Getpid()
	pidStr := strconv.Itoa(pid)
	libs := Find("/proc/"+pidStr, nil)

	libPathname := []string{}
	for _, lib := range libs {
		libPathname = append(libPathname, lib.Pathname)
	}

	expected := procBashReadPidMapsPathname(t, pid)
	assert.ElementsMatch(t, expected, libPathname)
}

func TestProcSelfFilterAllLibraries(t *testing.T) {
	pid := os.Getpid()
	pidStr := strconv.Itoa(pid)
	libs := Find("/proc/"+pidStr, AllLibraries)

	libPathname := []string{}
	for _, lib := range libs {
		libPathname = append(libPathname, lib.Pathname)
	}

	expected := procBashReadPidMapsPathname(t, pid, fmt.Sprintf(" | egrep '%s'", AllLibraries.String()))

	assert.ElementsMatch(t, expected, libPathname)
}

func TestProcSelfvsAllPidsNoFilter(t *testing.T) {
	pid := os.Getpid()
	pidStr := strconv.Itoa(pid)

	libs := Find("/proc", nil)

	libPathname := []string{}
	for _, lib := range libs {
		for _, p := range lib.PidsPath {
			if p == "/proc/"+pidStr {
				libPathname = append(libPathname, lib.Pathname)
			}
		}
	}

	expected := procBashReadPidMapsPathname(t, pid)

	assert.ElementsMatch(t, expected, libPathname)
}

func TestProcSelfvsAllPidsAllLibrariesFilter(t *testing.T) {
	pid := os.Getpid()
	pidStr := strconv.Itoa(pid)

	libs := Find("/proc", AllLibraries)

	libPathname := []string{}
	for _, lib := range libs {
		for _, p := range lib.PidsPath {
			if p == "/proc/"+pidStr {
				libPathname = append(libPathname, lib.Pathname)
			}
		}
	}

	expected := procBashReadPidMapsPathname(t, pid, fmt.Sprintf(" | egrep '%s'", AllLibraries.String()))

	assert.ElementsMatch(t, expected, libPathname)
}
