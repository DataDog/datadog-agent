// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// nvidiaCharMajor is the well-known static character-device major number
	// assigned to the NVIDIA driver (nvidiaN, nvidiactl, nvidia-modeset).
	nvidiaCharMajor = 195
	// nvidiaCtlMinor is the minor number of the /dev/nvidiactl control device.
	nvidiaCtlMinor = 255
	// deviceNodePerm is the permission bits NVIDIA device nodes should carry:
	// readable and writable by everyone, matching what the NVIDIA container
	// runtime injects.
	deviceNodePerm = 0666
	// deviceNodeMode is the mode passed to mknod: a character device with the
	// permission bits above. Note that mknod also applies the process umask, so
	// the permissions are re-asserted with an explicit chmod afterwards.
	deviceNodeMode = unix.S_IFCHR | deviceNodePerm
)

// devNode describes an NVIDIA character device node that should exist under /dev.
type devNode struct {
	name  string
	major uint32
	minor uint32
}

// recoverMissingDeviceNodes recreates NVIDIA character device nodes that are
// missing from the current mount namespace.
//
// This works around a known failure mode in containerized (Kubernetes)
// deployments: when only the agent container is recreated (e.g. after an
// OOMKill, liveness-probe failure or a SIGKILL of the agent process) without
// recreating the pod, the kubelet does not re-invoke the NVIDIA device plugin
// and the NVIDIA container runtime does not re-inject the per-GPU and UVM
// device nodes. The host driver stays loaded, so /proc/driver/nvidia is still
// present, but /dev/nvidia0 and /dev/nvidia-uvm are gone (ENOENT) and NVML
// fails to initialize with "Unknown Error". The agent containers are granted
// CAP_MKNOD by the Datadog operator specifically so the nodes can be recreated.
//
// It returns the number of device nodes it successfully (re)created. Creation
// is best-effort: individual failures are logged and skipped so that recovering
// one device does not prevent recovering the others.
func recoverMissingDeviceNodes() int {
	procRoot := kernel.ProcFSRoot()

	// Only attempt recovery if the NVIDIA driver is actually loaded on the
	// host. If /proc/driver/nvidia/gpus is absent there is no driver to talk
	// to and recreating device nodes would not help.
	gpusDir := filepath.Join(procRoot, "driver", "nvidia", "gpus")
	if info, err := os.Stat(gpusDir); err != nil || !info.IsDir() {
		return 0
	}

	nodes := expectedDeviceNodes(procRoot, gpusDir)
	if len(nodes) == 0 {
		return 0
	}

	created := 0
	for _, n := range nodes {
		// Device nodes must be created in this process' own mount namespace
		// (/dev), which is exactly the namespace NVML looks in.
		path := filepath.Join("/dev", n.name)
		if _, err := os.Stat(path); err == nil {
			// Node already present, nothing to do.
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			log.Warnf("could not stat device node %s: %v", path, err)
			continue
		}

		dev := unix.Mkdev(n.major, n.minor)
		if err := unix.Mknod(path, deviceNodeMode, int(dev)); err != nil {
			log.Warnf("failed to recreate missing NVIDIA device node %s (%d:%d): %v", path, n.major, n.minor, err)
			continue
		}

		// Mknod applies the process umask to the mode, which can strip the
		// group/other write bits we need. Force the permissions explicitly so
		// the node ends up world-accessible like the ones the NVIDIA container
		// runtime injects (NVML needs read+write to issue ioctls).
		if err := os.Chmod(path, deviceNodePerm); err != nil {
			log.Warnf("failed to set permissions on recreated NVIDIA device node %s: %v", path, err)
		}

		log.Infof("recreated missing NVIDIA device node %s (%d:%d)", path, n.major, n.minor)
		created++
	}

	return created
}

// expectedDeviceNodes builds the list of NVIDIA character device nodes that
// should exist given the currently loaded driver. GPU minors are read from
// /proc/driver/nvidia/gpus, and the (dynamically allocated) nvidia-uvm major is
// read from /proc/devices.
func expectedDeviceNodes(procRoot, gpusDir string) []devNode {
	nodes := []devNode{
		// The control device is always present when the driver is loaded.
		{name: "nvidiactl", major: nvidiaCharMajor, minor: nvidiaCtlMinor},
	}

	entries, err := os.ReadDir(gpusDir)
	if err != nil {
		log.Warnf("could not read %s to enumerate GPUs: %v", gpusDir, err)
		return nil
	}

	for _, entry := range entries {
		minor, err := readGPUMinor(filepath.Join(gpusDir, entry.Name(), "information"))
		if err != nil {
			log.Warnf("could not determine device minor for GPU %s: %v", entry.Name(), err)
			continue
		}
		nodes = append(nodes, devNode{name: "nvidia" + strconv.Itoa(minor), major: nvidiaCharMajor, minor: uint32(minor)})
	}

	// nvidia-uvm uses a dynamically allocated major number, so read it from
	// /proc/devices. If the UVM module is not loaded it simply won't be listed.
	if uvmMajor, ok := readCharDeviceMajor(filepath.Join(procRoot, "devices"), "nvidia-uvm"); ok {
		nodes = append(nodes,
			devNode{name: "nvidia-uvm", major: uvmMajor, minor: 0},
			devNode{name: "nvidia-uvm-tools", major: uvmMajor, minor: 1},
		)
	}

	return nodes
}

// readGPUMinor parses the "Device Minor" field from a
// /proc/driver/nvidia/gpus/<pci>/information file.
func readGPUMinor(informationPath string) (int, error) {
	f, err := os.Open(informationPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "Device Minor:") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "Device Minor:"))
		minor, err := strconv.Atoi(value)
		if err != nil {
			return 0, err
		}
		return minor, nil
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}

	return 0, errors.New("no \"Device Minor\" field found")
}

// readCharDeviceMajor scans the "Character devices" section of /proc/devices
// looking for a device with the given name, returning its major number.
func readCharDeviceMajor(devicesPath, name string) (uint32, bool) {
	f, err := os.Open(devicesPath)
	if err != nil {
		log.Warnf("could not read %s: %v", devicesPath, err)
		return 0, false
	}
	defer f.Close()

	inCharSection := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case line == "Character devices:":
			inCharSection = true
			continue
		case line == "Block devices:":
			// Reached the block-device section, stop searching.
			return 0, false
		case !inCharSection || line == "":
			continue
		}

		// Lines look like "234 nvidia-uvm".
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[1] != name {
			continue
		}
		major, err := strconv.ParseUint(fields[0], 10, 32)
		if err != nil {
			continue
		}
		return uint32(major), true
	}

	return 0, false
}

// isNvmlInitError reports whether err is an NVML API error raised by the Init
// call, i.e. the library loaded but could not initialize. This is the failure
// mode caused by missing device nodes; a plain "library not found" error is
// not recoverable by recreating device nodes.
func isNvmlInitError(err error) bool {
	// The device nodes being gone surfaces as a generic Init error rather than
	// ERROR_DRIVER_NOT_LOADED (the driver is loaded, just unreachable), so we
	// match on the API name and do not narrow further by error code.
	var nvmlErr *NvmlAPIError
	return errors.As(err, &nvmlErr) && nvmlErr.APIName == "Init"
}
