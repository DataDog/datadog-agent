// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package listeners

import (
	"os"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/assert"

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
		Labels: map[string]string{"io.datadog.check.id": "w00tw00t"},
	}
	ids = dl.getConfigIDFromPs(labeledCo)
	assert.Equal(t, []string{"w00tw00t"}, ids)
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

	co.Ports = make([]types.Port, 0)
	assert.Empty(t, dl.getPortsFromPs(co))

	co.Ports = append(co.Ports, types.Port{PrivatePort: 1234})
	co.Ports = append(co.Ports, types.Port{PrivatePort: 4321})
	ports := dl.getPortsFromPs(co)
	assert.Equal(t, 2, len(ports))
	assert.Contains(t, ports, 1234)
	assert.Contains(t, ports, 4321)
}

func TestGetADIdentifiers(t *testing.T) {
	s := DockerService{ID: ID("deadbeef")}

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

	s = DockerService{ID: ID("deadbeef")}
	labeledCo := types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{ID: "deadbeef", Image: "test"},
		Mounts:            make([]types.MountPoint, 0),
		Config:            &container.Config{Labels: map[string]string{"io.datadog.check.id": "w00tw00t"}},
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
		Config:            &container.Config{Labels: map[string]string{"io.datadog.check.id": "w00tw00t"}},
		NetworkSettings:   &types.NetworkSettings{},
	}
	// add cj to the cache to avoir having to query docker in the test
	cacheKey := docker.GetInspectCacheKey(id, false)
	cache.Cache.Set(cacheKey, cj, 10*time.Second)

	svc := DockerService{
		ID: ID(id),
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
		Config:            &container.Config{},
		NetworkSettings:   &networkSettings,
	}
	// update cj in the cache
	cacheKey = docker.GetInspectCacheKey(id, false)
	cache.Cache.Set(cacheKey, cj, 10*time.Second)

	svc = DockerService{
		ID: ID(id),
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
			"io.datadog.check.id":     "w00tw00t",
			"io.rancher.container.ip": "10.42.90.224/16",
		}},
		NetworkSettings: &networkSettings,
	}
	// add cj to the cache to avoir having to query docker in the test
	cacheKey := docker.GetInspectCacheKey(id, false)
	cache.Cache.Set(cacheKey, cj, 10*time.Second)

	svc := DockerService{
		ID: ID(id),
	}

	hosts, _ := svc.GetHosts()
	assert.Equal(t, "10.42.90.224", hosts["rancher"])
	assert.Equal(t, 1, len(hosts))
}

func TestGetPorts(t *testing.T) {
	id := "deadbeefffff"
	cBase := types.ContainerJSONBase{
		ID:    id,
		Image: "test",
	}

	ports := make(nat.PortMap)
	networkSettings := types.NetworkSettings{
		NetworkSettingsBase: types.NetworkSettingsBase{Ports: ports},
		Networks:            make(map[string]*network.EndpointSettings),
	}

	cj := types.ContainerJSON{
		ContainerJSONBase: &cBase,
		Mounts:            make([]types.MountPoint, 0),
		Config:            &container.Config{},
		NetworkSettings:   &networkSettings,
	}
	// add cj to the cache so svc.GetPorts finds it
	cacheKey := docker.GetInspectCacheKey(id, false)
	cache.Cache.Set(cacheKey, cj, 10*time.Second)

	svc := DockerService{
		ID: ID(id),
	}
	svcPorts, _ := svc.GetPorts()
	assert.Empty(t, svcPorts)

	id = "test"
	cBase = types.ContainerJSONBase{
		ID:    id,
		Image: "test",
	}

	ports = make(nat.PortMap, 2)
	p, _ := nat.NewPort("tcp", "1234")
	ports[p] = nil
	p, _ = nat.NewPort("tcp", "4321")
	ports[p] = nil

	networkSettings = types.NetworkSettings{
		NetworkSettingsBase: types.NetworkSettingsBase{Ports: ports},
		Networks:            make(map[string]*network.EndpointSettings),
	}

	cj = types.ContainerJSON{
		ContainerJSONBase: &cBase,
		Mounts:            make([]types.MountPoint, 0),
		Config:            &container.Config{},
		NetworkSettings:   &networkSettings,
	}
	// add cj to the cache so svc.GetPorts finds it
	cacheKey = docker.GetInspectCacheKey(id, false)
	cache.Cache.Set(cacheKey, cj, 10*time.Second)

	svc = DockerService{
		ID: ID(id),
	}

	pts, _ := svc.GetPorts()
	assert.Equal(t, 2, len(pts))
	assert.Contains(t, pts, 1234)
	assert.Contains(t, pts, 4321)
}

func TestGetPid(t *testing.T) {
	s := DockerService{ID: ID("foo")}

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
