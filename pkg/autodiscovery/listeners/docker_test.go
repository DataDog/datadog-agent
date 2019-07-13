// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package listeners

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

func TestMain(m *testing.M) {
	// Temporary measure until we rework the listener
	// to use common docker methods and move that testing
	// to integration tests
	docker.EnableTestingMode()
	os.Exit(m.Run())
}

func TestGetConfigIDFromPs(t *testing.T) {
	co := types.Container{
		ID:    "deadbeef",
		Image: "test",
	}
	dl := DockerListener{}

	ids := dl.getConfigIDFromPs(co)
	assert.Equal(t, []string{"docker://deadbeef", "test"}, ids)

	prefixCo := types.Container{
		ID:    "deadbeef",
		Image: "org/test",
	}
	ids = dl.getConfigIDFromPs(prefixCo)
	assert.Equal(t, []string{"docker://deadbeef", "org/test", "test"}, ids)

	labeledCo := types.Container{
		ID:     "deadbeef",
		Image:  "test",
		Labels: map[string]string{"com.datadoghq.ad.check.id": "w00tw00t"},
	}
	ids = dl.getConfigIDFromPs(labeledCo)
	assert.Equal(t, []string{"w00tw00t"}, ids)

	legacyCo := types.Container{
		ID:     "deadbeef",
		Image:  "test",
		Labels: map[string]string{"com.datadoghq.sd.check.id": "w00tw00t"},
	}
	ids = dl.getConfigIDFromPs(legacyCo)
	assert.Equal(t, []string{"w00tw00t"}, ids)

	doubleCo := types.Container{
		ID:    "deadbeef",
		Image: "test",
		Labels: map[string]string{
			// Both labels, new one takes over
			"com.datadoghq.ad.check.id": "new",
			"com.datadoghq.sd.check.id": "old",
		},
	}
	ids = dl.getConfigIDFromPs(doubleCo)
	assert.Equal(t, []string{"new"}, ids)

	templatedCo := types.Container{
		ID:     "deadbeef",
		Image:  "org/test",
		Labels: map[string]string{"com.datadoghq.ad.instances": "[]]"},
	}
	ids = dl.getConfigIDFromPs(templatedCo)
	assert.Equal(t, []string{"docker://deadbeef"}, ids)
}

func TestGetHostsFromPs(t *testing.T) {
	dl := DockerListener{}

	co := types.Container{
		ID:    "foo",
		Image: "test",
	}

	assert.Empty(t, dl.getHostsFromPs(co))

	nets := make(map[string]*network.EndpointSettings)
	nets["bridge"] = &network.EndpointSettings{IPAddress: "172.17.0.2"}
	nets["foo"] = &network.EndpointSettings{IPAddress: "172.17.0.3"}
	networkSettings := types.SummaryNetworkSettings{
		Networks: nets}

	co = types.Container{
		ID:              "deadbeef",
		Image:           "test",
		NetworkSettings: &networkSettings,
		Ports:           []types.Port{{PrivatePort: 1337}, {PrivatePort: 42}},
	}
	hosts := dl.getHostsFromPs(co)

	assert.Equal(t, "172.17.0.2", hosts["bridge"])
	assert.Equal(t, "172.17.0.3", hosts["foo"])
	assert.Equal(t, 2, len(hosts))
}

func TestGetRancherIPFromPs(t *testing.T) {
	dl := DockerListener{}

	co := types.Container{
		ID:    "foo",
		Image: "test",
	}

	assert.Empty(t, dl.getHostsFromPs(co))

	nets := make(map[string]*network.EndpointSettings)
	nets["none"] = &network.EndpointSettings{}
	networkSettings := types.SummaryNetworkSettings{
		Networks: nets}

	co = types.Container{
		ID:              "deadbeef",
		Image:           "test",
		NetworkSettings: &networkSettings,
		Ports:           []types.Port{{PrivatePort: 1337}, {PrivatePort: 42}},
		Labels: map[string]string{
			"io.rancher.container.ip": "10.42.90.224/16",
		},
	}
	hosts := dl.getHostsFromPs(co)

	assert.Equal(t, "10.42.90.224", hosts["rancher"])
	assert.Equal(t, 1, len(hosts))
}

