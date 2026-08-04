// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// cgroupKillFile is the cgroup v2 interface file that kills a whole cgroup at once.
// Available since Linux 5.14, on non-root and non-threaded cgroups only.
const cgroupKillFile = "cgroup.kill"

// errCgroupKillUnavailable is returned when the host cannot kill cgroups in one shot, in which
// case the caller falls back to signalling each process individually.
var errCgroupKillUnavailable = errors.New("one-shot cgroup kill is unavailable")

// cgroupKiller kills every process of a cgroup in a single operation, by writing to the cgroup
// v2 `cgroup.kill` interface. Compared to signalling each PID in turn, the kernel deals with
// concurrent forks and migrations itself, so a forking process cannot outrun the kill, and there
// is no PID reuse window to guard against.
//
// Note that cgroup.kill also kills the processes of every descendant cgroup, and only ever
// delivers SIGKILL.
type cgroupKiller struct {
	// bases are the directories a cgroup ID is resolved against, tried in order. There is more
	// than one because the agent's own view of cgroupfs is not necessarily writable.
	bases []string
	// selfCGroupID is the agent's own cgroup, used to refuse killing ourselves.
	selfCGroupID containerutils.CGroupID
}

// newCgroupKiller returns a cgroupKiller, or an error if the host does not support killing
// cgroups in one shot.
func newCgroupKiller() (*cgroupKiller, error) {
	// Restrict this to pure cgroup v2 hosts: on a hybrid hierarchy the cgroup IDs CWS resolves
	// are relative to the v1 mount point, so they must not be joined onto the v2 one.
	if !utils.IsPureCGroupV2Available() {
		return nil, fmt.Errorf("%w: not a pure cgroup v2 hierarchy", errCgroupKillUnavailable)
	}

	agentMountPoint, err := utils.GetCgroup2MountPoint()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errCgroupKillUnavailable, err)
	}
	if agentMountPoint == "" {
		return nil, fmt.Errorf("%w: cgroup2 is not mounted", errCgroupKillUnavailable)
	}

	// Best effort: without it we simply have one less path to try.
	hostMountPoint, err := utils.GetHostCgroup2MountPoint()
	if err != nil {
		seclog.Debugf("unable to resolve the host cgroup2 mount point: %s", err)
	}

	// Best effort: without it we rely on the per-process exclusion list to protect the agent.
	self, err := selfCGroupID()
	if err != nil {
		seclog.Warnf("unable to resolve the agent's own cgroup, cgroup kill won't be able to detect self-destruction: %s", err)
	}

	return &cgroupKiller{
		bases:        cgroupKillWriteBases(agentMountPoint, hostMountPoint, kernel.ProcFSRoot()),
		selfCGroupID: self,
	}, nil
}

// cgroupKillWriteBases returns the directories to resolve a cgroup ID against, in order of
// preference: the agent's own view of the cgroup2 mount point first, then the same mount point
// reached through pid 1's root. The latter matters because the Kubernetes manifests bind-mount
// cgroupfs read-only into the agent's containers, which makes opening cgroup.kill for writing
// fail with EROFS, while the host mount it points to stays writable.
func cgroupKillWriteBases(agentMountPoint, hostMountPoint, procRoot string) []string {
	bases := []string{agentMountPoint}
	if hostMountPoint != "" {
		bases = append(bases, filepath.Join(procRoot, "1", "root", hostMountPoint))
	}
	return bases
}

// selfCGroupID returns the cgroup the agent itself belongs to.
func selfCGroupID() (containerutils.CGroupID, error) {
	pid := utils.Getpid()
	_, cgroupContext, _, err := utils.DefaultCGroupFS().FindCGroupContext(pid, pid)
	if err != nil {
		return "", err
	}
	if cgroupContext.CGroupID == "" {
		return "", errors.New("empty cgroup ID")
	}
	return cgroupContext.CGroupID, nil
}

// kill sends SIGKILL to every process of the target cgroup, and of its descendant cgroups, in a
// single operation. An error means nothing was killed, so the caller can safely fall back to
// killing each process individually.
func (c *cgroupKiller) kill(target cgroupKillTarget) error {
	relPath, err := cgroupRelPath(target.id)
	if err != nil {
		return err
	}

	if c.isSelfOrAncestor(relPath) {
		return fmt.Errorf("refusing to kill cgroup `%s`, it holds the agent itself", target.id)
	}

	var errs []error
	for _, base := range c.bases {
		dir := filepath.Join(base, relPath)

		// Make sure the path still points at the cgroup CWS resolved. A stale or mismatching
		// path would kill an unrelated workload, so skip this view rather than write to it.
		// It also rules out the reverse mistake of resolving the ID against a namespaced
		// cgroupfs that happens to hold a cgroup with the same path.
		if err := checkCgroupInode(dir, target.inode); err != nil {
			errs = append(errs, err)
			continue
		}

		// Opening cgroup.kill has no effect of its own, the kill happens on write, so a failure
		// here is safe to retry against the next base.
		file, err := os.OpenFile(filepath.Join(dir, cgroupKillFile), os.O_WRONLY, 0)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		_, err = file.WriteString("1")
		file.Close()
		if err != nil {
			// The write is the point of no return: a cgroup that is threaded (EOPNOTSUPP) or
			// already gone (ENODEV) killed nothing, so let the caller fall back.
			return fmt.Errorf("failed to kill cgroup `%s`: %w", target.id, err)
		}
		return nil
	}

	return fmt.Errorf("failed to kill cgroup `%s`: %w", target.id, errors.Join(errs...))
}

// isSelfOrAncestor returns whether the given cgroup holds the agent itself. Since cgroup.kill
// also kills descendant cgroups, an ancestor of the agent's cgroup is just as fatal as its own.
func (c *cgroupKiller) isSelfOrAncestor(relPath string) bool {
	if c.selfCGroupID == "" {
		return false
	}
	self := path.Clean("/" + string(c.selfCGroupID))
	return self == relPath || strings.HasPrefix(self, relPath+"/")
}

// cgroupRelPath validates a cgroup ID and returns it as a cleaned, rooted relative path.
func cgroupRelPath(cgroupID containerutils.CGroupID) (string, error) {
	if cgroupID == "" {
		return "", errors.New("unable to kill cgroup: empty cgroup ID")
	}
	if slices.Contains(strings.Split(string(cgroupID), "/"), "..") {
		return "", fmt.Errorf("unable to kill cgroup: `%s` is not a valid cgroup ID", cgroupID)
	}
	relPath := path.Clean("/" + string(cgroupID))
	if relPath == "/" {
		// The root cgroup has no cgroup.kill file, and killing it would take down the host.
		return "", errors.New("refusing to kill the root cgroup")
	}
	return relPath, nil
}

// checkCgroupInode returns an error unless dir is a directory with the expected inode.
func checkCgroupInode(dir string, expectedInode uint64) error {
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("unable to stat cgroup `%s`", dir)
	}
	if stat.Ino != expectedInode {
		return fmt.Errorf("cgroup `%s` has inode %d, expected %d", dir, stat.Ino, expectedInode)
	}
	return nil
}
