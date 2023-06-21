package main

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/proto/test2"
	"log"

	"google.golang.org/grpc"
)

func main() {
	conn, err := grpc.Dial("unix:///tmp/my_grpc.sock", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}

	client := test2.NewSystemProbeClient(conn)

	response, err := client.GetConnections(context.Background(), &test2.GetConnectionsRequest{ClientID: "1"})
	if err != nil {
		log.Fatalf("Failed to call get connections: %v", err)
	}

	res, err := response.Recv()
	if err != nil {
		log.Fatalf("Failed to get response: %v", err)
	}
	fmt.Printf("bla %d\n", res.Data)

}
