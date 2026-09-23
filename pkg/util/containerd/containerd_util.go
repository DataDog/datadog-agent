// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd

// Package containerd provides a containerd client.
package containerd

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/opencontainers/image-spec/identity"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	dderrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/containers/image"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"

	"github.com/containerd/containerd/api/types"
	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/core/leases"
	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/containerd/v2/defaults"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/errdefs"
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
	ListImagesWithDigest(namespace string, digest string) ([]containerd.Image, error)
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
	Mounts(ctx context.Context, expiration time.Duration, namespace string, img containerd.Image) ([]mount.Mount, func(context.Context) error, error)
	MountsWithSnapshotter(ctx context.Context, expiration time.Duration, namespace string, img containerd.Image) ([]mount.Mount, string, func(context.Context) error, error)
}

// ContainerdUtil is the util used to interact with the Containerd api.
type ContainerdUtil struct {
	cl                *containerd.Client
	socketPath        string
	initRetry         retry.Retrier
	queryTimeout      time.Duration
	connectionTimeout time.Duration
}

// NewContainerdUtil creates the Containerd util containing the Containerd client and implementing the ContainerdItf
// Errors are handled in the retrier.
func NewContainerdUtil() (ContainerdItf, error) {
	// A singleton does not work because different parts of the code
	// (workloadmeta, checks, etc.) might need to fetch info from different
	// namespaces at the same time.
	containerdUtil := &ContainerdUtil{
		queryTimeout:      pkgconfigsetup.Datadog().GetDuration("cri_query_timeout") * time.Second,
		connectionTimeout: pkgconfigsetup.Datadog().GetDuration("cri_connection_timeout") * time.Second,
		socketPath:        pkgconfigsetup.Datadog().GetString("cri_socket_path"),
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

// RawClient tries to connect to containerd api
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

	// containerd.New does not dial, it only builds a lazy gRPC client, so this
	// is the first call that actually reaches the daemon. If it fails we have to
	// close the client ourselves: callers discard the ContainerdUtil when the
	// initial connection fails and never get a chance to. An unclosed
	// ClientConn is kept alive forever by the callback serializers that its
	// idle mode re-arms, so every orphan is a permanent leak.
	ver, err := c.Metadata()
	if err != nil {
		if errClose := c.cl.Close(); errClose != nil {
			log.Warnf("Could not close the containerd client after a failed connection attempt: %v", errClose)
		}
		c.cl = nil
		return err
	}

	log.Infof("Connected to containerd - Version %s/%s", ver.Version, ver.Revision)
	return nil
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

// ListImagesWithDigest interfaces with the containerd api to list image with digest filter
// Digest is the sha256 digest (repo digest) of the compressed image manifest which is defined in
// https://github.com/opencontainers/image-spec/blob/main/descriptor.md#digests
func (c *ContainerdUtil) ListImagesWithDigest(namespace string, digest string) ([]containerd.Image, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.queryTimeout)
	defer cancel()
	ctxNamespace := namespaces.WithNamespace(ctx, namespace)
	filter := "target.digest==" + digest
	return c.cl.ListImages(ctxNamespace, filter)
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
	spec := ctn.Spec.GetValue()
	if len(spec) >= maxSize {
		return nil, fmt.Errorf("unable to get spec for container: %s/%s, err: %w", namespace, ctn.ID, ErrSpecTooLarge)
	}

	var s oci.Spec
	if err := json.Unmarshal(spec, &s); err != nil {
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

// Mounts retrieves mounts and returns a function to clean the snapshot and release the lease. The lease is already released in error cases.
func (c *ContainerdUtil) Mounts(ctx context.Context, expiration time.Duration, namespace string, img containerd.Image) ([]mount.Mount, func(context.Context) error, error) {
	mounts, _, cleanup, err := c.MountsWithSnapshotter(ctx, expiration, namespace, img)
	return mounts, cleanup, err
}

// cleanupTimeout bounds the calls that release a view snapshot and its lease.
const cleanupTimeout = 30 * time.Second

// cleanupContext keeps ctx's values, the containerd namespace among them, and
// replaces ctx's cancellation with a deadline of its own. Releasing a view
// snapshot takes two RPCs, and the caller is usually cancelling by then, which
// would fail both and leave the snapshot holding what it materialised.
func cleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
}

// MountsWithSnapshotter selects an unpacked snapshotter for the existing SBOM paths.
func (c *ContainerdUtil) MountsWithSnapshotter(ctx context.Context, expiration time.Duration, namespace string, img containerd.Image) ([]mount.Mount, string, func(context.Context) error, error) {
	ctx = namespaces.WithNamespace(ctx, namespace)
	var failures []error
	for _, snapshotter := range []string{"nydus", defaults.DefaultSnapshotter} {
		unpacked, err := img.IsUnpacked(ctx, snapshotter)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if !unpacked {
			continue
		}
		mounts, cleanup, err := acquireImageMounts(ctx, c.cl, expiration, namespace, img, snapshotter)
		return mounts, snapshotter, cleanup, err
	}
	return nil, "", nil, errors.Join(fmt.Errorf("image %s is not unpacked in a supported snapshotter", img.Name()), errors.Join(failures...))
}

func acquireImageMounts(ctx context.Context, client *containerd.Client, expiration time.Duration, namespace string, img containerd.Image, snapshotter string) ([]mount.Mount, func(context.Context) error, error) {
	ctx = namespaces.WithNamespace(ctx, namespace)
	unpacked, err := img.IsUnpacked(ctx, snapshotter)
	if err != nil {
		return nil, nil, fmt.Errorf("check image %s in %s: %w", img.Name(), snapshotter, err)
	}
	if !unpacked {
		return nil, nil, fmt.Errorf("image %s is not unpacked in %s", img.Name(), snapshotter)
	}
	diffIDs, err := img.RootFS(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("read image rootfs: %w", err)
	}
	if len(diffIDs) == 0 {
		return nil, nil, errors.New("image has no filesystem layers")
	}
	chainID := identity.ChainID(diffIDs).String()
	return acquireImageMountsForChain(ctx, client, expiration, namespace, chainID, snapshotter)
}

func acquireImageMountsForChain(ctx context.Context, client *containerd.Client, expiration time.Duration, namespace, chainID, snapshotter string) ([]mount.Mount, func(context.Context) error, error) {
	ctx = namespaces.WithNamespace(ctx, namespace)
	imageID := "datadog-image-view-" + rand.Text()
	// Create directly: Client.WithLease may reuse a lease inherited from ctx.
	leaseService := client.LeasesService()
	lease, err := leaseService.Create(ctx, leases.WithID(imageID), leases.WithExpiration(expiration),
		leases.WithLabels(map[string]string{"containerd.io/gc.ref.snapshot." + snapshotter: chainID}))
	if err != nil {
		return nil, nil, fmt.Errorf("create image lease: %w", err)
	}
	ctx = leases.WithLease(ctx, lease.ID)
	s := client.SnapshotService(snapshotter)
	mounts, viewErr := s.View(ctx, imageID, chainID)
	ownedView := viewErr == nil
	cleanup := imageViewCleanup(namespace, func(releaseCtx context.Context) error {
		if ownedView {
			return s.Remove(releaseCtx, imageID)
		}
		return nil
	}, func(releaseCtx context.Context) error { return leaseService.Delete(releaseCtx, lease) })
	if viewErr != nil {
		return nil, nil, errors.Join(fmt.Errorf("create image view: %w", viewErr), cleanup(ctx))
	}
	if len(mounts) == 0 {
		return nil, nil, errors.Join(errors.New("snapshotter returned no image mounts"), cleanup(ctx))
	}
	if env.IsContainerized() {
		for i := range mounts {
			mounts[i].Source = image.SanitizeHostPath(mounts[i].Source)

			var errs []error
			for j, opt := range mounts[i].Options {
				for _, prefix := range []string{"upperdir=", "lowerdir=", "workdir="} {
					if trimmedOpt, ok := strings.CutPrefix(opt, prefix); ok {
						dirs := strings.Split(trimmedOpt, ":")
						for n, dir := range dirs {
							dirs[n] = image.SanitizeHostPath(dir)
							if _, err := os.Stat(dirs[n]); err != nil {
								errs = append(errs, fmt.Errorf("unreachable folder %s for overlayfs mount: %w", dir, err))
							}
						}
						mounts[i].Options[j] = prefix + strings.Join(dirs, ":")
					}
				}

				log.Debugf("Sanitized overlayfs mount options to %s", strings.Join(mounts[i].Options, ","))
			}

			if len(errs) > 0 {
				log.Warnf("Unreachable path detected in mounts for image %s: %s", imageID, errors.Join(errs...))
			}
		}
	}

	return mounts, cleanup, nil
}

func imageViewCleanup(namespace string, removeView, releaseLease func(context.Context) error) func(context.Context) error {
	var once sync.Once
	var cleanupErr error
	return func(callerCtx context.Context) error {
		once.Do(func() {
			releaseCtx, cancel := cleanupContext(namespaces.WithNamespace(callerCtx, namespace))
			viewErr := removeView(releaseCtx)
			cancel()
			if errdefs.IsNotFound(viewErr) {
				viewErr = nil
			}
			// Even if view removal exhausts its deadline, release the lease
			// with a fresh budget so snapshot expiry is only a final backstop.
			releaseCtx, cancel = cleanupContext(namespaces.WithNamespace(callerCtx, namespace))
			leaseErr := releaseLease(releaseCtx)
			cancel()
			if errdefs.IsNotFound(leaseErr) {
				leaseErr = nil
			}
			cleanupErr = errors.Join(viewErr, leaseErr)
		})
		return cleanupErr
	}
}

// MountImage mounts an image to a directory
func (c *ContainerdUtil) MountImage(ctx context.Context, expiration time.Duration, namespace string, img containerd.Image, targetDir string) (func(context.Context) error, error) {
	mounts, clean, err := c.Mounts(ctx, expiration, namespace, img)
	if err != nil {
		return nil, err
	}
	if err := mount.All(mounts, targetDir); err != nil {
		if err := clean(ctx); err != nil {
			log.Warnf("Unable to clean snapshot, err: %v", err)
		}
		return nil, fmt.Errorf("unable to mount image %s to dir %s, err: %w", img.Name(), targetDir, err)
	}
	return func(ctx context.Context) error {
		ctx = namespaces.WithNamespace(ctx, namespace)
		if err := mount.UnmountAll(targetDir, 0); err != nil {
			return fmt.Errorf("unable to unmount directory: %s for image: %s, err: %w", targetDir, img.Name(), err)
		}
		return clean(ctx)
	}, nil
}
