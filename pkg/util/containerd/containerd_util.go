// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build containerd

package containerd

import (
	"github.com/containerd/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"time"
	"context"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type ContainerdItf interface {
	GetEvents() containerd.EventService
	EnsureServing(ctx context.Context) error
	GetNamespaces(ctx context.Context) ([]string, error)
	Containers(ctx context.Context) ([]containerd.Container, error)
}

type ContainerdUtil struct {
	cl *containerd.Client
	initRetry retry.Retrier
}

// InstanciateContainerdUtil creates the containerd util containing the containerd client and implementing the ContainerdItf
// Errors are handled in the retrier.
func InstanciateContainerdUtil() ContainerdItf {
	log.Infof("[DEV] Calling Containerd init")
	util := &ContainerdUtil{}
	// Initialize the client in the connect method
	util.initRetry.SetupRetrier(&retry.Config{
		Name:   "containerdutil",
		AttemptMethod:  util.connect,
		Strategy:   retry.RetryCount,
		RetryCount: 10,
		RetryDelay: 30 * time.Second,
	})
	return util
}

// reconnect is our retry strategy, it can be retriggered when the check is running if we lose connectivity.
func (c *ContainerdUtil) connect() error {
	var err error
	log.Infof("[DEV] Calling Containerd connect", c.initRetry.RetryStatus())
	if c.cl != nil {
		err = c.cl.Reconnect()
		if err != nil {
			log.Errorf("Could not reconnect to the containerd daemon: %v", err)
			return c.cl.Close() // Attempt to close connections to avoid overloading the GRPC
		}
		log.Infof("returning here 1")
		return nil
	}
	// If we lose the connection, let's reset the state including the Dial options
	c.cl, err = containerd.New("/run/containerd/containerd.sock")
	log.Infof("[DEV] client is %#v and error is %s", c.cl, err)
	return err
}

// EnsureServing checks if the containerd daemon is healthy and tries to reconnect if need be.
func (c * ContainerdUtil) EnsureServing(ctx context.Context) error {
	log.Infof("[DEV] ensuring serving %#v", c)
	if c.cl != nil {
		//  Check if the current client is healthy
		s, err := c.cl.IsServing(ctx)
		log.Infof("[DEV] is serving %v err is %v", s, err)
		if s {
			return nil
		}
		log.Errorf("Current client is not responding: %v", err)
	}
	err := c.initRetry.TriggerRetry()
	if err != nil {
		log.Errorf("Can't connect to containerd, will retry later: %v", err)
		return err
	}
	return nil
}


func (c *ContainerdUtil) GetEvents() containerd.EventService {
	return c.cl.EventService()
}

func (c *ContainerdUtil) GetNamespaces(ctx context.Context) ([]string, error) {
	return c.cl.NamespaceService().List(ctx)
}

func (c *ContainerdUtil) Containers(ctx context.Context) ([]containerd.Container, error) {
	return c.cl.Containers(ctx)
}
