package cloudfoundry

import (
	"code.cloudfoundry.org/garden/gardenfakes"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/containers/providers"
	"net"
	"testing"

	"code.cloudfoundry.org/garden"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/stretchr/testify/assert"
)

func TestParseContainerPorts(t *testing.T) {
	info := garden.ContainerInfo{
		MappedPorts: []garden.PortMapping{
			{
				HostPort:      10,
				ContainerPort: 20,
			},
			{
				HostPort:      11,
				ContainerPort: 21,
			},
		},
		ExternalIP: "127.0.0.1",
	}
	addresses := parseContainerPorts(info)
	expected := []containers.NetworkAddress{
		{
			IP:       net.ParseIP("127.0.0.1"),
			Port:     10,
			Protocol: "tcp",
		},
		{
			IP:       net.ParseIP("127.0.0.1"),
			Port:     11,
			Protocol: "tcp",
		},
	}
	assert.Equal(t, expected, addresses)
}

type fakeContainerImpl struct {
}

func (f fakeContainerImpl) Prefetch() error {
	return nil
}

func (f fakeContainerImpl) ContainerExists(containerID string) bool {
	return true
}

func (f fakeContainerImpl) GetContainerStartTime(containerID string) (int64, error) {
	panic("implement me")
}

func (f fakeContainerImpl) DetectNetworkDestinations(pid int) ([]containers.NetworkDestination, error) {
	panic("implement me")
}

func (f fakeContainerImpl) GetAgentCID() (string, error) {
	panic("implement me")
}

func (f fakeContainerImpl) GetPIDs(containerID string) ([]int32, error) {
	return []int32{123}, nil
}

func (f fakeContainerImpl) ContainerIDForPID(pid int) (string, error) {
	panic("implement me")
}

func (f fakeContainerImpl) GetDefaultGateway() (net.IP, error) {
	panic("implement me")
}

func (f fakeContainerImpl) GetDefaultHostIPs() ([]string, error) {
	panic("implement me")
}

func (f fakeContainerImpl) GetContainerMetrics(containerID string) (*metrics.ContainerMetrics, error) {
	return nil, nil
}

func (f fakeContainerImpl) GetContainerLimits(containerID string) (*metrics.ContainerLimits, error) {
	return nil, nil
}

func (f fakeContainerImpl) GetNetworkMetrics(containerID string, networks map[string]string) (metrics.ContainerNetStats, error) {
	return nil, nil
}

func TestListContainers(t *testing.T) {
	defer providers.Deregister()
	providers.Register(fakeContainerImpl{})
	cli := gardenfakes.FakeClient{}
	bulkContainers := map[string]garden.ContainerInfoEntry{
		"ok": {
			Info: garden.ContainerInfo{
				State: "active",
			},
			Err: nil,
		},
		"ok err metrics": {
			Info: garden.ContainerInfo{
				State: "active",
			},
			Err: nil,
		},
		"not ok": {
			Info: garden.ContainerInfo{
				State: "on fire",
			},
			Err: garden.NewError("problem!"),
		},
	}
	metrics := map[string]garden.ContainerMetricsEntry{
		"ok":             {},
		"ok err metrics": {Err: garden.NewError("another problem!")},
		"not ok":         {},
	}
	okc := gardenfakes.FakeContainer{}
	okc.HandleReturns("ok")
	oknometricsc := gardenfakes.FakeContainer{}
	oknometricsc.HandleReturns("ok err metrics")
	nokc := gardenfakes.FakeContainer{}
	nokc.HandleReturns("not ok")
	containers := []garden.Container{
		&okc, &oknometricsc, &nokc,
	}

	cli.BulkInfoReturns(bulkContainers, nil)
	cli.BulkMetricsReturns(metrics, nil)
	cli.ContainersReturns(containers, nil)
	gu := GardenUtil{
		cli: &cli,
	}

	result, err := gu.ListContainers()
	assert.Nil(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "ok", result[0].ID)
}
