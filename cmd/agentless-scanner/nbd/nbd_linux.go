// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build linux

// Package nbd defines the Network Block Device and provides the functionality
// to start and stop the NBD server and client.
package nbd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path"
	"sync"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/types"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/jinroh/go-nbd/pkg/backend"
	"github.com/jinroh/go-nbd/pkg/server"
)

var (
	nbds   = make(map[string]*nbd)
	nbdsMu sync.Mutex
)

type nbd struct {
	b          backend.Backend
	scan       *types.ScanTask
	deviceName string
	srv        net.Listener
	closed     chan struct{}
	closing    chan struct{}
}

// StartNBDBlockDevice starts the NBD server and client for the given device
// name with the provided backend.
func StartNBDBlockDevice(scan *types.ScanTask, deviceName string, b backend.Backend) error {
	bd := &nbd{
		scan:       scan,
		b:          b,
		deviceName: deviceName,
		closed:     make(chan struct{}),
		closing:    make(chan struct{}),
	}
	nbdsMu.Lock()
	if _, ok := nbds[bd.deviceName]; ok {
		nbdsMu.Unlock()
		return fmt.Errorf("nbd: already running nbd server for device %q", bd.deviceName)
	}
	nbds[bd.deviceName] = bd
	nbdsMu.Unlock()

	_, err := os.Stat(bd.deviceName)
	if err != nil {
		return fmt.Errorf("nbd: could not stat device %q: %w", bd.deviceName, err)
	}
	if err := bd.startServer(); err != nil {
		return err
	}
	if err := bd.startClient(); err != nil {
		return err
	}
	return nil
}

// StopNBDBlockDevice stops the NBD server and client for the given device name.
func StopNBDBlockDevice(ctx context.Context, deviceName string) {
	nbdsMu.Lock()
	bd, ok := nbds[deviceName]
	delete(nbds, deviceName)
	nbdsMu.Unlock()

	if !ok {
		log.Debugf("nbdclient: disconnecting unknown client for device %q", deviceName)
		if err := exec.CommandContext(ctx, "nbd-client", "-d", deviceName).Run(); err != nil {
			log.Errorf("nbd-client: %q disconnecting failed: %v", deviceName, err)
		} else {
			log.Tracef("nbd-client: %q disconnected", deviceName)
		}
		return
	}

	log.Debugf("nbdclient: disconnecting client for device %q", deviceName)
	if err := exec.CommandContext(ctx, "nbd-client", "-d", deviceName).Run(); err != nil {
		log.Errorf("%s: nbd-client: %q disconnecting failed: %v", bd.scan, deviceName, err)
	} else {
		log.Tracef("%s: nbd-client: %q disconnected", bd.scan, deviceName)
	}
	if err := bd.waitServerClosed(ctx); err != nil {
		log.Errorf("%s: nbdserver: %q could not close: %v", bd.scan, deviceName, err)
	}
	return
}

func (bd *nbd) String() string {
	return fmt.Sprintf("nbdserver: %q", bd.deviceName)
}

func (bd *nbd) getSocketAddr() string {
	return fmt.Sprintf("/tmp/nbd-%s-%s.sock", bd.scan.ID, path.Base(bd.deviceName))
}

func (bd *nbd) startClient() error {
	_, err := exec.LookPath("nbd-client")
	if err != nil {
		return fmt.Errorf("nbd: could not locate 'nbd-client' util binary in PATH: %w", err)
	}
	addr := bd.getSocketAddr()
	cmd := exec.CommandContext(context.Background(), "nbd-client",
		"-readonly",
		"-unix", addr, bd.deviceName,
		"-name", bd.scan.ID,
		"-connections", "5")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("nbd-client: %q failed: %s", bd.deviceName, string(out))
		return err
	}
	return nil
}

func (bd *nbd) startServer() (err error) {
	defer func() {
		if err != nil {
			close(bd.closed)
		}
	}()

	addr := bd.getSocketAddr()
	if _, err := os.Stat(addr); err == nil {
		return fmt.Errorf("nbd: socket %q already exists", addr)
	}

	bd.srv, err = net.Listen("unix", addr)
	if err != nil {
		return fmt.Errorf("nbd: could not list to %q: %w", addr, err)
	}
	if err := os.Chmod(addr, 0700); err != nil {
		return fmt.Errorf("nbd: could not chmod %q: %w", addr, err)
	}

	conns := make(map[net.Conn]struct{})
	addConn := make(chan net.Conn)
	rmvConn := make(chan net.Conn)

	go func() {
		for {
			conn, err := bd.srv.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				log.Warnf("nbdserver: %q could not accept connection: %v", bd.deviceName, err)
			} else {
				log.Tracef("nbdserver: client connected")
				addConn <- conn
			}
		}
	}()

	log.Debugf("nbdserver: %q accepting connections on %q", bd.deviceName, addr)
	go func() {
		defer func() {
			log.Debugf("nbdserver: %q closed successfully", bd.deviceName)
			bd.srv.Close()
			close(bd.closed)
		}()
		for {
			select {
			case conn := <-addConn:
				conns[conn] = struct{}{}
				go func() {
					bd.serverHandleConn(conn, bd.b)
					rmvConn <- conn
				}()

			case conn := <-rmvConn:
				log.Tracef("nbdserver: %q client disconnected", bd.deviceName)
				delete(conns, conn)
				conn.Close()
				if len(conns) == 0 {
					return
				}

			case <-bd.closing:
				if len(conns) == 0 {
					return
				}
			}
		}
	}()
	return nil
}

func (bd *nbd) waitServerClosed(ctx context.Context) error {
	close(bd.closing)
	select {
	case <-bd.closed:
		return nil
	case <-ctx.Done():
		log.Warnf("nbdserver: %q forced to close", bd.deviceName)
		if bd.srv != nil {
			bd.srv.Close() // forcing close of server
		}
	}
	return ctx.Err()
}

func (bd *nbd) serverHandleConn(conn net.Conn, backend backend.Backend) {
	log.Tracef("nbdserver: %q client connected ", bd.deviceName)
	err := server.Handle(conn,
		[]*server.Export{
			{
				Name:    bd.scan.ID,
				Backend: backend,
			},
		},
		&server.Options{
			ReadOnly:           true,
			MinimumBlockSize:   1,
			PreferredBlockSize: 4096,
			MaximumBlockSize:   0xffffffff,
			SupportsMultiConn:  true,
		})
	if err != nil {
		if !errors.Is(err, net.ErrClosed) {
			log.Errorf("nbdserver: %q could not handle new connection: %v", bd.deviceName, err)
		}
	}
}
