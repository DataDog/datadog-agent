// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// mutex protecting build process
var mux sync.Mutex
var cachedArtifactPath string

func OpenFromAnotherProcess(t *testing.T, paths ...string) *exec.Cmd {
	build(t)

	cmd := exec.Command(cachedArtifactPath, paths...)
	require.NoError(t, cmd.Start())

	t.Cleanup(func() {
		if cmd.Process == nil {
			return
		}
		_ = cmd.Process.Kill()
	})

	return cmd
}

func build(t *testing.T) {
	mux.Lock()
	defer mux.Unlock()

	if cachedArtifactPath != "" {
		return
	}

	tmpDir, err := os.MkdirTemp("", "build_directory")
	require.NoError(t, err)

	// Write go source to a file
	srcPath := filepath.Join(tmpDir, "source.go")
	os.WriteFile(srcPath, []byte(programSrc), 0755)

	// Compile program
	artifactPath := filepath.Join(tmpDir, "file_opener")
	cmd := exec.Command("go", "build", "-buildvcs=false", "-a", "-ldflags=-extldflags '-static'", "-o", artifactPath, srcPath)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "could not build file opener program: %s\noutput: %s", err, string(out))

	// If we reach this point then compilation succeeded and we update the
	// cached compilation artifact path
	cachedArtifactPath = artifactPath
}

const programSrc = `
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/exp/mmap"
)

func main() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan bool, 1)

	readers := make([]*mmap.ReaderAt, len(os.Args)-1)
	defer func() {
		for _, r := range readers {
			_ = r.Close()
		}
	}()
	for _, path := range os.Args[1:] {
		r, err := mmap.Open(path)
		if err != nil {
			panic(err)
		}
		readers = append(readers, r)
	}

	go func() {
		<-sigs
		done <- true
	}()

	fmt.Println("awaiting signal")
	<-done
	fmt.Println("exiting")
}
`
