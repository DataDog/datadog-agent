package main

import (
	"net"
	"os"
	"os/signal"
	"path"
	"syscall"
)

func main() {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGKILL, syscall.SIGTERM)

	socket := path.Join(os.TempDir(), "fake-memcached.sock")
	// Make sure the socket is removed
	os.Remove(socket)
	ln, _ := net.Listen("unix", socket)

	go func(ln net.Listener, c chan os.Signal) {
		<-sigc
		ln.Close()
		os.Exit(0)
	}(ln, sigc)

	for {
	}
}
