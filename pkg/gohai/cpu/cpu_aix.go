// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build aix

package cpu

import (
	"os/exec"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
)

func getCPUInfo() *Info {
	info := &Info{
		VendorID:             utils.NewValue("IBM"),
		ModelName:            utils.NewErrorValue[string](utils.ErrNotCollectable),
		CPUCores:             utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		CPULogicalProcessors: utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		Mhz:                  utils.NewErrorValue[float64](utils.ErrNotCollectable),
		CacheSizeKB:          utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		Family:               utils.NewErrorValue[string](utils.ErrNotCollectable),
		Model:                utils.NewErrorValue[string](utils.ErrNotCollectable),
		Stepping:             utils.NewErrorValue[string](utils.ErrNotCollectable),
		CPUPkgs:              utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		CPUNumaNodes:         utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		CacheSizeL1Bytes:     utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		CacheSizeL2Bytes:     utils.NewErrorValue[uint64](utils.ErrNotCollectable),
		CacheSizeL3Bytes:     utils.NewErrorValue[uint64](utils.ErrNotCollectable),
	}

	out, err := exec.Command("prtconf").Output()
	if err != nil {
		return info
	}

	for _, line := range strings.Split(string(out), "\n") {
		key, val, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)

		switch key {
		case "Processor Type":
			info.ModelName = utils.NewValue(val)
		case "Number Of Processors":
			if n, parseErr := strconv.ParseUint(val, 10, 64); parseErr == nil {
				info.CPUCores = utils.NewValue(n)
				// Default logical to physical; overridden below if bindprocessor succeeds.
				info.CPULogicalProcessors = utils.NewValue(n)
			}
		case "Processor Clock Speed":
			// "2000 MHz" - grab just the numeric part
			fields := strings.Fields(val)
			if len(fields) > 0 {
				if mhz, parseErr := strconv.ParseFloat(fields[0], 64); parseErr == nil {
					info.Mhz = utils.NewValue(mhz)
				}
			}
		}
	}

	// Use bindprocessor -q to count logical CPUs (includes SMT threads).
	// Output: "The available processors are:  0 1 2 3 ... N"
	if bpOut, bpErr := exec.Command("bindprocessor", "-q").Output(); bpErr == nil {
		fields := strings.Fields(string(bpOut))
		// Skip the 4-word prefix "The available processors are:"
		const prefixWords = 4
		if len(fields) > prefixWords {
			info.CPULogicalProcessors = utils.NewValue(uint64(len(fields) - prefixWords))
		}
	}

	return info
}
