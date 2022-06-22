// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd
// +build containerd

package containerd

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	dderrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/api/types"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
)

const (
	// The check config is used if the containerd socket is detected.
	// However we want to cover cases with custom config files.
	containerdDefaultSocketPath = "/var/run/containerd/containerd.sock"
)

// ContainerdItf is the interface implementing a subset of methods that leverage the Containerd api.
type ContainerdItf interface {
	Close() error
	CheckConnectivity() *retry.Error
	Container(id string) (containerd.Container, error)
	ContainerWithContext(ctx context.Context, id string) (containerd.Container, error)
	Containers() ([]containerd.Container, error)
	EnvVars(ctn containerd.Container) (map[string]string, error)
	GetEvents() containerd.EventService
	Info(ctn containerd.Container) (containers.Container, error)
	Labels(ctn containerd.Container) (map[string]string, error)
	LabelsWithContext(ctx context.Context, ctn containerd.Container) (map[string]string, error)
	ListImages() ([]containerd.Image, error)
	Image(ctn containerd.Container) (containerd.Image, error)
	ImageSize(ctn containerd.Container) (int64, error)
	Spec(ctn containerd.Container) (*oci.Spec, error)
	SpecWithContext(ctx context.Context, ctn containerd.Container) (*oci.Spec, error)
	Metadata() (containerd.Version, error)
	CurrentNamespace() string
	SetCurrentNamespace(namespace string)
	Namespaces(ctx context.Context) ([]string, error)
	TaskMetrics(ctn containerd.Container) (*types.Metric, error)
	TaskPids(ctn containerd.Container) ([]containerd.ProcessInfo, error)
	Status(ctn containerd.Container) (containerd.ProcessStatus, error)
	CallWithClientContext(f func(context.Context) error) error
	Annotations(ctn containerd.Container) (map[string]string, error)
	IsSandbox(ctn containerd.Container) (bool, error)
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

// NewContainerdUtil creates the Containerd util containing the Containerd client and implementing the ContainerdItf
// Errors are handled in the retrier.
func NewContainerdUtil() (ContainerdItf, error) {
	// A singleton does not work because different parts of the code
	// (workloadmeta, checks, etc.) might need to fetch info from different
	// namespaces at the same time.
	containerdUtil := &ContainerdUtil{
		queryTimeout:      config.Datadog.GetDuration("cri_query_timeout") * time.Second,
		connectionTimeout: config.Datadog.GetDuration("cri_connection_timeout") * time.Second,
		socketPath:        config.Datadog.GetString("cri_socket_path"),
	}
	if containerdUtil.socketPath == "" {
		log.Info("No socket path was specified, defaulting to /var/run/containerd/containerd.sock")
		containerdUtil.socketPath = containerdDefaultSocketPath
	}
	// Initialize the client in the connect method
	containerdUtil.initRetry.SetupRetrier(&retry.Config{ //nolint:errcheck
		Name:              "containerdutil",
		AttemptMethod:     containerdUtil.connect,
		Strategy:          retry.Backoff,
		InitialRetryDelay: 1 * time.Second,
		MaxRetryDelay:     5 * time.Minute,
	})

	if err := containerdUtil.CheckConnectivity(); err != nil {
		log.Errorf("Containerd init error: %s", err.Error())
		return nil, err
	}

	return containerdUtil, nil
}

// CheckConnectivity tries to connect to containerd api
func (c *ContainerdUtil) CheckConnectivity() *retry.Error {
	return c.initRetry.TriggerRetry()
}

// CurrentNamespace returns the current containerd namespace
func (c *ContainerdUtil) CurrentNamespace() string {
	return c.namespace
}

// SetCurrentNamespace sets the current containerd namespace
func (c *ContainerdUtil) SetCurrentNamespace(namespace string) {
	c.namespace = namespace
}

// Namespaces lists the containerd namespaces
func (c *ContainerdUtil) Namespaces(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.queryTimeout)
	defer cancel()
	return c.cl.NamespaceService().List(ctx)
}

// CallWithClientContext allows passing an additional context when calling the containerd api
func (c *ContainerdUtil) CallWithClientContext(f func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, c.namespace)

	return f(ctxNamespace)
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

// Container interfaces with the containerd api to get a container by ID.
func (c *ContainerdUtil) Container(id string) (containerd.Container, error) {
	return c.ContainerWithContext(context.Background(), id)
}

// ContainerWithContext interfaces with the containerd api to get a container by ID.
// It allows passing the parent context
func (c *ContainerdUtil) ContainerWithContext(ctx context.Context, id string) (containerd.Container, error) {
	ctx, cancel := context.WithTimeout(ctx, c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, c.namespace)
	ctn, err := c.cl.LoadContainer(ctxNamespace, id)
	if errdefs.IsNotFound(err) {
		return ctn, dderrors.NewNotFound(id)
	}

	return ctn, err
}

// Containers interfaces with the containerd api to get the list of Containers.
func (c *ContainerdUtil) Containers() ([]containerd.Container, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, c.namespace)
	return c.cl.Containers(ctxNamespace)
}

