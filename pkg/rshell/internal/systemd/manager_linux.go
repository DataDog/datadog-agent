// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package systemd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"
)

const (
	managerOperationTimeout = 30 * time.Second
	managerBusFDDir         = "/proc/self/fd"
)

type dbusManagerBus struct {
	connection    *dbus.Conn
	signalHandler *boundedManagerSignalHandler
}

func (c *Client) ListSystemServices(ctx context.Context, request builtins.SystemServiceListRequest) ([]builtins.SystemServiceState, error) {
	if err := validateManagerUnits(request.Services, true); err != nil {
		return nil, err
	}
	if len(request.Services) == 0 {
		return []builtins.SystemServiceState{}, nil
	}
	var states []builtins.SystemServiceState
	err := c.withManagerBus(ctx, func(ctx context.Context, bus *dbusManagerBus) error {
		var err error
		states, err = listSystemServicesWithBus(ctx, bus, request)
		return err
	})
	return states, err
}

func (c *Client) InspectSystemServices(ctx context.Context, units []string) ([]builtins.SystemServiceState, error) {
	if err := validateManagerUnits(units, false); err != nil {
		return nil, err
	}
	var states []builtins.SystemServiceState
	err := c.withManagerBus(ctx, func(ctx context.Context, bus *dbusManagerBus) error {
		var err error
		states, err = inspectSystemServicesWithBus(ctx, bus, units)
		return err
	})
	return states, err
}

func (c *Client) RunSystemServiceJobs(ctx context.Context, action builtins.SystemServiceJobAction, units []string) error {
	if err := validateManagerUnits(units, false); err != nil {
		return err
	}
	if _, ok := managerJobMethod(action); !ok {
		return fmt.Errorf("unsupported systemd manager job action %q", action)
	}
	return c.withManagerBus(ctx, func(ctx context.Context, bus *dbusManagerBus) error {
		return runSystemServiceJobsWithBus(ctx, bus, action, units)
	})
}

func (c *Client) EnableSystemServices(ctx context.Context, units []string) error {
	if err := validateManagerUnits(units, false); err != nil {
		return err
	}
	return c.withManagerBus(ctx, func(ctx context.Context, bus *dbusManagerBus) error {
		return enableSystemServicesWithBus(ctx, bus, units)
	})
}

func (c *Client) DisableSystemServices(ctx context.Context, units []string) error {
	if err := validateManagerUnits(units, false); err != nil {
		return err
	}
	return c.withManagerBus(ctx, func(ctx context.Context, bus *dbusManagerBus) error {
		return disableSystemServicesWithBus(ctx, bus, units)
	})
}

func (c *Client) withManagerBus(ctx context.Context, operation func(context.Context, *dbusManagerBus) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	operationCtx, cancel := context.WithTimeout(ctx, managerOperationTimeout)
	defer cancel()
	bus, err := c.openManagerBus(operationCtx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			return fmt.Errorf("systemd manager operation timed out after %s: %w", managerOperationTimeout, err)
		}
		return err
	}
	defer bus.connection.Close()
	if err := operation(operationCtx, bus); err != nil {
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			return fmt.Errorf("systemd manager operation timed out after %s: %w", managerOperationTimeout, err)
		}
		return err
	}
	return nil
}

