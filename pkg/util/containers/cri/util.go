// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cri

package cri

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	criv1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	criv1alpha2 "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	"github.com/DataDog/datadog-agent/internal/third_party/kubernetes/pkg/kubelet/cri/remote/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

var (
	globalCRIUtil *CRIUtil
	once          sync.Once
)

// CRIClient abstracts the CRI client methods
type CRIClient interface {
	ListContainerStats() (map[string]*criv1.ContainerStats, error)
	GetContainerStats(containerID string) (*criv1.ContainerStats, error)
	GetRuntime() string
	GetRuntimeVersion() string
}

// CRIUtil wraps interactions with the CRI and implements CRIClient
// see https://github.com/kubernetes/kubernetes/blob/release-1.12/pkg/kubelet/apis/cri/runtime/v1alpha2/api.proto
type CRIUtil struct {
	// used to setup the CRIUtil
	initRetry retry.Retrier

	sync.Mutex
	clientV1          criv1.RuntimeServiceClient
	clientV1alpha2    criv1alpha2.RuntimeServiceClient
	runtime           string
	runtimeVersion    string
	queryTimeout      time.Duration
	connectionTimeout time.Duration
	socketPath        string
}

// init makes an empty CRIUtil bootstrap itself.
// This is not exposed as public API but is called by the retrier embed.
func (c *CRIUtil) init() error {
	if c.socketPath == "" {
		return fmt.Errorf("no cri_socket_path was set")
	}

	var protocol string
	if runtime.GOOS == "windows" {
		protocol = "npipe"
	} else {
		protocol = "unix"
	}

	_, dialer, err := util.GetAddressAndDialer(fmt.Sprintf("%s://%s", protocol, c.socketPath))
	if err != nil {
		return fmt.Errorf("failed to get dialer: %s", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.connectionTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, c.socketPath, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock(), grpc.WithContextDialer(dialer))
	if err != nil {
		return fmt.Errorf("failed to dial: %v", err)
	}

	var v *criv1.VersionResponse
	err = c.detectAPIVersion(conn)
	if err == nil {
		v, err = c.version()
	}

	if err != nil {
		// if detecting the API version fails, conn needs to be closed,
		// otherwise it'll leak as init may get retried.
		connErr := conn.Close()
		if connErr != nil {
			log.Debugf("failed to close gRPC connection: %s", connErr)
		}

		return err
	}

	c.runtime = v.RuntimeName
	c.runtimeVersion = v.RuntimeVersion
	log.Debugf("Successfully connected to CRI %s %s", c.runtime, c.runtimeVersion)

	return nil
}

// GetUtil returns a ready to use CRIUtil. It is backed by a shared singleton.
func GetUtil() (*CRIUtil, error) {
	once.Do(func() {
		globalCRIUtil = &CRIUtil{
			queryTimeout:      config.Datadog.GetDuration("cri_query_timeout") * time.Second,
			connectionTimeout: config.Datadog.GetDuration("cri_connection_timeout") * time.Second,
			socketPath:        config.Datadog.GetString("cri_socket_path"),
		}
		globalCRIUtil.initRetry.SetupRetrier(&retry.Config{ //nolint:errcheck
			Name:              "criutil",
			AttemptMethod:     globalCRIUtil.init,
			Strategy:          retry.Backoff,
			InitialRetryDelay: 1 * time.Second,
			MaxRetryDelay:     5 * time.Minute,
		})
	})

	if err := globalCRIUtil.initRetry.TriggerRetry(); err != nil {
		log.Debugf("CRI init error: %s", err)
		return nil, err
	}
	return globalCRIUtil, nil
}

// GetContainerStats returns the stats for the container with the given ID
func (c *CRIUtil) GetContainerStats(containerID string) (*criv1.ContainerStats, error) {
	stats, err := c.listContainerStatsWithFilter(&criv1.ContainerStatsFilter{Id: containerID})
	if err != nil {
		return nil, err
	}

	containerStats, found := stats[containerID]
	if !found {
		return nil, fmt.Errorf("could not get stats for container with ID %s ", containerID)
	}

	return containerStats, nil
}

// ListContainerStats sends a ListContainerStatsRequest to the server, and parses the returned response
func (c *CRIUtil) ListContainerStats() (map[string]*criv1.ContainerStats, error) {
	return c.listContainerStatsWithFilter(&criv1.ContainerStatsFilter{})
}

// GetRuntime returns the CRI runtime
func (c *CRIUtil) GetRuntime() string {
	return c.runtime
}

// GetRuntimeVersion returns the CRI runtime version
func (c *CRIUtil) GetRuntimeVersion() string {
	return c.runtimeVersion
}

func (c *CRIUtil) detectAPIVersion(conn *grpc.ClientConn) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.connectionTimeout)
	defer cancel()

	clientV1 := criv1.NewRuntimeServiceClient(conn)

	if _, err := clientV1.Version(ctx, &criv1.VersionRequest{}); err == nil {
		log.Info("Using CRI v1 API")
		c.clientV1 = clientV1
	} else if status.Code(err) == codes.Unimplemented {
		log.Info("Using CRI v1alpha2 API")
		c.clientV1alpha2 = criv1alpha2.NewRuntimeServiceClient(conn)
	} else {
		return err
	}

	return nil
}

func (c *CRIUtil) version() (*criv1.VersionResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()

	if c.clientV1 != nil {
		return c.clientV1.Version(ctx, &criv1.VersionRequest{})
	}

	v, err := c.clientV1alpha2.Version(ctx, &criv1alpha2.VersionRequest{})
	if err != nil {
		return nil, err
	}

	return fromV1alpha2VersionResponse(v), nil
}

func (c *CRIUtil) listContainerStatsWithFilter(filter *criv1.ContainerStatsFilter) (map[string]*criv1.ContainerStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()

	var r *criv1.ListContainerStatsResponse
	var err error

	if c.clientV1 != nil {
		r, err = c.clientV1.ListContainerStats(ctx, &criv1.ListContainerStatsRequest{Filter: filter})
	} else {
		var rv1alpha2 *criv1alpha2.ListContainerStatsResponse
		rv1alpha2, err = c.clientV1alpha2.ListContainerStats(ctx, &criv1alpha2.ListContainerStatsRequest{Filter: v1alpha2ContainerStatsFilter(filter)})
		if err == nil {
			r = fromV1alpha2ListContainerStatsResponse(rv1alpha2)
		}
	}

	if err != nil {
		return nil, err
	}

	stats := make(map[string]*criv1.ContainerStats)
	for _, s := range r.GetStats() {
		stats[s.Attributes.Id] = s
	}
	return stats, nil
}
