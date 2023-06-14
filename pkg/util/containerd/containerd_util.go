// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd

package containerd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	dderrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"github.com/opencontainers/image-spec/identity"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/api/types"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/leases"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
)

const (
	// The check config is used if the containerd socket is detected.
	// However we want to cover cases with custom config files.
	containerdDefaultSocketPath = "/var/run/containerd/containerd.sock"

	// DefaultAllowedSpecMaxSize is the recommended maxSize for Spec parsing
	DefaultAllowedSpecMaxSize = 2 * 1024 * 1024
)

// ErrSpecTooLarge is returned when container Spec is too large
var ErrSpecTooLarge = errors.New("Container spec is too large")

// ContainerdItf is the interface implementing a subset of methods that leverage the Containerd api.
type ContainerdItf interface {
	RawClient() *containerd.Client
	Close() error
	CheckConnectivity() *retry.Error
	Container(namespace string, id string) (containerd.Container, error)
	ContainerWithContext(ctx context.Context, namespace string, id string) (containerd.Container, error)
	Containers(namespace string) ([]containerd.Container, error)
	GetEvents() containerd.EventService
	Info(namespace string, ctn containerd.Container) (containers.Container, error)
	Labels(namespace string, ctn containerd.Container) (map[string]string, error)
	LabelsWithContext(ctx context.Context, namespace string, ctn containerd.Container) (map[string]string, error)
	ListImages(namespace string) ([]containerd.Image, error)
	Image(namespace string, name string) (containerd.Image, error)
	ImageOfContainer(namespace string, ctn containerd.Container) (containerd.Image, error)
	ImageSize(namespace string, ctn containerd.Container) (int64, error)
	Spec(namespace string, ctn containers.Container, maxSize int) (*oci.Spec, error)
	Metadata() (containerd.Version, error)
	Namespaces(ctx context.Context) ([]string, error)
	TaskMetrics(namespace string, ctn containerd.Container) (*types.Metric, error)
	TaskPids(namespace string, ctn containerd.Container) ([]containerd.ProcessInfo, error)
	Status(namespace string, ctn containerd.Container) (containerd.ProcessStatus, error)
	CallWithClientContext(namespace string, f func(context.Context) error) error
	IsSandbox(namespace string, ctn containerd.Container) (bool, error)
	MountImage(ctx context.Context, expiration time.Duration, namespace string, img containerd.Image, targetDir string) (func(context.Context) error, error)
}

// ContainerdUtil is the util used to interact with the Containerd api.
type ContainerdUtil struct {
	cl                *containerd.Client
	socketPath        string
	initRetry         retry.Retrier
	queryTimeout      time.Duration
	connectionTimeout time.Duration
}

type ContainerdAccessor func() (ContainerdItf, error)

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
func (c *ContainerdUtil) RawClient() *containerd.Client {
	return c.cl
}

// CheckConnectivity tries to connect to containerd api
func (c *ContainerdUtil) CheckConnectivity() *retry.Error {
	return c.initRetry.TriggerRetry()
}

// Namespaces lists the containerd namespaces
func (c *ContainerdUtil) Namespaces(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.queryTimeout)
	defer cancel()
	return c.cl.NamespaceService().List(ctx)
}

// CallWithClientContext allows passing an additional context when calling the containerd api
func (c *ContainerdUtil) CallWithClientContext(namespace string, f func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, namespace)

	return f(ctxNamespace)
}

// Metadata is used to collect the version and revision of the Containerd API
func (c *ContainerdUtil) Metadata() (containerd.Version, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()
	return c.cl.Version(ctx)
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
func (c *ContainerdUtil) Container(namespace string, id string) (containerd.Container, error) {
	return c.ContainerWithContext(context.Background(), namespace, id)
}

// ContainerWithContext interfaces with the containerd api to get a container by ID.
// It allows passing the parent context
func (c *ContainerdUtil) ContainerWithContext(ctx context.Context, namespace string, id string) (containerd.Container, error) {
	ctx, cancel := context.WithTimeout(ctx, c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, namespace)
	ctn, err := c.cl.LoadContainer(ctxNamespace, id)
	if errdefs.IsNotFound(err) {
		return ctn, dderrors.NewNotFound(id)
	}

	return ctn, err
}

// Containers interfaces with the containerd api to get the list of Containers.
func (c *ContainerdUtil) Containers(namespace string) ([]containerd.Container, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, namespace)
	return c.cl.Containers(ctxNamespace)
}

