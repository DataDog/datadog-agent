// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build podman

// Package podman implements the podman Workloadmeta collector.
package podman

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"sort"
	"strings"

	"go.uber.org/fx"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	dderrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/podman"
)

const (
	collectorID       = "podman"
	componentName     = "workloadmeta-podman"
	defaultBoltDBPath = "/var/lib/containers/storage/libpod/bolt_state.db"
	defaultSqlitePath = "/var/lib/containers/storage/db.sql"

	// defaultHomePodmanBoltDBSuffix and defaultHomePodmanSQLiteSuffix are appended to a user's
	// home directory to find their rootless Podman database.
	defaultHomePodmanBoltDBSuffix = "/.local/share/containers/storage/libpod/bolt_state.db"
	defaultHomePodmanSQLiteSuffix = "/.local/share/containers/storage/db.sql"
)

type podmanClient interface {
	GetAllContainers() ([]podman.Container, error)
}

// podmanDBClient couples a DB client with the Podman storage root dir derived
// from the DB path so that Pull can tag each container with its origin.
type podmanDBClient struct {
	client  podmanClient
	rootDir string
}

type collector struct {
	id      string
	clients []podmanDBClient
	store   workloadmeta.Component
	catalog workloadmeta.AgentType
	seen    map[workloadmeta.EntityID]struct{}
}

// NewCollector returns a new podman collector provider and an error
func NewCollector() (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &collector{
			id:      collectorID,
			seen:    make(map[workloadmeta.EntityID]struct{}),
			catalog: workloadmeta.NodeAgent,
		},
	}, nil
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewCollector)
}

// Start the collector for the provided workloadmeta component
func (c *collector) Start(_ context.Context, store workloadmeta.Component) error {
	if !env.IsFeaturePresent(env.Podman) {
		return dderrors.NewDisabled(componentName, "Podman not detected")
	}

	rawDBPath := pkgconfigsetup.Datadog().GetString("podman_db_path")

	var dbPaths []string
	if rawDBPath != "" {
		dbPaths = parsePaths(rawDBPath)
	} else {
		// Auto-discover DB paths for root and all users under /home/.
		dbPaths = discoverDefaultDBPaths()
		if len(dbPaths) == 0 {
			return dderrors.NewDisabled(componentName, "Podman feature detected but no accessible container database found")
		}
	}

	var clients []podmanDBClient
	for _, dbPath := range dbPaths {
		if !dbIsAccessible(dbPath) {
			log.Warnf("Podman DB path %q is not accessible, skipping", dbPath)
			continue
		}
		client, err := clientForPath(dbPath)
		if err != nil {
			log.Warnf("Could not create Podman client for path %q: %v, skipping", dbPath, err)
			continue
		}
		rootDir := log.ExtractPodmanRootDirFromDBPath(dbPath)
		if rootDir == "" {
			log.Warnf("Could not derive Podman storage root dir from DB path %q, skipping", dbPath)
			continue
		}
		log.Infof("Using Podman DB at %q (root dir: %q)", dbPath, rootDir)
		clients = append(clients, podmanDBClient{client: client, rootDir: rootDir})
	}

	if len(clients) == 0 {
		return dderrors.NewDisabled(componentName, "podman_db_path is misconfigured/not accessible")
	}

	c.clients = clients
	c.store = store

	return nil
}

func (c *collector) Pull(_ context.Context) error {
	seen := make(map[workloadmeta.EntityID]struct{})
	events := []workloadmeta.CollectorEvent{}

	for _, dbc := range c.clients {
		containers, err := dbc.client.GetAllContainers()
		if err != nil {
			log.Warnf("Error fetching Podman containers from one of the configured databases: %v", err)
			continue
		}
		for _, container := range containers {
			event := convertToEvent(&container, dbc.rootDir)
			seen[event.Entity.GetID()] = struct{}{}
			events = append(events, event)
		}
	}

	for seenID := range c.seen {
		if _, ok := seen[seenID]; ok {
			continue
		}

		events = append(events, workloadmeta.CollectorEvent{
			Type:   workloadmeta.EventTypeUnset,
			Source: workloadmeta.SourceRuntime,
			Entity: &workloadmeta.Container{
				EntityID: seenID,
			},
		})
	}

	c.seen = seen

	c.store.Notify(events)

	return nil
}

