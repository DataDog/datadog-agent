package main

import (
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
)

func main() {
	kernelVersion, err := ebpf.CurrentKernelVersion()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	fmt.Printf("-- Kernel: %d (%d.%d)--\n", kernelVersion, (kernelVersion>>16)&0xff, (kernelVersion>>8)&0xff)

	if ok, err := ebpf.IsTracerSupportedByOS(nil); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	} else if !ok {
		fmt.Fprintln(os.Stderr, "tracer is not supported by current OS")
		os.Exit(1)
	}

	cfg := ebpf.NewDefaultConfig()
	fmt.Printf("-- Config: %+v --\n", cfg)
	cfg.BPFDebug = true

	t, err := ebpf.NewTracer(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Initialization complete. Starting nettop\n")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, os.Kill)

	printConns := func(now time.Time) {
		fmt.Printf("-- %s --\n", now)
		cs, err := t.GetActiveConnections(fmt.Sprintf("%d", os.Getpid()))
		if err != nil {
			fmt.Println(err)
		}
		for _, c := range cs.Conns {
			fmt.Println(c)
		}
	}

	stopChan := make(chan struct{})
	go func() {
		// Print active connections immediately, and then again every 5 seconds
		tick := time.NewTicker(5 * time.Second)
		printConns(time.Now())
		for {
			select {
			case now := <-tick.C:
				printConns(now)
			case <-stopChan:
				tick.Stop()
				return
			}
		}
	}()

	<-sig
	stopChan <- struct{}{}

	t.Stop()
}