// EnvVarsFromSpec returns the env variables of a containerd container from its Spec
func EnvVarsFromSpec(spec *oci.Spec, filter func(string) bool) (map[string]string, error) {
	envs := make(map[string]string)
	if spec == nil || spec.Process == nil {
		return envs, nil
	}

	for _, env := range spec.Process.Env {
		envSplit := strings.SplitN(env, "=", 2)

		if len(envSplit) < 2 {
			return nil, errors.New("unexpected environment variable format")
		}

		if filter == nil || filter(envSplit[0]) {
			envs[envSplit[0]] = envSplit[1]
		}
	}

	return envs, nil
}

// ListImages interfaces with the containerd api to list image
func (c *ContainerdUtil) ListImages(namespace string) ([]containerd.Image, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, namespace)

	return c.cl.ListImages(ctxNamespace)
}

// Image interfaces with the containerd api to get an image
func (c *ContainerdUtil) Image(namespace string, name string) (containerd.Image, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, namespace)

	return c.cl.GetImage(ctxNamespace, name)
}

// ImageOfContainer interfaces with the containerd api to get an image
func (c *ContainerdUtil) ImageOfContainer(namespace string, ctn containerd.Container) (containerd.Image, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, namespace)

	return ctn.Image(ctxNamespace)
}

// ImageSize interfaces with the containerd api to get the size of an image
func (c *ContainerdUtil) ImageSize(namespace string, ctn containerd.Container) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, namespace)

	img, err := ctn.Image(ctxNamespace)
	if err != nil {
		return 0, err
	}
	return img.Size(ctxNamespace)
}

// Info interfaces with the containerd api to get Container info
func (c *ContainerdUtil) Info(namespace string, ctn containerd.Container) (containers.Container, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, namespace)

	return ctn.Info(ctxNamespace)
}

// Labels interfaces with the containerd api to get Container labels
func (c *ContainerdUtil) Labels(namespace string, ctn containerd.Container) (map[string]string, error) {
	return c.LabelsWithContext(context.Background(), namespace, ctn)
}

// LabelsWithContext interfaces with the containerd api to get Container labels
// It allows passing the parent context
func (c *ContainerdUtil) LabelsWithContext(ctx context.Context, namespace string, ctn containerd.Container) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, namespace)

	return ctn.Labels(ctxNamespace)
}

// Spec unmarshal Spec from container.Info(), will return parsed Spec if size < maxSize
func (c *ContainerdUtil) Spec(namespace string, ctn containers.Container, maxSize int) (*oci.Spec, error) {
	if len(ctn.Spec.Value) >= maxSize {
		return nil, fmt.Errorf("unable to get spec for container: %s/%s, err: %w", namespace, ctn.ID, ErrSpecTooLarge)
	}

	var s oci.Spec
	if err := json.Unmarshal(ctn.Spec.Value, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// TaskMetrics interfaces with the containerd api to get the metrics from a container
func (c *ContainerdUtil) TaskMetrics(namespace string, ctn containerd.Container) (*types.Metric, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, namespace)

	t, errTask := ctn.Task(ctxNamespace, nil)
	if errTask != nil {
		return nil, errTask
	}

	return t.Metrics(ctxNamespace)
}

// TaskPids interfaces with the containerd api to get the pids from a container
func (c *ContainerdUtil) TaskPids(namespace string, ctn containerd.Container) ([]containerd.ProcessInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, namespace)

	t, errTask := ctn.Task(ctxNamespace, nil)
	if errTask != nil {
		return nil, errTask
	}

	return t.Pids(ctxNamespace)
}

// Status interfaces with the containerd api to get the status for a container
func (c *ContainerdUtil) Status(namespace string, ctn containerd.Container) (containerd.ProcessStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, namespace)

	task, err := ctn.Task(ctxNamespace, nil)
	if err != nil {
		return "", err
	}

	ctx, cancel = context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()
	ctxNamespace = namespaces.WithNamespace(ctx, namespace)

	taskStatus, err := task.Status(ctxNamespace)
	if err != nil {
		return "", err
	}

	return taskStatus.Status, nil
}

// IsSandbox returns whether a container is a sandbox (a.k.a pause container).
// It checks the io.cri-containerd.kind label
// Ref:
// - https://github.com/containerd/cri/blob/release/1.4/pkg/server/helpers.go#L74
func (c *ContainerdUtil) IsSandbox(namespace string, ctn containerd.Container) (bool, error) {
	labels, err := c.Labels(namespace, ctn)
	if err != nil {
		return false, err
	}

	return labels["io.cri-containerd.kind"] == "sandbox", nil
}

