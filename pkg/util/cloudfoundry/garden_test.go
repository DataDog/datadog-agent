package cloudfoundry

import (
	"net"
	"testing"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/garden/gardenfakes"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/providers"
	providerMocks "github.com/DataDog/datadog-agent/pkg/util/containers/providers/mock"
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

func TestListContainers(t *testing.T) {
	defer providers.Deregister()
	providers.Register(providerMocks.FakeContainerImpl{})
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