func TestGetPortsFromPs(t *testing.T) {
	dl := DockerListener{}

	co := types.Container{
		ID:    "foo",
		Image: "test",
	}
	assert.Empty(t, dl.getPortsFromPs(co))
	assert.Nil(t, dl.getPortsFromPs(co)) // return must be nil to trigger GetPorts on resolution

	co.Ports = make([]types.Port, 0)
	assert.Empty(t, dl.getPortsFromPs(co))

	co.Ports = append(co.Ports, types.Port{PrivatePort: 4321})
	co.Ports = append(co.Ports, types.Port{PrivatePort: 1234})
	ports := dl.getPortsFromPs(co)

	// Make sure the order is OK too
	assert.Equal(t, []ContainerPort{{1234, ""}, {4321, ""}}, ports)
}

func TestGetADIdentifiers(t *testing.T) {
	s := DockerService{cID: "deadbeef"}

	// Setting mocked data in cache
	co := types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{ID: "deadbeef", Image: "org/test"},
		Mounts:            make([]types.MountPoint, 0),
		Config:            &container.Config{},
		NetworkSettings:   &types.NetworkSettings{},
	}
	cacheKey := docker.GetInspectCacheKey("deadbeef", false)
	cache.Cache.Set(cacheKey, co, 10*time.Second)

	ids, err := s.GetADIdentifiers()
	assert.Nil(t, err)
	assert.Equal(t, []string{"docker://deadbeef", "org/test", "test"}, ids)

	s = DockerService{cID: "deadbeef"}
	labeledCo := types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{ID: "deadbeef", Image: "test"},
		Mounts:            make([]types.MountPoint, 0),
		Config:            &container.Config{Labels: map[string]string{"com.datadoghq.ad.check.id": "w00tw00t"}},
		NetworkSettings:   &types.NetworkSettings{},
	}
	cache.Cache.Set(cacheKey, labeledCo, 10*time.Second)

	ids, err = s.GetADIdentifiers()
	assert.Nil(t, err)
	assert.Equal(t, []string{"w00tw00t"}, ids)
}

func TestGetHosts(t *testing.T) {
	id := "fooooooooooo"
	cBase := types.ContainerJSONBase{
		ID:    id,
		Image: "test",
	}
	cj := types.ContainerJSON{
		ContainerJSONBase: &cBase,
		Mounts:            make([]types.MountPoint, 0),
		Config:            &container.Config{Labels: map[string]string{"com.datadoghq.ad.check.id": "w00tw00t"}},
		NetworkSettings:   &types.NetworkSettings{},
	}
	// add cj to the cache to avoir having to query docker in the test
	cacheKey := docker.GetInspectCacheKey(id, false)
	cache.Cache.Set(cacheKey, cj, 10*time.Second)

	svc := DockerService{
		cID: id,
	}

	res, _ := svc.GetHosts()
	assert.Empty(t, res)

	nets := make(map[string]*network.EndpointSettings)
	nets["bridge"] = &network.EndpointSettings{IPAddress: "172.17.0.2"}
	nets["foo"] = &network.EndpointSettings{IPAddress: "172.17.0.3"}
	ports := make(nat.PortMap)
	p, _ := nat.NewPort("tcp", "1337")
	ports[p] = make([]nat.PortBinding, 0)
	p, _ = nat.NewPort("tcp", "42")
	ports[p] = make([]nat.PortBinding, 0)

	id = "deadbeefffff"
	cBase = types.ContainerJSONBase{
		ID:    id,
		Image: "test",
	}
	networkSettings := types.NetworkSettings{
		NetworkSettingsBase: types.NetworkSettingsBase{Ports: ports},
		Networks:            nets,
	}

	cj = types.ContainerJSON{
		ContainerJSONBase: &cBase,
		Mounts:            make([]types.MountPoint, 0),
		Config: &container.Config{
			// Should NOT be picked up, as we have valid IPs
			Hostname: "ip-172-29-161-245.ec2.internal",
		},
		NetworkSettings: &networkSettings,
	}
	// update cj in the cache
	cacheKey = docker.GetInspectCacheKey(id, false)
	cache.Cache.Set(cacheKey, cj, 10*time.Second)

	svc = DockerService{
		cID: id,
	}
	hosts, _ := svc.GetHosts()

	assert.Equal(t, "172.17.0.2", hosts["bridge"])
	assert.Equal(t, "172.17.0.3", hosts["foo"])
	assert.Equal(t, 2, len(hosts))
}

