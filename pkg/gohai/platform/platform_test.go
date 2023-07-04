// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package platform

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectPlatform(t *testing.T) {
	platformInfo := CollectInfo()

	errorGetters := map[string]func() error{
		"GoVersion":        utils.ValueErrorGetter(&platformInfo.GoVersion),
		"GoOS":             utils.ValueErrorGetter(&platformInfo.GoOS),
		"GoArch":           utils.ValueErrorGetter(&platformInfo.GoArch),
		"KernelName":       utils.ValueErrorGetter(&platformInfo.KernelName),
		"KernelRelease":    utils.ValueErrorGetter(&platformInfo.KernelRelease),
		"Hostname":         utils.ValueErrorGetter(&platformInfo.Hostname),
		"Machine":          utils.ValueErrorGetter(&platformInfo.Machine),
		"OS":               utils.ValueErrorGetter(&platformInfo.OS),
		"Family":           utils.ValueErrorGetter(&platformInfo.Family),
		"KernelVersion":    utils.ValueErrorGetter(&platformInfo.KernelVersion),
		"Processor":        utils.ValueErrorGetter(&platformInfo.Processor),
		"HardwarePlatform": utils.ValueErrorGetter(&platformInfo.HardwarePlatform),
	}

	for fieldname, errorGetter := range errorGetters {
		err := errorGetter()
		if err != nil {
			assert.ErrorIsf(t, err, utils.ErrNotCollectable, "platform: field %s could not be collected", fieldname)
		}
	}
}

func TestPlatformAsJSON(t *testing.T) {
	platformInfo := CollectInfo()
	marshallable, _, err := platformInfo.AsJSON()
	require.NoError(t, err)

	marshalled, err := json.Marshal(marshallable)
	require.NoError(t, err)

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

	decoder := json.NewDecoder(bytes.NewReader(marshalled))
	// do not ignore unknown fields
	decoder.DisallowUnknownFields()

	var decodedPlatform Platform
	err = decoder.Decode(&decodedPlatform)
	require.NoError(t, err)

	// check that we read the full json
	require.False(t, decoder.More())

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