// EnvVars returns the env variables of a containerd container
func (c *ContainerdUtil) EnvVars(ctn containerd.Container) (map[string]string, error) {
	spec, err := c.Spec(ctn)
	if err != nil {
		return nil, err
	}

	envs := make(map[string]string)

	for _, env := range spec.Process.Env {
		envSplit := strings.SplitN(env, "=", 2)

		if len(envSplit) < 2 {
			return nil, errors.New("unexpected environment variable format")
		}

		envs[envSplit[0]] = envSplit[1]
	}

	return envs, nil
}

// ListImages interfaces with the containerd api to list image
func (c *ContainerdUtil) ListImages() ([]containerd.Image, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, c.namespace)

	return c.cl.ListImages(ctxNamespace)
}

// Image interfaces with the containerd api to get an image
func (c *ContainerdUtil) Image(ctn containerd.Container) (containerd.Image, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, c.namespace)

	return ctn.Image(ctxNamespace)
}

// ImageSize interfaces with the containerd api to get the size of an image
func (c *ContainerdUtil) ImageSize(ctn containerd.Container) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, c.namespace)

	img, err := ctn.Image(ctxNamespace)
	if err != nil {
		return 0, err
	}
	return img.Size(ctxNamespace)
}

// Info interfaces with the containerd api to get Container info
func (c *ContainerdUtil) Info(ctn containerd.Container) (containers.Container, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, c.namespace)

	return ctn.Info(ctxNamespace)
}

// Labels interfaces with the containerd api to get Container labels
func (c *ContainerdUtil) Labels(ctn containerd.Container) (map[string]string, error) {
	return c.LabelsWithContext(context.Background(), ctn)
}

// LabelsWithContext interfaces with the containerd api to get Container labels
// It allows passing the parent context
func (c *ContainerdUtil) LabelsWithContext(ctx context.Context, ctn containerd.Container) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, c.namespace)

	return ctn.Labels(ctxNamespace)
}

// Spec interfaces with the containerd api to get container OCI Spec
func (c *ContainerdUtil) Spec(ctn containerd.Container) (*oci.Spec, error) {
	return c.SpecWithContext(context.Background(), ctn)
}

// SpecWithContext interfaces with the containerd api to get container OCI Spec
// It allows passing the parent context
func (c *ContainerdUtil) SpecWithContext(ctx context.Context, ctn containerd.Container) (*oci.Spec, error) {
	ctx, cancel := context.WithTimeout(ctx, c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, c.namespace)

	return ctn.Spec(ctxNamespace)
}

// TaskMetrics interfaces with the containerd api to get the metrics from a container
func (c *ContainerdUtil) TaskMetrics(ctn containerd.Container) (*types.Metric, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, c.namespace)

	t, errTask := ctn.Task(ctxNamespace, nil)
	if errTask != nil {
		return nil, errTask
	}

	return t.Metrics(ctxNamespace)
}

// TaskPids interfaces with the containerd api to get the pids from a container
func (c *ContainerdUtil) TaskPids(ctn containerd.Container) ([]containerd.ProcessInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, c.namespace)

	t, errTask := ctn.Task(ctxNamespace, nil)
	if errTask != nil {
		return nil, errTask
	}

	return t.Pids(ctxNamespace)
}

// Status interfaces with the containerd api to get the status for a container
func (c *ContainerdUtil) Status(ctn containerd.Container) (containerd.ProcessStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, c.namespace)

	task, err := ctn.Task(ctxNamespace, nil)
	if err != nil {
		return "", err
	}

	ctx, cancel = context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()
	ctxNamespace = namespaces.WithNamespace(ctx, c.namespace)

	taskStatus, err := task.Status(ctxNamespace)
	if err != nil {
		return "", err
	}

	return taskStatus.Status, nil
}

// Annotations returns the container annotations from its spec
func (c *ContainerdUtil) Annotations(ctn containerd.Container) (map[string]string, error) {
	spec, err := c.Spec(ctn)
	if err != nil {
		return nil, err
	}

	return spec.Annotations, nil
}

// IsSandbox returns whether a container is a sandbox (a.k.a pause container).
// It checks the io.cri-containerd.kind label and the io.kubernetes.cri.container-type annotation.
// Ref:
// - https://github.com/containerd/cri/blob/release/1.4/pkg/server/helpers.go#L74
// - https://github.com/containerd/cri/blob/release/1.4/pkg/annotations/annotations.go#L30
func (c *ContainerdUtil) IsSandbox(ctn containerd.Container) (bool, error) {
	labels, err := c.Labels(ctn)
	if err != nil {
		return false, err
	}

	if labels["io.cri-containerd.kind"] == "sandbox" {
		return true, nil
	}

	annotations, err := c.Annotations(ctn)
	if err != nil {
		return false, err
	}

	return annotations["io.kubernetes.cri.container-type"] == "sandbox", nil
}
