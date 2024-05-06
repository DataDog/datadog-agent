//go:build linux_bpf
// +build linux_bpf

package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	networkconfig "github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/usm"
)

var (
	srcPort = flag.Int("src_port", 0, "src port filter")
	dstPort = flag.Int("dst_port", 0, "dst port filter")
	srcAddr = flag.String("src_addr", "", "src addr filter")
	dstAddr = flag.String("dst_addr", "", "dst addr filter")
)

func main() {
	err := ddconfig.SetupLogger(
		"usm-debugger",
		"debug",
		"",
		ddconfig.GetSyslogURI(),
		false,
		true,
		false,
	)
	checkError(err)

	cleanupFn := setupBytecode()
	defer cleanupFn()

	monitor, err := usm.NewMonitor(getConfiguration(), nil)
	checkError(err)

	err = monitor.Start()
	checkError(err)

	go func() {
		t := time.NewTicker(10 * time.Second)
		for range t.C {
			_ = monitor.GetProtocolStats()
		}
	}()

	defer monitor.Stop()
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	<-done
}

func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", err)
		os.Exit(-1)
	}
}

func getConfiguration() *networkconfig.Config {
	// done for the purposes of initializing the configuration values
	_, err := config.New("")
	checkError(err)

	c := networkconfig.New()

	// run debug version of the eBPF program
	c.BPFDebug = true

	// don't buffer data in userspace
	// this is to ensure that we won't inadvertently trigger an OOM kill
	// by enabling the debugger inside a system-probe container.
	c.MaxHTTPStatsBuffered = 0
	c.MaxKafkaStatsBuffered = 0

	// make sure we use the CO-RE compilation artifact embedded
	// in this build (see `ebpf_bytecode.go`)
	c.EnableCORE = true
	c.EnableRuntimeCompiler = false
	c.AllowPrecompiledFallback = false

	// configure filters using command line arguments
	flag.Parse()
	c.USMFilterSport = *srcPort
	c.USMFilterDport = *dstPort
	c.USMFilterSaddr = *srcAddr
	c.USMFilterDaddr = *dstAddr

	return c
}
