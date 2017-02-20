package host

import (
	"os"
	"path"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/check/py"
	"github.com/DataDog/datadog-agent/pkg/collector/metadata"
	"github.com/DataDog/datadog-agent/pkg/util"
	python "github.com/sbinet/go-python"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/host"
	"github.com/stretchr/testify/assert"
)

// Setup the test module
func TestMain(m *testing.M) {
	state := py.Initialize()

	ret := m.Run()

	python.PyEval_RestoreThread(state)
	python.Finalize()

	os.Exit(ret)
}

func TestGetPayload(t *testing.T) {
	p := GetPayload("myhostname")
	assert.NotEmpty(t, p.Os)
	assert.NotEmpty(t, p.PythonVersion)
	assert.Equal(t, "myhostname", p.InternalHostname)
	assert.NotEmpty(t, p.UUID)
	assert.NotNil(t, p.SytemStats)
	assert.NotNil(t, p.Meta)
}

func TestGetSystemStats(t *testing.T) {
	assert.NotNil(t, getSystemStats())
	fakeStats := &systemStats{Machine: "fooMachine"}
	key := path.Join(metadata.CachePrefix, "systemStats")
	util.Cache.Set(key, fakeStats, util.NoExpiration)
	s := getSystemStats()
	assert.NotNil(t, s)
	assert.Equal(t, fakeStats.Machine, s.Machine)
}

func TestGetPythonVersion(t *testing.T) {
	assert.NotEmpty(t, getPythonVersion())
	key := path.Join(metadata.CachePrefix, "python")
	util.Cache.Set(key, "Python 2.8", util.NoExpiration)
	assert.Equal(t, "Python 2.8", getPythonVersion())
}

func TestGetCPUInfo(t *testing.T) {
	assert.NotNil(t, getCPUInfo())
	fakeInfo := &cpu.InfoStat{Cores: 42}
	key := path.Join(metadata.CachePrefix, "cpuInfo")
	util.Cache.Set(key, fakeInfo, util.NoExpiration)
	info := getCPUInfo()
	assert.Equal(t, int32(42), info.Cores)
}

func TestGetHostInfo(t *testing.T) {
	assert.NotNil(t, getHostInfo())
	fakeInfo := &host.InfoStat{HostID: "FOOBAR"}
	key := path.Join(metadata.CachePrefix, "hostInfo")
	util.Cache.Set(key, fakeInfo, util.NoExpiration)
	info := getHostInfo()
	assert.Equal(t, "FOOBAR", info.HostID)
}

func TestGetMeta(t *testing.T) {
	meta := getMeta()
	assert.NotEmpty(t, meta.SocketHostname)
	assert.NotEmpty(t, meta.Timezones)
	assert.NotEmpty(t, meta.SocketFqdn)
	assert.Empty(t, meta.EC2Hostname) // this is temporary
}
