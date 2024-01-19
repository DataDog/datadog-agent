package main

import (
	"fmt"
	"log"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/cilium/ebpf"
)

func main() {
	var err error

	bc, err := bytecode.GetReader("./bytecode/build/co-re", "usm.o")
	if err != nil {
		log.Printf("couldn't find asset: %s", err)
		return
	}

	defer bc.Close()

	collectionSpec, err := ebpf.LoadCollectionSpecFromReader(bc)
	if err != nil {
		log.Println(err)
		return
	}

	for _, mapSpec := range collectionSpec.Maps {
		if mapSpec.MaxEntries == 0 {
			mapSpec.MaxEntries = 1
		}
	}

	opts := ebpf.CollectionOptions{
		Programs: ebpf.ProgramOptions{
			LogLevel: ebpf.LogLevelBranch | ebpf.LogLevelStats,
			LogSize:  100 * 1024 * 1024,
		},
	}
	collection, err := ebpf.NewCollectionWithOptions(collectionSpec, opts)
	if err != nil {
		log.Printf("Load collection: %v", err)
		return
	}

	progs := struct {
		Prog *ebpf.Program `ebpf:"socket__http2_eos_parser"`
	}{}
	err = collection.Assign(&progs)
	if err != nil {
		log.Println(err)
		return
	}

	vlog := progs.Prog.VerifierLog
	fmt.Printf("Verifier Log:\n%s\n", vlog)
}
