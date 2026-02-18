// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ptracer holds ptracer related files
package ptracer

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/kraken-hpc/go-fork"
	"github.com/stretchr/testify/assert"
)

const fifoPath = "/tmp/.gotestfifo"

func init() {
	// register child for following fork
	fork.RegisterFunc("child", child)
	fork.Init()

	// create fifo to communicate between parent and child
	if _, err := os.Stat(fifoPath); os.IsNotExist(err) {
		err := syscall.Mkfifo(fifoPath, 0666)
		if err != nil {
			fmt.Println("Error creating named pipe:", err)
			os.Exit(1)
		}
	}
}

func child() {
	// Open the FIFO for writing
	fifo, err := os.OpenFile(fifoPath, os.O_WRONLY, os.ModeNamedPipe)
	if err != nil {
		os.Exit(1)
		return
	}
	defer fifo.Close()

	// Send PID to parent
	binary.Write(fifo, binary.LittleEndian, int32(os.Getpid()))

	// Child process: Listen for signals and notify parent
	sigs := make(chan os.Signal, 1)
	defer close(sigs)
	signal.Notify(sigs)
	defer signal.Reset()
	for sig := range sigs {
		_, err := fmt.Fprintf(fifo, "%v\n", sig)
		if err != nil {
			os.Exit(1)
		}
	}
	os.Exit(0)
}

func TestSignalForwarding(t *testing.T) {
	// fork to have a child to receive signals
	err := fork.Fork("child")
	if err != nil {
		t.Fatal(err)
	}

	// open the fifo for reading
	fifo, err := os.OpenFile(fifoPath, os.O_RDONLY, os.ModeNamedPipe)
	if err != nil {
		t.Fatal("Error opening FIFO for reading:", err)
		return
	}
	defer fifo.Close()

	// read child pid
	var childPID int32
	binary.Read(fifo, binary.LittleEndian, &childPID)

	// ensure child process is killed at test end
	defer syscall.Kill(int(childPID), syscall.SIGKILL)

	// start the forwarder
	startSignalForwarder(int(childPID))

	reader := bufio.NewReader(fifo)
	for _, sig := range forwardedSignals {
		t.Run(fmt.Sprintf("%v", sig), func(t *testing.T) {
			// send signal to ourselves
			unixSig, _ := sig.(syscall.Signal)
			syscall.Kill(os.Getpid(), unixSig)

			// wait for child response
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, fmt.Sprintf("%v\n", sig), line)
		})
	}

	t.Run("non-forwarded-signal", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		resultChan := make(chan string, 1)
		defer close(resultChan)

		go func() {
			if line, err := reader.ReadString('\n'); err == nil {
				resultChan <- line
			}
		}()

		// Send a non-forwarded signal
		syscall.Kill(os.Getpid(), syscall.SIGPIPE)

		select {
		case <-ctx.Done():
			return
		case line := <-resultChan:
			t.Errorf("Non-forwarded signal was received: %s", line)
			return
		}
	})
}
