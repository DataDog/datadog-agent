// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gpu

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// driverGPUsDir is where the NVIDIA kernel driver exposes one directory per
// GPU, named after its PCI bus location, relative to the procfs root.
const driverGPUsDir = "driver/nvidia/gpus"

// DriverGPU is a GPU as reported by the NVIDIA kernel driver in procfs.
//
// The driver publishes these files on every node where it is loaded, so they
// identify GPUs without NVML. Device plugins that allocate GPUs by index, such
// as the GKE device plugin (nvidia0, nvidia0/vgpu3), refer to the device minor.
type DriverGPU struct {
	// Minor is the device minor number, N in /dev/nvidiaN.
	Minor int
	// UUID is the GPU UUID as reported by the driver, for example GPU-<uuid>.
	UUID string
	// Model is the product name, for example "NVIDIA L4".
	Model string
	// BusLocation is the PCI bus location, for example 0000:00:03.0.
	BusLocation string
}

// ReadDriverGPUs returns the GPUs listed by the NVIDIA kernel driver under
// <procRoot>/driver/nvidia/gpus, sorted by device minor. It returns no GPUs and
// no error when the driver is not loaded. GPUs whose information file cannot be
// parsed are skipped and reported in the returned error.
func ReadDriverGPUs(procRoot string) ([]DriverGPU, error) {
	entries, err := os.ReadDir(filepath.Join(procRoot, driverGPUsDir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var gpus []DriverGPU
	var errs []error
	for _, entry := range entries {
		path := filepath.Join(procRoot, driverGPUsDir, entry.Name(), "information")
		gpu, err := readDriverGPUInformation(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
			continue
		}
		gpus = append(gpus, gpu)
	}
	sort.Slice(gpus, func(i, j int) bool { return gpus[i].Minor < gpus[j].Minor })
	return gpus, errors.Join(errs...)
}

func readDriverGPUInformation(path string) (DriverGPU, error) {
	f, err := os.Open(path)
	if err != nil {
		return DriverGPU{}, err
	}
	defer f.Close()
	return parseDriverGPUInformation(f)
}

// parseDriverGPUInformation parses an information file, which holds one
// "Key: value" pair per line, for example:
//
//	Model:           NVIDIA L4
//	GPU UUID:        GPU-e80c8f33-55f5-55d2-18b7-64563f748393
//	Bus Location:    0000:00:03.0
//	Device Minor:    0
func parseDriverGPUInformation(r io.Reader) (DriverGPU, error) {
	gpu := DriverGPU{Minor: -1}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "Model":
			gpu.Model = value
		case "GPU UUID":
			gpu.UUID = value
		case "Bus Location":
			gpu.BusLocation = value
		case "Device Minor":
			minor, err := strconv.Atoi(value)
			if err != nil {
				return DriverGPU{}, fmt.Errorf("invalid device minor %q: %w", value, err)
			}
			gpu.Minor = minor
		}
	}
	if err := scanner.Err(); err != nil {
		return DriverGPU{}, err
	}
	if gpu.UUID == "" || gpu.Minor < 0 {
		return DriverGPU{}, errors.New("missing GPU UUID or device minor")
	}
	return gpu, nil
}
