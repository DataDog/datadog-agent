package docker

// Copied from https://github.com/DataDog/datadog-process-agent/blob/e44b0de7b9eb7f05c96a2dbf91e763e931bf6174/util/docker/cgroup.go
// FIXME: merge back these utils

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
)

var (
	containerRe = regexp.MustCompile("[0-9a-f]{64}")
)

// ContainerIDForPID is a lighter version of CgroupsForPids to only retrieve the
// container ID for origin detection. Returns container id as a string, empty if
// the PID is not in a container.
func ContainerIDForPID(pid int) (string, error) {
	cgPath := filepath.Join(config.Datadog.GetString("proc_root"), strconv.Itoa(pid), "cgroup")
	containerID, _, err := readCgroupPaths(cgPath)
	if err != nil {
		return "", err
	}
	return containerID, nil
}

// readCgroupPaths reads the cgroups from a /sys/$pid/cgroup path.
func readCgroupPaths(pidCgroupPath string) (string, map[string]string, error) {
	f, err := os.Open(pidCgroupPath)
	if os.IsNotExist(err) {
		return "", nil, err
	} else if err != nil {
		return "", nil, err
	}
	defer f.Close()
	return parseCgroupPaths(f)
}

// parseCgroupPaths parses out the cgroup paths from a /proc/$pid/cgroup file.
// The file format will be something like:
//
// 11:net_cls:/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/47fc31db38b4fa0f4db44b99d0cad10e3cd4d5f142135a7721c1c95c1aadfb2e
// 10:freezer:/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/47fc31db38b4fa0f4db44b99d0cad10e3cd4d5f142135a7721c1c95c1aadfb2e
// 9:cpu,cpuacct:/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/47fc31db38b4fa0f4db44b99d0cad10e3cd4d5f142135a7721c1c95c1aadfb2e
// 8:memory:/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/47fc31db38b4fa0f4db44b99d0cad10e3cd4d5f142135a7721c1c95c1aadfb2e
// 7:blkio:/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/47fc31db38b4fa0f4db44b99d0cad10e3cd4d5f142135a7721c1c95c1aadfb2e
//
// Returns the common containerID and a mapping of target => path
// If the first line doesn't have a valid container ID we will return an empty string
func parseCgroupPaths(r io.Reader) (string, map[string]string, error) {
	var ok bool
	var containerID string
	paths := make(map[string]string)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		l := scanner.Text()
		if containerID == "" {
			// Check if this process running inside a container.
			containerID, ok = containerIDFromCgroup(l)
			if !ok {
				return "", nil, nil
			}
		}

		sp := strings.SplitN(l, ":", 3)
		if len(sp) < 3 {
			continue
		}
		// Target can be comma-separate values like cpu,cpuacct
		tsp := strings.Split(sp[1], ",")
		for _, target := range tsp {
			paths[target] = sp[2]
		}
	}
	if err := scanner.Err(); err != nil {
		return "", nil, err
	}

	// In Ubuntu Xenial, we've encountered containers with no `cpu`
	_, cpuok := paths["cpu"]
	cpuacct, cpuacctok := paths["cpuacct"]
	if !cpuok && cpuacctok {
		paths["cpu"] = cpuacct
	}

	return containerID, paths, nil
}

func containerIDFromCgroup(cgroup string) (string, bool) {
	sp := strings.SplitN(cgroup, ":", 3)
	if len(sp) < 3 {
		return "", false
	}
	match := containerRe.Find([]byte(sp[2]))
	if match == nil {
		return "", false
	}
	return string(match), true
}