func TestGetRancherIP(t *testing.T) {
	id := "fooooooooooo"
	cBase := types.ContainerJSONBase{
		ID:    id,
		Image: "test",
	}

	nets := make(map[string]*network.EndpointSettings)
	nets["none"] = &network.EndpointSettings{}

	networkSettings := types.NetworkSettings{
		Networks: nets,
	}

	cj := types.ContainerJSON{
		ContainerJSONBase: &cBase,
		Mounts:            make([]types.MountPoint, 0),
		Config: &container.Config{Labels: map[string]string{
			"com.datadoghq.ad.check.id": "w00tw00t",
			"io.rancher.container.ip":   "10.42.90.224/16",
		}},
		NetworkSettings: &networkSettings,
	}
	// add cj to the cache to avoir having to query docker in the test
	cacheKey := docker.GetInspectCacheKey(id, false)
	cache.Cache.Set(cacheKey, cj, 10*time.Second)

	svc := DockerService{
		cID: id,
	}

	hosts, _ := svc.GetHosts()
	assert.Equal(t, "10.42.90.224", hosts["rancher"])
	assert.Equal(t, 1, len(hosts))
}

func TestFallbackToHostname(t *testing.T) {
	id := "fooooooooooo"
	cBase := types.ContainerJSONBase{
		ID:    id,
		Image: "test",
	}

	nets := make(map[string]*network.EndpointSettings)
	nets["none"] = &network.EndpointSettings{}

	networkSettings := types.NetworkSettings{
		Networks: nets,
	}

	cj := types.ContainerJSON{
		ContainerJSONBase: &cBase,
		Mounts:            make([]types.MountPoint, 0),
		Config: &container.Config{
			Hostname: "ip-172-29-161-245.ec2.internal",
		},
		NetworkSettings: &networkSettings,
	}
	// add cj to the cache to avoir having to query docker in the test
	cacheKey := docker.GetInspectCacheKey(id, false)
	cache.Cache.Set(cacheKey, cj, 10*time.Second)

	svc := DockerService{
		cID: id,
	}

	hosts, _ := svc.GetHosts()
	assert.Equal(t, "ip-172-29-161-245.ec2.internal", hosts["hostname"])
	assert.Equal(t, 1, len(hosts))
}

func TestGetPorts(t *testing.T) {
	id := "no_ports"
	cBase := types.ContainerJSONBase{
		ID:    id,
		Image: "test",
	}

	// Priority source
	ports := make(nat.PortMap)
	networkSettings := types.NetworkSettings{
		NetworkSettingsBase: types.NetworkSettingsBase{Ports: ports},
		Networks:            make(map[string]*network.EndpointSettings),
	}

	// Fallback source
	exposedPorts := make(map[nat.Port]struct{})

	// Empty ports
	cj := types.ContainerJSON{
		ContainerJSONBase: &cBase,
		Mounts:            make([]types.MountPoint, 0),
		Config: &container.Config{
			ExposedPorts: exposedPorts,
		},
		NetworkSettings: &networkSettings,
	}
	// add cj to the cache so svc.GetPorts finds it
	cacheKey := docker.GetInspectCacheKey(id, false)
	cache.Cache.Set(cacheKey, cj, 10*time.Second)

	svc := DockerService{
		cID: id,
	}
	svcPorts, _ := svc.GetPorts()
	assert.NotNil(t, svcPorts) // Return array must be non-nil to avoid calling GetPorts again
	assert.Empty(t, svcPorts)

	// Only exposed ports, should be picked up
	id = "only_exposed"
	cBase = types.ContainerJSONBase{
		ID:    id,
		Image: "test",
	}

	ep, _ := nat.NewPort("tcp", "42-45")
	exposedPorts[ep] = struct{}{}

	networkSettings = types.NetworkSettings{
		NetworkSettingsBase: types.NetworkSettingsBase{Ports: ports},
		Networks:            make(map[string]*network.EndpointSettings),
	}

	cj = types.ContainerJSON{
		ContainerJSONBase: &cBase,
		Mounts:            make([]types.MountPoint, 0),
		Config: &container.Config{
			ExposedPorts: exposedPorts,
		},
		NetworkSettings: &networkSettings,
	}
	// add cj to the cache so svc.GetPorts finds it
	cacheKey = docker.GetInspectCacheKey(id, false)
	cache.Cache.Set(cacheKey, cj, 10*time.Second)

	svc = DockerService{
		cID: id,
	}

	pts, _ := svc.GetPorts()
	assert.Equal(t, 4, len(pts))
	assert.Contains(t, pts, ContainerPort{42, ""})
	assert.Contains(t, pts, ContainerPort{43, ""})
	assert.Contains(t, pts, ContainerPort{44, ""})
	assert.Contains(t, pts, ContainerPort{45, ""})

	// Both binding ports and exposed ports, only firsts should be picked up
	id = "test"
	cBase = types.ContainerJSONBase{
		ID:    id,
		Image: "test",
	}

	ports = make(nat.PortMap, 2)
	p, _ := nat.NewPort("tcp", "4321")
	ports[p] = nil
	p, _ = nat.NewPort("tcp", "1234")
	ports[p] = nil

	networkSettings = types.NetworkSettings{
		NetworkSettingsBase: types.NetworkSettingsBase{Ports: ports},
		Networks:            make(map[string]*network.EndpointSettings),
	}

	cj = types.ContainerJSON{
		ContainerJSONBase: &cBase,
		Mounts:            make([]types.MountPoint, 0),
		Config: &container.Config{
			ExposedPorts: exposedPorts,
		},
		NetworkSettings: &networkSettings,
	}
	// add cj to the cache so svc.GetPorts finds it
	cacheKey = docker.GetInspectCacheKey(id, false)
	cache.Cache.Set(cacheKey, cj, 10*time.Second)

	svc = DockerService{
		cID: id,
	}

	pts, _ = svc.GetPorts()
	assert.Equal(t, []ContainerPort{{1234, ""}, {4321, ""}}, pts)
}

