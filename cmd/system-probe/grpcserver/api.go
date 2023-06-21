package grpcserver

import (
	"github.com/DataDog/datadog-agent/pkg/proto/test2"
	"google.golang.org/grpc"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
)

func StartgRPCServer() error {
	socketFile := "/tmp/my_grpc.sock"

	// Remove existing socket file if it exists
	os.Remove(socketFile)

	listener, err := net.Listen("unix", socketFile)
	if err != nil {
		return err
	}

	server := grpc.NewServer()
	test2.RegisterSystemProbeServer(server, &SystemProbeServer{})

	go func() {
		log.Println("gRPC server listening on Unix domain socket...")

		err = server.Serve(listener)
		if err != nil {
			return
		}
	}()

	// Wait for interrupt signal to gracefully stop the server
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	// Gracefully stop the server
	server.GracefulStop()
	return nil
}
