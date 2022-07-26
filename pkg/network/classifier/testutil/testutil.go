package testutil

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TLSServer spins up a TCP server which accepts TLS encrypted connections
// The server accepts one connection at a time. In order to terminate the
// connection the client must send a string 'close\n'
func TLSServer(t *testing.T, addr string) func() {
	curDir, _ := curDir()
	crtPath := filepath.Join(curDir, "testdata/cert.pem.0")
	keyPath := filepath.Join(curDir, "testdata/server.key")
	cer, err := tls.LoadX509KeyPair(crtPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}

	config := &tls.Config{Certificates: []tls.Certificate{cer}, MaxVersion: tls.VersionTLS13, MinVersion: tls.VersionTLS10}
	ln, err := tls.Listen("tcp", addr, config)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		defer ln.Close()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			handleConnection(conn)
		}
	}()

	timeout := time.Now().Add(5 * time.Second)
	for {
		c, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true})
		if err == nil {
			c.Close()
			break
		}
		if time.Now().After(timeout) {
			break
		}
	}

	return func() { ln.Close() }
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	for {
		msg, err := r.ReadString('\n')
		if err != nil {
			break
		}
		if strings.Compare(strings.TrimRight(msg, "\n"), "close") == 0 {
			return
		}
	}
}

func curDir() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("unable to get current file build path")
	}

	buildDir := filepath.Dir(file)

	// build relative path from base of repo
	buildRoot := rootDir(buildDir)
	relPath, err := filepath.Rel(buildRoot, buildDir)
	if err != nil {
		return "", err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	curRoot := rootDir(cwd)

	return filepath.Join(curRoot, relPath), nil
}

// rootDir returns the base repository directory, just before `pkg`.
// If `pkg` is not found, the dir provided is returned.
func rootDir(dir string) string {
	pkgIndex := -1
	parts := strings.Split(dir, string(filepath.Separator))
	for i, d := range parts {
		if d == "pkg" {
			pkgIndex = i
			break
		}
	}
	if pkgIndex == -1 {
		return dir
	}
	return strings.Join(parts[:pkgIndex], string(filepath.Separator))
}