func (c *Client) openManagerBus(ctx context.Context) (*dbusManagerBus, error) {
	if c.target.MachineIDPath == "" {
		return nil, errors.New("systemd target machine ID path is unavailable")
	}
	expectedMachineID, err := c.readMachineID()
	if err != nil {
		return nil, fmt.Errorf("validate systemd target machine ID: %w", err)
	}
	if c.target.ManagerBusSocket == "" {
		return nil, errors.New("systemd target manager bus socket is unavailable")
	}

	networkConnection, err := c.dialManagerBus(ctx, c.target.ManagerBusSocket)
	if err != nil {
		return nil, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		if err := networkConnection.SetDeadline(deadline); err != nil {
			networkConnection.Close()
			return nil, fmt.Errorf("set systemd manager bus deadline: %w", err)
		}
	}
	bounded := &boundedDBusConn{ReadWriteCloser: networkConnection}
	signalHandler := newBoundedManagerSignalHandler()
	connection, err := dbus.NewConn(
		bounded,
		dbus.WithContext(ctx),
		dbus.WithSignalHandler(signalHandler),
		dbus.WithIncomingInterceptor(rejectManagerInboundMethodCalls(bounded)),
	)
	if err != nil {
		networkConnection.Close()
		return nil, fmt.Errorf("create systemd manager D-Bus connection: %w", err)
	}
	closeOnError := func(err error) (*dbusManagerBus, error) {
		connection.Close()
		if contextErr := contextNetworkError(ctx, err); contextErr != nil {
			return nil, contextErr
		}
		// The connection deadline above is set to ctx.Deadline(), so any I/O
		// timeout on it is inherently a deadline-exceeded condition even if
		// ctx.Err() hasn't observably transitioned yet (its cancellation
		// timer and the socket's kernel deadline are independent clocks that
		// can fire in either order under scheduler pressure).
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, context.DeadlineExceeded
		}
		return nil, err
	}
	if err := connection.Auth([]dbus.Auth{dbus.AuthExternal(strconv.Itoa(os.Geteuid()))}); err != nil {
		return closeOnError(fmt.Errorf("authenticate systemd manager D-Bus connection: %w", err))
	}
	if err := connection.Hello(); err != nil {
		return closeOnError(fmt.Errorf("initialize systemd manager D-Bus connection: %w", err))
	}

	bus := &dbusManagerBus{connection: connection, signalHandler: signalHandler}
	if err := verifySystemdManagerMachineID(ctx, bus, expectedMachineID); err != nil {
		return closeOnError(err)
	}
	return bus, nil
}

func (c *Client) dialManagerBus(ctx context.Context, path string) (net.Conn, error) {
	socket, err := c.openManagerBusSocket(path)
	if err != nil {
		return nil, err
	}
	return dialPinnedManagerBus(ctx, socket)
}

func (c *Client) openManagerBusSocket(path string) (*os.File, error) {
	socket, err := c.openTargetFileFlags(path, unix.O_PATH|unix.O_NOFOLLOW)
	if err != nil {
		return nil, fmt.Errorf("inspect systemd manager bus socket: %w", err)
	}
	info, err := socket.Stat()
	if err != nil {
		_ = socket.Close()
		return nil, fmt.Errorf("inspect systemd manager bus socket: %w", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		_ = socket.Close()
		return nil, errors.New("systemd manager bus endpoint is not a Unix socket")
	}
	return socket, nil
}

// dialPinnedManagerBus takes ownership of socket and closes it before
// returning. Reopening the pinned descriptor prevents later pathname changes
// from redirecting the connection.
func dialPinnedManagerBus(ctx context.Context, socket *os.File) (net.Conn, error) {
	defer socket.Close()
	endpoint := filepath.Join(managerBusFDDir, strconv.Itoa(int(socket.Fd())))
	var dialer net.Dialer
	connection, err := dialer.DialContext(ctx, "unix", endpoint)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("connect to pinned systemd manager bus socket: %w", err)
	}
	return connection, nil
}

func (b *dbusManagerBus) call(ctx context.Context, destination string, path dbus.ObjectPath, method string, arguments ...any) ([]any, error) {
	call := b.connection.Object(destination, path).CallWithContext(ctx, method, dbus.FlagNoAutoStart, arguments...)
	if call.Err != nil {
		return nil, call.Err
	}
	return call.Body, nil
}

func (b *dbusManagerBus) addJobRemovedMatch(ctx context.Context) error {
	return b.connection.AddMatchSignalContext(ctx,
		dbus.WithMatchSender(systemdBusDestination),
		dbus.WithMatchObjectPath(systemdManagerPath),
		dbus.WithMatchInterface(systemdManagerIface),
		dbus.WithMatchMember("JobRemoved"),
	)
}

func (b *dbusManagerBus) registerSignals(channel chan<- *dbus.Signal) <-chan struct{} {
	b.connection.Signal(channel)
	return b.signalHandler.Overflow()
}

func (b *dbusManagerBus) removeSignals(channel chan<- *dbus.Signal) {
	b.connection.RemoveSignal(channel)
}

func (*Client) openTargetFileFlags(path string, flag int) (*os.File, error) {
	return os.OpenFile(path, flag, 0)
}
