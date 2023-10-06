// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package platform

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	"github.com/stretchr/testify/assert"
)

func TestCollectPlatform(t *testing.T) {
	platformInfo := CollectInfo()

	errorGetters := map[string]error{
		"GoVersion":        platformInfo.GoVersion.Error(),
		"GoOS":             platformInfo.GoOS.Error(),
		"GoArch":           platformInfo.GoArch.Error(),
		"KernelName":       platformInfo.KernelName.Error(),
		"KernelRelease":    platformInfo.KernelRelease.Error(),
		"Hostname":         platformInfo.Hostname.Error(),
		"Machine":          platformInfo.Machine.Error(),
		"OS":               platformInfo.OS.Error(),
		"Family":           platformInfo.Family.Error(),
		"KernelVersion":    platformInfo.KernelVersion.Error(),
		"Processor":        platformInfo.Processor.Error(),
		"HardwarePlatform": platformInfo.HardwarePlatform.Error(),
	}

	for fieldname, err := range errorGetters {
		if err != nil {
			assert.ErrorIsf(t, err, utils.ErrNotCollectable, "platform: field %s could not be collected", fieldname)
		}
	}
}

func TestPlatformAsJSON(t *testing.T) {
	platformInfo := CollectInfo()

	// Any change to this datastructure should be notified to the backend
	// team to ensure compatibility.
	type Platform struct {
		GoArch           string `json:"GOOARCH"`
		GoOS             string `json:"GOOS"`
		GoVersion        string `json:"goV"`
		Hostname         string `json:"hostname"`
		KernelName       string `json:"kernel_name"`
		KernelRelease    string `json:"kernel_release"`
		KernelVersion    string `json:"kernel_version"`
		Machine          string `json:"machine"`
		OS               string `json:"os"`
		Processor        string `json:"processor"`
		Family           string `json:"family"`
		HardwarePlatform string `json:"hardware_platform"`
	}

	var decodedPlatform Platform
	utils.RequireMarshallJSON(t, platformInfo, &decodedPlatform)

	utils.AssertDecodedValue(t, decodedPlatform.GoVersion, &platformInfo.GoVersion, "")
	utils.AssertDecodedValue(t, decodedPlatform.GoOS, &platformInfo.GoOS, "")
	utils.AssertDecodedValue(t, decodedPlatform.GoArch, &platformInfo.GoArch, "")
	utils.AssertDecodedValue(t, decodedPlatform.KernelName, &platformInfo.KernelName, "")
	utils.AssertDecodedValue(t, decodedPlatform.KernelRelease, &platformInfo.KernelRelease, "")
	utils.AssertDecodedValue(t, decodedPlatform.Hostname, &platformInfo.Hostname, "")
	utils.AssertDecodedValue(t, decodedPlatform.Machine, &platformInfo.Machine, "")
	utils.AssertDecodedValue(t, decodedPlatform.OS, &platformInfo.OS, "")
	utils.AssertDecodedValue(t, decodedPlatform.Family, &platformInfo.Family, "")
	utils.AssertDecodedValue(t, decodedPlatform.KernelVersion, &platformInfo.KernelVersion, "")
	utils.AssertDecodedValue(t, decodedPlatform.Processor, &platformInfo.Processor, "")
	utils.AssertDecodedValue(t, decodedPlatform.HardwarePlatform, &platformInfo.HardwarePlatform, "")
}
