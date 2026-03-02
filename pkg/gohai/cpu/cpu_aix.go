// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build aix

package cpu

import "github.com/DataDog/datadog-agent/pkg/gohai/utils"

func getCPUInfo() *Info {
	return &Info{
		VendorID:             utils.NewErrorValue[string](utils.ErrNotCollectable),
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
}