func (c *collector) GetID() string {
	return c.id
}

func (c *collector) GetTargetCatalog() workloadmeta.AgentType {
	return c.catalog
}

func convertToEvent(container *podman.Container, rootDir string) workloadmeta.CollectorEvent {
	containerID := container.Config.ID

	// Start from the OCI spec annotations and add our own metadata.
	annotations := make(map[string]string)
	if spec := container.Config.Spec; spec != nil {
		maps.Copy(annotations, spec.Annotations)
	}
	if rootDir != "" {
		annotations[log.ContainerRootDirAnnotationKey] = rootDir
	}

	envs, err := envVars(container)
	if err != nil {
		log.Warnf("Could not get env vars for container %s", containerID)
	}

	imageName := container.Config.RawImageName
	if imageName == "" {
		imageName = container.Config.RootfsImageName
	}
	image, err := workloadmeta.NewContainerImage(container.Config.ContainerRootFSConfig.RootfsImageID, imageName)
	if err != nil {
		log.Warnf("Could not get image for container %s", containerID)
	}

	var ports []workloadmeta.ContainerPort
	for _, portMapping := range container.Config.PortMappings {
		ports = append(ports, workloadmeta.ContainerPort{
			Port:     int(portMapping.ContainerPort),
			Protocol: portMapping.Protocol,
		})
	}

	var eventType workloadmeta.EventType
	if container.State.State == podman.ContainerStateRunning {
		eventType = workloadmeta.EventTypeSet
	} else {
		eventType = workloadmeta.EventTypeUnset
	}

	return workloadmeta.CollectorEvent{
		Type:   eventType,
		Source: workloadmeta.SourceRuntime,
		Entity: &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   containerID,
			},
			EntityMeta: workloadmeta.EntityMeta{
				Name:        container.Config.Name,
				Namespace:   container.Config.Namespace,
				Annotations: annotations,
				Labels:      container.Config.Labels,
			},
			EnvVars:    envs,
			Hostname:   hostname(container),
			Image:      image,
			NetworkIPs: networkIPs(container),
			PID:        container.State.PID,
			Ports:      ports,
			Runtime:    workloadmeta.ContainerRuntimePodman,
			State: workloadmeta.ContainerState{
				Running:    container.State.State == podman.ContainerStateRunning,
				Status:     status(container.State.State),
				StartedAt:  container.State.StartedTime,
				CreatedAt:  container.State.StartedTime, // CreatedAt not available
				FinishedAt: container.State.FinishedTime,
			},
			RestartCount: int(container.State.RestartCount),
		},
	}
}

func getShortID(container *podman.Container) (containerID string) {
	if len(container.Config.ID) >= 12 {
		containerID = container.Config.ID[:12]
	} else {
		containerID = container.Config.ID
	}
	return
}

func networkIPs(container *podman.Container) map[string]string {
	res := make(map[string]string)

	// container.Config.Networks contains only the networks specified at container creation time
	// and not the ones attached afterwards with `podman network attach`
	// They appear in the order in which they were specified in the `podman run --net=…` command
	networkNames := make([]string, len(container.Config.Networks))
	copy(networkNames, container.Config.Networks)
	sort.Strings(networkNames)

	// Handle the default case where no `--net` is specified
	if len(networkNames) == 0 && len(container.State.NetworkStatus) == 1 {
		networkNames = []string{"podman"}
	}

	if len(networkNames) != len(container.State.NetworkStatus) {
		log.Warnf("podman container %s %s has now a number of networks (%d) different from what it was at creation time (%d). This can be due to the use of `podman network attach`/`podman network detach`. This may confuse the agent.", getShortID(container), container.Config.Name, len(container.State.NetworkStatus), len(networkNames))
		return map[string]string{}
	}

	// container.State.NetworkStatus contains all the networks but they are not in the same order
	// as in container.Config.Network. Here, they are sorted by network name.
	for i := 0; i < len(networkNames); i++ {
		if len(container.State.NetworkStatus[i].IPs) > 1 {
			log.Warnf("podman container %s %s has several IPs on network %s. This is most probably because of a dual-stack IPv4/IPv6 setup. The agent will use only the first IP.", getShortID(container), container.Config.Name, networkNames[i])
		}
		if len(container.State.NetworkStatus[i].IPs) > 0 {
			res[networkNames[i]] = container.State.NetworkStatus[i].IPs[0].Address.IP.String()
		}
	}

	return res
}

