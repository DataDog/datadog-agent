// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen_test

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	"github.com/DataDog/datadog-agent/pkg/dyninst/config"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

func TestIRGen(t *testing.T) {
	binary, err := safeelf.Open("/git/datadog-agent/pkg/dyninst/test/pinger")
	require.NoError(t, err)
	require.NoError(t, err)
	defer binary.Close()
	var probes []config.Probe
	// symbols, err := binary.Symbols()
	// for i, s := range symbols {
	// 	if strings.HasPrefix(s.Name, "type:.") {
	// 		continue
	// 	}
	// 	fmt.Printf("symbol: %#v\n", s)
	// 	probes = append(probes, &config.LogProbe{
	// 		ID: fmt.Sprintf("probe_%d", i),
	// 		Where: &config.Where{
	// 			MethodName: s.Name,
	// 		},
	// 	})
	// }
	probes = append(probes, &config.LogProbe{
		ID: "probe_ping",
		Where: &config.Where{
			MethodName: "main.ping",
		},
	})

	obj, err := object.NewElfObject(binary)
	require.NoError(t, err)
	ir, err := irgen.GenerateIR(1, obj, probes)
	require.NoError(t, err)
	require.NotNil(t, ir)
	// m, err := json.Marshal(ir)
	// require.NoError(t, err)
	// fmt.Printf("%s\n", m)

	bpfObj, err := compiler.CompileBPFProgram(*ir)
	require.NoError(t, err)

	// f, err := os.Create("/git/datadog-agent/pkg/dyninst/test/pinger.bpf.o")
	// require.NoError(t, err)
	// defer f.Close()
	// _, err = io.Copy(f, bpfObj)
	// require.NoError(t, err)

	spec, err := ebpf.LoadCollectionSpecFromReader(bpfObj)
	require.NoError(t, err)
	// fmt.Printf("%#v\n", spec)

	fmt.Println("Loading...")
	bpfCollection, err := ebpf.NewCollectionWithOptions(spec, ebpf.CollectionOptions{})
	if err != nil {
		var ve *ebpf.VerifierError
		if errors.As(err, &ve) {
			for _, l := range ve.Log {
				fmt.Println(l)
			}
		}
	}
	require.NoError(t, err)
	defer bpfObj.Close()
	fmt.Printf("%#v\n", bpfObj)

	exec, err := link.OpenExecutable("/git/datadog-agent/pkg/dyninst/test/pinger")
	require.NoError(t, err)

	probeProg, ok := bpfCollection.Programs["probe_run_with_cookie"]
	require.True(t, ok)
	fmt.Printf("%#v\n", probeProg)

	probe, err := exec.Uprobe("main.ping", probeProg, &link.UprobeOptions{
		PID:    96453,
		Cookie: 0,
	})
	require.NoError(t, err)
	fmt.Printf("%#v\n", probe)
	defer probe.Close()

	rd, err := ringbuf.NewReader(bpfCollection.Maps["out_ringbuf"])
	require.NoError(t, err)
	defer rd.Close()

	stopper := make(chan os.Signal, 1)
	signal.Notify(stopper, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stopper

		if err := rd.Close(); err != nil {
			log.Fatalf("closing ringbuf reader: %s", err)
		}
	}()

	for {
		record, err := rd.Read()
		require.NoError(t, err)

		header := (*output.EventHeader)(unsafe.Pointer(&record.RawSample[0]))
		fmt.Printf("(%d) header: %#v\n", len(record.RawSample), header)

		pos := uint32(unsafe.Sizeof(*header)) + uint32(header.Stack_byte_len)
		for pos < header.Data_byte_len {
			di := (*output.DataItemHeader)(unsafe.Pointer(&record.RawSample[pos]))
			fmt.Printf("%d: data item: %#v %#v\n", pos, di,
				record.RawSample[pos+uint32(unsafe.Sizeof(*di)):pos+uint32(unsafe.Sizeof(*di))+di.Length])
			pos += uint32(unsafe.Sizeof(*di)) + uint32(di.Length)
			if pos%8 > 0 {
				pos += 8 - pos%8
			}
		}
	}
}
