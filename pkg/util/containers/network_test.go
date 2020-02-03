// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-2020 Datadog, Inc.

package containers

import (
	"io/ioutil"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultGateway(t *testing.T) {
	testCases := []struct {
		netRouteContent []byte
		expectedIP      string
	}{
		{
			[]byte(`Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
ens33	00000000	0280A8C0	0003	0	0	100	00000000	0	0	0
`),
			"192.168.128.2",
		},
		{
			[]byte(`Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
ens33	00000000	FE01A8C0	0003	0	0	100	00000000	0	0	0
`),
			"192.168.1.254",
		},
		{
			[]byte(`Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
ens33	00000000	FEFEA8C0	0003	0	0	100	00000000	0	0	0
`),
			"192.168.254.254",
		},
	}
	for _, testCase := range testCases {
		t.Run("", func(t *testing.T) {
			testProc, err := testutil.NewTempFolder("test-default-gateway")
			require.NoError(t, err)
			defer testProc.RemoveAll()
			err = os.MkdirAll(path.Join(testProc.RootPath, "net"), os.ModePerm)
			require.NoError(t, err)

			err = ioutil.WriteFile(path.Join(testProc.RootPath, "net", "route"), testCase.netRouteContent, os.ModePerm)
			require.NoError(t, err)
			config.Datadog.SetDefault("proc_root", testProc.RootPath)
			ip, err := DefaultGateway()
			require.NoError(t, err)
			assert.Equal(t, testCase.expectedIP, ip.String())

			testProc.RemoveAll()
			ip, err = DefaultGateway()
			require.NoError(t, err)
			require.Nil(t, ip)
		})
	}
}

func TestDefaulHostIPs(t *testing.T) {
	dummyProcDir, err := testutil.NewTempFolder("test-default-host-ips")
	require.Nil(t, err)
	defer dummyProcDir.RemoveAll()
	config.Datadog.SetDefault("proc_root", dummyProcDir.RootPath)

	t.Run("routing table contains a gateway entry", func(t *testing.T) {
		routes := `
		    Iface    Destination Gateway  Flags RefCnt Use Metric Mask     MTU Window IRTT
		    default  00000000    010011AC 0003  0      0   0      00000000 0   0      0
		    default  000011AC    00000000 0001  0      0   0      0000FFFF 0   0      0
		    eth1     000012AC    00000000 0001  0      0   0      0000FFFF 0   0      0 `

		// Pick an existing device and replace the "default" placeholder by its name
		interfaces, err := net.Interfaces()
		require.NoError(t, err)
		require.NotEmpty(t, interfaces)
		netInterface := interfaces[0]
		routes = strings.ReplaceAll(routes, "default", netInterface.Name)

		// Populate routing table file
		err = dummyProcDir.Add(filepath.Join("net", "route"), routes)
		require.NoError(t, err)

		// Retrieve IPs bound to the "default" network interface
		var expectedIPs []string
		netAddrs, err := netInterface.Addrs()
		require.NoError(t, err)
		require.NotEmpty(t, netAddrs)
		for _, address := range netAddrs {
			ip := strings.Split(address.String(), "/")[0]
			require.NotNil(t, net.ParseIP(ip))
			expectedIPs = append(expectedIPs, ip)
		}

		// Verify they match the IPs returned by DefaultHostIPs()
		defaultIPs, err := DefaultHostIPs()
		assert.Nil(t, err)
		assert.Equal(t, expectedIPs, defaultIPs)
	})

	t.Run("routing table missing a gateway entry", func(t *testing.T) {
		routes := `
	        Iface    Destination Gateway  Flags RefCnt Use Metric Mask     MTU Window IRTT
	        eth0     000011AC    00000000 0001  0      0   0      0000FFFF 0   0      0
	        eth1     000012AC    00000000 0001  0      0   0      0000FFFF 0   0      0 `

		err = dummyProcDir.Add(filepath.Join("net", "route"), routes)
		require.NoError(t, err)
		ips, err := DefaultHostIPs()
		assert.Nil(t, ips)
		assert.NotNil(t, err)
	})
}
