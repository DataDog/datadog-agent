// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hosttags

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/gohai/cpu"
	"github.com/DataDog/datadog-agent/pkg/gohai/memory"
	"github.com/DataDog/datadog-agent/pkg/gohai/platform"
	"github.com/DataDog/datadog-agent/pkg/inventory/systeminfo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const infrastructureModeEndUserDevice = "end_user_device"

// collectEUDMTagsFunc is overridable in tests.
var collectEUDMTagsFunc = collectEUDMHardwareTags

// getEUDMTags returns host tags describing the device when running in
// end_user_device infrastructure mode. Hardware/OS tags are only collected on
// macOS and Windows to mirror the hostsysteminfo metadata gate.
func getEUDMTags() []string {
	tags := []string{"infra_mode:" + infrastructureModeEndUserDevice}
	return append(tags, collectEUDMTagsFunc()...)
}

func collectEUDMHardwareTags() []string {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		return nil
	}

	tags := []string{"os_name:" + runtime.GOOS}

	platformInfo := platform.CollectInfo()
	if v, err := platformInfo.KernelRelease.Value(); err == nil && v != "" {
		tags = append(tags, "os_version:"+sanitizeEUDMTagValue(v))
	} else if err != nil {
		log.Debugf("EUDM host tags: os_version unavailable: %v", err)
	}

	cpuInfo := cpu.CollectInfo()
	if v, err := cpuInfo.ModelName.Value(); err == nil && v != "" {
		tags = append(tags, "cpu_model:"+sanitizeEUDMTagValue(v))
	} else if err != nil {
		log.Debugf("EUDM host tags: cpu_model unavailable: %v", err)
	}

	memInfo := memory.CollectInfo()
	if v, err := memInfo.TotalBytes.Value(); err == nil && v > 0 {
		tags = append(tags, fmt.Sprintf("total_memory_gb:%d", bytesToGB(v)))
	} else if err != nil {
		log.Debugf("EUDM host tags: total_memory_gb unavailable: %v", err)
	}

	sysInfo, err := systeminfo.Collect()
	if err != nil {
		log.Debugf("EUDM host tags: device_model unavailable: %v", err)
	} else if sysInfo != nil && sysInfo.Identifier != "" {
		tags = append(tags, "device_model:"+sanitizeEUDMTagValue(sysInfo.Identifier))
	}

	return tags
}

// bytesToGB converts bytes to gibibytes, rounding to the nearest GB.
func bytesToGB(b uint64) uint64 {
	const gib = 1024 * 1024 * 1024
	return (b + gib/2) / gib
}

// eudmTagValueReplacer collapses whitespace in tag values to underscores so a
// single token-style tag is produced.
var eudmTagValueReplacer = strings.NewReplacer(" ", "_", "\t", "_", "\n", "_", "\r", "_")

// sanitizeEUDMTagValue normalises a free-form value for use as a tag value.
func sanitizeEUDMTagValue(v string) string {
	return eudmTagValueReplacer.Replace(strings.TrimSpace(v))
}