func (c *ContainerdUtil) MountImage(ctx context.Context, expiration time.Duration, namespace string, img containerd.Image, targetDir string) (func(context.Context) error, error) {
	snapshotter := containerd.DefaultSnapshotter
	ctx = namespaces.WithNamespace(ctx, namespace)

	// Checking if image is already unpacked
	imgUnpacked, err := img.IsUnpacked(ctx, snapshotter)
	if err != nil {
		return nil, fmt.Errorf("unable to check if image named: %s is unpacked, err: %w", img.Name(), err)
	}
	if !imgUnpacked {
		return nil, fmt.Errorf("unable to scan image named: %s, image is not unpacked", img.Name())
	}

	// Getting image id
	imgConfig, err := img.Config(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to get image config for image named: %s, err: %w", img.Name(), err)
	}
	imageID := imgConfig.Digest.String()

	// Adding a lease to cleanup dandling snaphots at expiration
	ctx, done, err := c.cl.WithLease(ctx,
		leases.WithID(imageID),
		leases.WithExpiration(expiration),
		leases.WithLabels(map[string]string{
			"containerd.io/gc.ref.snapshot." + snapshotter: imageID,
		}),
	)
	if err != nil && !errdefs.IsAlreadyExists(err) {
		return nil, fmt.Errorf("unable to get a lease, err: %w", err)
	}

	// Getting top layer image id
	diffIDs, err := img.RootFS(ctx)
	if err != nil {
		if err := done(ctx); err != nil {
			log.Warnf("Unable to cancel containerd lease with id: %s, err: %v", imageID, err)
		}
		return nil, fmt.Errorf("unable to get layers digests for image: %s, err: %w", imageID, err)
	}
	chainID := identity.ChainID(diffIDs).String()

	// Creating snaphot for the top layer
	s := c.cl.SnapshotService(snapshotter)
	mounts, err := s.View(ctx, imageID, chainID)
	if err != nil && !errdefs.IsAlreadyExists(err) {
		if err := done(ctx); err != nil {
			log.Warnf("Unable to cancel containerd lease with id: %s, err: %v", imageID, err)
		}
		return nil, fmt.Errorf("unable to build snapshot for image: %s, err: %w", imageID, err)
	}
	cleanSnapshot := func(ctx context.Context) error {
		return s.Remove(ctx, imageID)
	}

	// Nothing returned
	if len(mounts) == 0 {
		if err := cleanSnapshot(ctx); err != nil {
			log.Warnf("Unable to clean snapshot with id: %s, err: %v", imageID, err)
		}
		if err := done(ctx); err != nil {
			log.Warnf("Unable to cancel containerd lease with id: %s, err: %v", imageID, err)
		}
		return nil, fmt.Errorf("No snapshots returned for image: %s, err: %w", imageID, err)
	}

	// Transforming mounts in case we're running in a container
	if config.IsContainerized() {
		for i := range mounts {
			mounts[i].Source = strings.ReplaceAll(mounts[i].Source, "/var/lib", "/host/var/lib")
			for j := range mounts[i].Options {
				mounts[i].Options[j] = strings.ReplaceAll(mounts[i].Options[j], "/var/lib", "/host/var/lib")
			}
		}
	}

	// Mouting returned mounts
	log.Infof("Mounting %+v to %s", mounts, targetDir)
	if err := mount.All(mounts, targetDir); err != nil {
		if err := cleanSnapshot(ctx); err != nil {
			log.Warnf("Unable to clean snapshot with id: %s, err: %v", imageID, err)
		}
		if err := done(ctx); err != nil {
			log.Warnf("Unable to cancel containerd lease with id: %s, err: %v", imageID, err)
		}
		return nil, fmt.Errorf("unable to mount image %s to dir %s, err: %w", imageID, targetDir, err)
	}

	return func(ctx context.Context) error {
		ctx = namespaces.WithNamespace(ctx, namespace)

		if err := mount.UnmountAll(targetDir, 0); err != nil {
			return fmt.Errorf("unable to unmount directory: %s for image: %s, err: %w", targetDir, imageID, err)
		}
		if err := cleanSnapshot(ctx); err != nil && !errdefs.IsNotFound(err) {
			return fmt.Errorf("unable to cleanup snapshot for image: %s, err: %w", imageID, err)
		}
		if err := done(ctx); err != nil {
			return fmt.Errorf("unable to cancel lease for image: %s, err: %w", imageID, err)
		}

		return nil
	}, nil
}
