// Simple TLS server for misattribution reproduction test
package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
)

var (
	port     = flag.Int("port", 8443, "Port to listen on")
	certFile = flag.String("cert", "server.crt", "TLS certificate file")
	keyFile  = flag.String("key", "server.key", "TLS key file")
)

func main() {
	flag.Parse()

	serverName := fmt.Sprintf("server-%d", *port)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Return a response that identifies this server
		fmt.Fprintf(w, "Hello from %s! Path: %s\n", serverName, r.URL.Path)
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "OK")
	})

	// Unique path for each server to detect misattribution
	uniquePath := fmt.Sprintf("/server%d/identify", *port)
	http.HandleFunc(uniquePath, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "This is definitively %s\n", serverName)
	})

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Starting %s on %s", serverName, addr)

	server := &http.Server{
		Addr: addr,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	err := server.ListenAndServeTLS(*certFile, *keyFile)
	if err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