func envVars(container *podman.Container) (map[string]string, error) {
	res := make(map[string]string)

	if container.Config.Spec == nil || container.Config.Spec.Process == nil {
		return res, nil
	}

	for _, env := range container.Config.Spec.Process.Env {
		envSplit := strings.SplitN(env, "=", 2)

		if len(envSplit) < 2 {
			return nil, errors.New("unexpected environment variable format")
		}

		if containers.EnvVarFilterFromConfig().IsIncluded(envSplit[0]) {
			res[envSplit[0]] = envSplit[1]
		}
	}

	return res, nil
}

// This function has been copied from
// https://github.com/containers/podman/blob/v3.4.1/libpod/container.go
func hostname(container *podman.Container) string {
	if container.Config.Spec.Hostname != "" {
		return container.Config.Spec.Hostname
	}

	if len(container.Config.ID) < 11 {
		return container.Config.ID
	}
	return container.Config.ID[:12]
}

func status(state podman.ContainerStatus) workloadmeta.ContainerStatus {
	switch state {
	case podman.ContainerStateConfigured, podman.ContainerStateCreated:
		return workloadmeta.ContainerStatusCreated
	case podman.ContainerStateStopping, podman.ContainerStateExited, podman.ContainerStateStopped, podman.ContainerStateRemoving:
		return workloadmeta.ContainerStatusStopped
	case podman.ContainerStateRunning:
		return workloadmeta.ContainerStatusRunning
	case podman.ContainerStatePaused:
		return workloadmeta.ContainerStatusPaused
	}

	return workloadmeta.ContainerStatusUnknown
}

// parsePaths splits a comma-separated list of DB paths and trims whitespace from each entry.
func parsePaths(raw string) []string {
	parts := strings.Split(raw, ",")
	paths := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

// clientForPath creates the appropriate Podman DB client based on the file extension of dbPath.
func clientForPath(dbPath string) (podmanClient, error) {
	if strings.HasSuffix(dbPath, ".sql") {
		log.Debugf("Using SQLite client for Podman DB at %q", dbPath)
		return podman.NewSQLDBClient(dbPath), nil
	} else if strings.HasSuffix(dbPath, ".db") {
		log.Debugf("Using BoltDB client for Podman DB at %q", dbPath)
		return podman.NewDBClient(dbPath), nil
	}
	return nil, fmt.Errorf("path %q does not end in a known format (.db or .sql)", dbPath)
}

// discoverDefaultDBPaths finds all accessible Podman database files: the rootfull database and
// rootless databases for all users under /home/.
func discoverDefaultDBPaths() []string {
	var paths []string

	// Rootfull databases
	for _, p := range []string{defaultSqlitePath, defaultBoltDBPath} {
		if dbIsAccessible(p) {
			log.Infof("Auto-discovered Podman DB at %q", p)
			paths = append(paths, p)
			break
		}
	}

	// Rootless databases: scan /home/ for users with Podman installed
	homeEntries, err := os.ReadDir("/home")
	if err != nil {
		log.Debugf("Could not scan /home/ for Podman databases: %v", err)
		return paths
	}

	for _, entry := range homeEntries {
		if !entry.IsDir() {
			continue
		}
		homeDir := "/home/" + entry.Name()
		for _, suffix := range []string{defaultHomePodmanSQLiteSuffix, defaultHomePodmanBoltDBSuffix} {
			p := homeDir + suffix
			if dbIsAccessible(p) {
				log.Infof("Auto-discovered Podman DB at %q", p)
				paths = append(paths, p)
				break
			}
		}
	}

	return paths
}

// dbIsAccessible verifies whether or not the provided file is accessible by the Agent
func dbIsAccessible(dbPath string) bool {
	if _, err := os.Stat(dbPath); err == nil {
		return true
	}
	return false
}
