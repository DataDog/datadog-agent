// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main is a simple utility to run tcp client requests.
package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("usage: tcp_client <server_address> <client_address> <rst>")
		os.Exit(1)
	}

	serverPath := os.Args[1]
	clientPath := os.Args[2]
	useRST, _ := strconv.ParseBool(os.Args[3])

	localAddr, err := net.ResolveTCPAddr("tcp", clientPath)
	if err != nil {
		fmt.Println("resolve client address:", clientPath, " error:", err)
		os.Exit(1)
	}

	serverAddr, err := net.ResolveTCPAddr("tcp", serverPath)
	if err != nil {
		fmt.Println("resolve server address:", serverPath, " error:", err)
		os.Exit(1)
	}

	dialer := net.Dialer{
		LocalAddr: localAddr,
		Timeout:   50 * time.Millisecond,
	}
	conn, err := dialer.Dial("tcp", serverAddr.String())
	if err != nil {
		fmt.Println("connecting server error:", err)
		os.Exit(1)
	}
	defer conn.Close()

	_, err = conn.Write([]byte("some"))
	if err != nil {
		fmt.Println("sending message error:", err)
		os.Exit(1)
	}

	if useRST {
		// closed connection issues RST instead of FIN
		fd, _ := conn.(*net.TCPConn).File()
		syscall.SetsockoptLinger(int(fd.Fd()), syscall.SOL_SOCKET, syscall.SO_LINGER, &syscall.Linger{Onoff: 1, Linger: 0})
	}
}
