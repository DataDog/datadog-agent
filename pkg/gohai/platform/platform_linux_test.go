// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package platform

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestIsVendorAMD(t *testing.T) {
	athlonSource := `processor   : 0
vendor_id   : AuthenticAMD
cpu family  : 15
model       : 67
model name  : Dual-Core AMD Opteron(tm) Processor 1218 HE`
	reader := strings.NewReader(athlonSource)
	require.True(t, isVendorAMD(reader))

	notAthlonSource := `processor	: 0
vendor_id	: GenuineIntel
cpu family	: 6
model		: 79
model name	: Intel(R) Xeon(R) CPU E5-2686 v4 @ 2.30GHz`
	reader = strings.NewReader(notAthlonSource)
	require.False(t, isVendorAMD(reader))
}

func TestUpdateUnameInfo(t *testing.T) {
	uname := &unix.Utsname{}
	sysname := "A"
	copy(uname.Sysname[:], []byte(sysname))
	nodename := "B"
	copy(uname.Nodename[:], []byte(nodename))
	release := "C"
	copy(uname.Release[:], []byte(release))
	version := "D"
	copy(uname.Version[:], []byte(version))
	machine := "E"
	copy(uname.Machine[:], []byte(machine))

	expected := Info{
		KernelName:       utils.NewValue(sysname),
		Hostname:         utils.NewValue(nodename),
		KernelRelease:    utils.NewValue(release),
		Machine:          utils.NewValue(machine),
		Processor:        utils.NewValue(getProcessorType(machine)),
		HardwarePlatform: utils.NewValue(getHardwarePlatform(machine)),
		KernelVersion:    utils.NewValue(version),
	}

	archInfo := Info{}
	updateUnameInfo(&archInfo, uname)

	require.Equal(t, expected, archInfo)
}
