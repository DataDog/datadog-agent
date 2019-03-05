// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build containerd

package containerd

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/namespaces"
)

const (
	// The check config is used if the containerd socket is detected.
	// However we want to cover cases with custom config files.
	containerdDefaultSocketPath = "/var/run/containerd/containerd.sock"
)

var (
	globalContainerdUtil *ContainerdUtil
	once                 sync.Once
)

// ContainerdItf is the interface implementing a subset of methods that leverage the Containerd api.
type ContainerdItf interface {
	GetEvents() containerd.EventService
	Containers() ([]containerd.Container, error)
	Metadata() (containerd.Version, error)
	Namespace() string
}

// ContainerdUtil is the util used to interact with the Containerd api.
type ContainerdUtil struct {
	cl                *containerd.Client
	socketPath        string
	initRetry         retry.Retrier
	queryTimeout      time.Duration
	connectionTimeout time.Duration
	namespace         string
}

// GetContainerdUtil creates the Containerd util containing the Containerd client and implementing the ContainerdItf
// Errors are handled in the retrier.
func GetContainerdUtil() (ContainerdItf, error) {
	once.Do(func() {

		globalContainerdUtil = &ContainerdUtil{
			queryTimeout:      config.Datadog.GetDuration("cri_query_timeout") * time.Second,
			connectionTimeout: config.Datadog.GetDuration("cri_connection_timeout") * time.Second,
			socketPath:        config.Datadog.GetString("cri_socket_path"),
			namespace:         config.Datadog.GetString("containerd_namespace"),
		}
		if globalContainerdUtil.socketPath == "" {
			log.Info("No socket path was specified, defaulting to /var/run/containerd/containerd.sock")
			globalContainerdUtil.socketPath = containerdDefaultSocketPath
		}
		// Initialize the client in the connect method
		globalContainerdUtil.initRetry.SetupRetrier(&retry.Config{
			Name:          "containerdutil",
			AttemptMethod: globalContainerdUtil.connect,
			Strategy:      retry.RetryCount,
			RetryCount:    10,
			RetryDelay:    30 * time.Second,
		})
	})
	if err := globalContainerdUtil.initRetry.TriggerRetry(); err != nil {
		log.Errorf("Containerd init error: %s", err.Error())
		return nil, err
	}
	return globalContainerdUtil, nil
}

func (c *ContainerdUtil) Namespace() string {
	return c.namespace
}

// Metadata is used to collect the version and revision of the Containerd API
func (c *ContainerdUtil) Metadata() (containerd.Version, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, c.namespace)
	return c.cl.Version(ctxNamespace)
}

// Close is used when done with a ContainerdUtil
func (c *ContainerdUtil) Close() error {
	if c.cl == nil {
		return log.Errorf("Containerd Client not initialized")
	}
	return c.cl.Close()
}

// connect is our retry strategy, it can be re-triggered when the check is running if we lose connectivity.
func (c *ContainerdUtil) connect() error {
	var err error
	if c.cl != nil {
		err = c.cl.Reconnect()
		if err != nil {
			log.Errorf("Could not reconnect to the containerd daemon: %v", err)
			return c.cl.Close() // Attempt to close connections to avoid overloading the GRPC
		}
		return nil
	}

	c.cl, err = containerd.New(c.socketPath, containerd.WithTimeout(c.connectionTimeout))
	if err != nil {
		return err
	}
	ver, err := c.Metadata()
	if err == nil {
		log.Infof("Connected to containerd - Version %s/%s", ver.Version, ver.Revision)
	}
	return err
}

// GetEvents interfaces with the containerd api to get the event service.
func (c *ContainerdUtil) GetEvents() containerd.EventService {
	return c.cl.EventService()
}

// Containers interfaces with the containerd api to get the list of Containers.
func (c *ContainerdUtil) Containers() ([]containerd.Container, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, c.namespace)
	return c.cl.Containers(ctxNamespace)
}
