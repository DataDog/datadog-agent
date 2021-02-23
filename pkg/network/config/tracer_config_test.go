// +build linux_bpf

package config

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/stretchr/testify/require"
)

func TestProcSysGetInt(t *testing.T) {
	procRoot := util.GetProcRoot()
	v := procSysGetInt(procRoot, "foo", -1)
	require.Equal(t, v, -1)

	v = procSysGetInt(procRoot, "net/ipv4/ip_forward", -1)
	require.NotEqual(t, v, -1)
}

func TestSysUDPTimeout(t *testing.T) {
	// ensure conntrack is enabled
	cmd := exec.Command("iptables", "-I", "INPUT", "1", "-m", "conntrack", "--ctstate", "NEW,RELATED,ESTABLISHED", "-j", "ACCEPT")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "error running iptables command, output: %s", out)
	defer func() {
		cmd := exec.Command("iptables", "-D", "INPUT", "1")
		cmd.CombinedOutput()
	}()

	procRoot := util.GetProcRoot()
	v := procSysGetInt(procRoot, "net/netfilter/nf_conntrack_udp_timeout", -1)
	require.NotEqual(t, v, -1)
	require.Equal(t, time.Duration(v)*time.Second, sysUDPTimeout())
}

func TestSysUDPStreamTimeout(t *testing.T) {
	// ensure conntrack is enabled
	cmd := exec.Command("iptables", "-I", "INPUT", "1", "-m", "conntrack", "--ctstate", "NEW,RELATED,ESTABLISHED", "-j", "ACCEPT")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "error running iptables command, output: %s", out)
	defer func() {
		cmd := exec.Command("iptables", "-D", "INPUT", "1")
		cmd.CombinedOutput()
	}()

	procRoot := util.GetProcRoot()
	v := procSysGetInt(procRoot, "net/netfilter/nf_conntrack_udp_timeout_stream", -1)
	require.NotEqual(t, v, -1)
	require.Equal(t, time.Duration(v)*time.Second, sysUDPStreamTimeout())

}

func TestSysUDPTimeoutDefault(t *testing.T) {
	hostProc := os.Getenv("HOST_PROC")
	defer os.Setenv("HOST_PROC", hostProc)
	os.Setenv("HOST_PROC", "/foo")

	require.Equal(t, time.Duration(defaultUDPTimeoutSeconds)*time.Second, sysUDPTimeout())
}

func TestSysUDPStreamTimeoutDefault(t *testing.T) {
	hostProc := os.Getenv("HOST_PROC")
	defer os.Setenv("HOST_PROC", hostProc)
	os.Setenv("HOST_PROC", "/foo")

	require.Equal(t, time.Duration(defaultUDPStreamTimeoutSeconds)*time.Second, sysUDPStreamTimeout())

}
