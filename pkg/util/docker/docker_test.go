package docker

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	dockernetwork "github.com/docker/docker/api/types/network"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestFindDockerNetworks(t *testing.T) {
	assert := assert.New(t)
	dummyProcDir, err := ioutil.TempDir("", "test-find-docker-networks")
	assert.Nil(err)
	defer os.RemoveAll(dummyProcDir) // clean up
	config.Datadog.SetDefault("proc_root", dummyProcDir)

	containerID := "test-find-docker-networks"
	for _, tc := range []struct {
		pid         int
		settings    *types.SummaryNetworkSettings
		routes, dev string
		networks    []dockerNetwork
		stat        *NetworkStat
	}{
		{
			pid: 1245,
			settings: &types.SummaryNetworkSettings{
				Networks: map[string]*dockernetwork.EndpointSettings{
					"eth0": &dockernetwork.EndpointSettings{
						Gateway: "172.0.0.1/24",
					},
				},
			},
			routes: detab(`
				Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
				eth0	00000000	010011AC	0003	0	0	0	00000000	0	0	0

				eth0	000011AC	00000000	0001	0	0	0	0000FFFF	0	0	0
			`),
			dev: detab(`
				Inter-|   Receive                                                |  Transmit
				 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
				  eth0:    1296      16    0    0    0     0          0         0        0       0    0    0    0     0       0          0
				    lo:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
			`),
			networks: []dockerNetwork{dockerNetwork{iface: "eth0", dockerName: "eth0"}},
			stat: &NetworkStat{
				BytesRcvd:   1296,
				PacketsRcvd: 16,
				BytesSent:   0,
				PacketsSent: 0,
			},
		},
		{
			pid: 5152,
			settings: &types.SummaryNetworkSettings{
				Networks: map[string]*dockernetwork.EndpointSettings{
					"isolated_nw": &dockernetwork.EndpointSettings{
						Gateway: "172.18.0.1",
					},
					"eth0": &dockernetwork.EndpointSettings{
						Gateway: "172.0.0.4/24",
					},
				},
			},
			routes: detab(`
				Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
				eth0	00000000	010012AC	0003	0	0	0	00000000	0	0	0

				eth0	000012AC	00000000	0001	0	0	0	0000FFFF	0	0	0
			`),
			dev: detab(`
				Inter-|   Receive                                                |  Transmit
				 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
				  eth0:    1111       2    0    0    0     0          0         0     1024      80    0    0    0     0       0          0
				    lo:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
			`),
			networks: []dockerNetwork{
				dockerNetwork{iface: "eth0", dockerName: "eth0"},
				dockerNetwork{iface: "eth0", dockerName: "isolated_nw"},
			},
			stat: &NetworkStat{
				BytesRcvd:   1111,
				PacketsRcvd: 2,
				BytesSent:   1024,
				PacketsSent: 80,
			},
		},
		// Dumb error case to make sure we don't panic
		{
			pid: 5157,
			settings: &types.SummaryNetworkSettings{
				Networks: map[string]*dockernetwork.EndpointSettings{
					"isolated_nw": &dockernetwork.EndpointSettings{
						Gateway: "172.18.0.1",
					},
					"eth0": &dockernetwork.EndpointSettings{
						Gateway: "172.0.0.4/24",
					},
				},
			},
			routes:   detab(``),
			networks: nil,
			dev: detab(`
				Inter-|   Receive                                                |  Transmit
				 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
				  eth0:    1111       2    0    0    0     0          0         0     1024      80    0    0    0     0       0          0
				    lo:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
			`),
			stat: &NetworkStat{},
		},
	} {
		// Create temporary files on disk with the routes and stats.
		routePath := filepath.Join(dummyProcDir, strconv.Itoa(int(tc.pid)), "net")
		err := os.MkdirAll(routePath, 0777)
		assert.NoError(err)
		f1, err := os.Create(filepath.Join(dummyProcDir, strconv.Itoa(int(tc.pid)), "net", "route"))
		assert.NoError(err)
		f1.WriteString(tc.routes)
		f2, err := os.Create(filepath.Join(dummyProcDir, strconv.Itoa(int(tc.pid)), "net", "dev"))
		assert.NoError(err)
		f2.WriteString(tc.dev)

		// Use the routes file and settings to get our networks.
		networks := findDockerNetworks(containerID, tc.pid, tc.settings)
		assert.Equal(tc.networks, networks)

		// And collect the stats on these networks.
		stat, err := collectNetworkStats(containerID, tc.pid, networks)
		assert.NoError(err)
		assert.Equal(tc.stat, stat)

		f1.Close()
		f2.Close()
	}
}