func TestGetPid(t *testing.T) {
	s := DockerService{cID: "foo"}

	// Setting mocked data in cache
	state := types.ContainerState{Pid: 1337}
	cBase := types.ContainerJSONBase{
		ID:    "foo",
		Image: "test",
		State: &state,
	}
	co := types.ContainerJSON{ContainerJSONBase: &cBase}
	cacheKey := docker.GetInspectCacheKey("foo", false)
	cache.Cache.Set(cacheKey, co, 10*time.Second)

	pid, err := s.GetPid()
	assert.Equal(t, 1337, pid)
	assert.Nil(t, err)
}

func TestParseDockerPort(t *testing.T) {
	testCases := []struct {
		proto         string
		port          string
		expectedPorts []ContainerPort
		expectedError error
	}{
		{
			proto:         "tcp",
			port:          "42",
			expectedPorts: []ContainerPort{{42, ""}},
			expectedError: nil,
		},
		{
			proto:         "udp",
			port:          "500-503",
			expectedPorts: []ContainerPort{{500, ""}, {501, ""}, {502, ""}, {503, ""}},
			expectedError: nil,
		},
		{
			proto:         "tcp",
			port:          "0",
			expectedPorts: nil,
			expectedError: errors.New("failed to extract port from: 0/tcp"),
		},
	}

	for i, test := range testCases {
		t.Run(fmt.Sprintf("case %d: %s/%s", i, test.port, test.proto), func(t *testing.T) {
			p, err := nat.NewPort(test.proto, test.port)
			assert.Nil(t, err)

			ports, err := parseDockerPort(p)
			if test.expectedError == nil {
				assert.Nil(t, err)
			} else {
				require.NotNil(t, err)
				assert.Equal(t, test.expectedError.Error(), err.Error())
			}

			assert.Equal(t, test.expectedPorts, ports)
		})
	}
}

func TestGetHostname(t *testing.T) {
	cId := "12345678901234567890123456789012"
	cBase := types.ContainerJSONBase{
		ID:    cId,
		Image: "test",
	}

	testCases := []struct {
		hostname      string
		domainname    string
		expected      string
		expectedError error
	}{
		{
			hostname:      "",
			domainname:    "",
			expected:      "",
			expectedError: errors.New("empty hostname for container 123456789012"),
		},
		{
			hostname:      "host",
			domainname:    "",
			expected:      "host",
			expectedError: nil,
		},
		{
			hostname:      "host",
			domainname:    "domain",
			expected:      "host",
			expectedError: nil,
		},
		{
			hostname:      "",
			domainname:    "domain",
			expected:      "",
			expectedError: errors.New("empty hostname for container 123456789012"),
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("case %d: %s.%s", i, tc.hostname, tc.domainname), func(t *testing.T) {
			cj := types.ContainerJSON{
				ContainerJSONBase: &cBase,
				Config: &container.Config{
					Hostname:   tc.hostname,
					Domainname: tc.domainname,
				},
			}
			// add cj to the cache so svc.GetPorts finds it
			cacheKey := docker.GetInspectCacheKey(cId, false)
			cache.Cache.Set(cacheKey, cj, 10*time.Second)

			svc := DockerService{
				cID: cId,
			}

			name, err := svc.GetHostname()
			assert.Equal(t, tc.expected, name)

			if tc.expectedError == nil {
				assert.NoError(t, err)
			} else {
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			}

		})
	}
}
