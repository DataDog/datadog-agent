package main

import (
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/grpc"
	"sync"
)

func main() {
	server, err := grpc.NewServer("127.0.0.1:9008")
	if err != nil {
		panic(err)
	}
	server.Run()
	wg := sync.WaitGroup{}
	wg.Add(1)

	wg.Wait()
}