// detab removes whitespace from the front of a string on every line
func detab(str string) string {
	detabbed := make([]string, 0)
	for _, l := range strings.Split(str, "\n") {
		s := strings.TrimSpace(l)
		if len(s) > 0 {
			detabbed = append(detabbed, s)
		}
	}
	return strings.Join(detabbed, "\n")
}

// Sanity-check that all containers works with different settings.
func TestAllContainers(t *testing.T) {
	InitDockerUtil(&Config{CollectNetwork: true})
	AllContainers()
	InitDockerUtil(&Config{CollectNetwork: false})
	AllContainers()
}

func TestContainerFilter(t *testing.T) {
	assert := assert.New(t)
	containers := []*Container{
		{ID: "1", Name: "secret-container-dd", Image: "docker-dd-agent"},
		{ID: "2", Name: "webapp1-dd", Image: "apache:2.2"},
		{ID: "3", Name: "mysql-dd", Image: "mysql:5.3"},
		{ID: "4", Name: "linux-dd", Image: "alpine:latest"},
		{ID: "5", Name: "k8s_POD.f8120f_kube-proxy-gke-pool-1-2890-pv0", Image: "gcr.io/google_containers/pause-amd64:3.0"},
	}

	for i, tc := range []struct {
		whitelist   []string
		blacklist   []string
		expectedIDs []string
	}{
		{
			expectedIDs: []string{"1", "2", "3", "4", "5"},
		},
		{
			blacklist:   []string{"name:secret"},
			expectedIDs: []string{"2", "3", "4", "5"},
		},
		{
			blacklist:   []string{"image:secret"},
			expectedIDs: []string{"1", "2", "3", "4", "5"},
		},
		{
			whitelist:   []string{},
			blacklist:   []string{"image:apache", "image:alpine"},
			expectedIDs: []string{"1", "3", "5"},
		},
		{
			whitelist:   []string{"name:mysql"},
			blacklist:   []string{"name:dd"},
			expectedIDs: []string{"3", "5"},
		},
		// Test kubernetes defaults
		{
			blacklist: []string{
				"image:gcr.io/google_containers/pause.*",
				"image:openshift/origin-pod",
			},
			expectedIDs: []string{"1", "2", "3", "4"},
		},
	} {
		f, err := newContainerFilter(tc.whitelist, tc.blacklist)
		assert.NoError(err, "case %d", i)

		var allowed []string
		for _, c := range containers {
			if !f.IsExcluded(c) {
				allowed = append(allowed, c.ID)
			}
		}
		assert.Equal(tc.expectedIDs, allowed, "case %d", i)
	}
}

func TestParseContainerHealth(t *testing.T) {
	assert := assert.New(t)
	for i, tc := range []struct {
		input    string
		expected string
	}{
		{
			input:    "",
			expected: "",
		},
		{
			input:    "Up 2 minutes",
			expected: "",
		},
		{
			input:    "Up about 1 hour (health: starting)",
			expected: "starting",
		},
		{
			input:    "Up 1 minute (health: unhealthy)",
			expected: "unhealthy",
		},
	} {
		assert.Equal(tc.expected, parseContainerHealth(tc.input), "test %d failed", i)
	}
}
