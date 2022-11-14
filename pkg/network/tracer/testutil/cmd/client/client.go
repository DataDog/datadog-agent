package main

import (
	"context"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/grpc"
)

func main() {
	c, err := grpc.NewClient("127.0.0.1:9008", grpc.Options{})
	if err != nil {
		panic(err)
	}
	defer c.Close()
	if err := c.HandleUnary(context.Background(), "test"); err != nil {
		panic(err)
	}
}
