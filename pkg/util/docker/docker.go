// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package docker

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/versions"
	"github.com/docker/docker/client"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

var (
	// ErrDockerNotAvailable is returned if Docker is not running on the current machine.
	// We'll use this when configuring the DockerUtil so we don't error on non-docker machines.
	ErrDockerNotAvailable = errors.New("docker not available")

	globalDockerUtil     *dockerUtil
	invalidationInterval = 5 * time.Minute
	lastErr              string

	// NullContainer is an empty container object that has
	// default values for all fields including sub-fields.
	// If new sub-structs are added to Container this must
	// be updated.
	NullContainer = &Container{
		CPU:     &CgroupTimesStat{},
		Memory:  &CgroupMemStat{},
		IO:      &CgroupIOStat{},
		Network: &NetworkStat{},
	}
)

const (
	ContainerCreatedState    string = "created"
	ContainerRunningState    string = "running"
	ContainerRestartingState string = "restarting"
	ContainerPausedState     string = "paused"
	ContainerExitedState     string = "exited"
	ContainerDeadState       string = "dead"
)

// NetworkStat stores network statistics about a Docker container.
type NetworkStat struct {
	BytesSent   uint64
	BytesRcvd   uint64
	PacketsSent uint64
	PacketsRcvd uint64
}

type containerFilter struct {
	Enabled        bool
	ImageWhitelist []*regexp.Regexp
	NameWhitelist  []*regexp.Regexp
	ImageBlacklist []*regexp.Regexp
	NameBlacklist  []*regexp.Regexp
}

// HostnameProvider docker implementation for the hostname provider
func HostnameProvider(hostName string) (string, error) {
	return GetHostname()
}

// DefaultGateway returns the default Docker gateway.
func DefaultGateway() (net.IP, error) {
	procRoot := config.Datadog.GetString("proc_root")
	netRouteFile := filepath.Join(procRoot, "net", "route")
	f, err := os.Open(netRouteFile)
	if os.IsNotExist(err) || os.IsPermission(err) {
		log.Errorf("unable to open %s: %s", netRouteFile, err)
		return nil, nil
	} else if err != nil {
		// Unknown error types will bubble up for handling.
		return nil, err
	}
	defer f.Close()

	ip := make(net.IP, 4)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 3 && fields[1] == "00000000" {
			ipInt, err := strconv.ParseInt(fields[2], 16, 32)
			if err != nil {
				return nil, fmt.Errorf("unable to parse ip %s, from %s: %s", fields[2], netRouteFile, err)
			}
			binary.LittleEndian.PutUint32(ip, uint32(ipInt))
			break
		}
	}
	return ip, nil
}

// NewcontainerFilter creates a new container filter from a two slices of
// regexp patterns for a whitelist and blacklist. Each pattern should have
// the following format: "field:pattern" where field can be: [image, name].
// An error is returned if any of the expression don't compile.
func newContainerFilter(whitelist, blacklist []string) (*containerFilter, error) {
	iwl, nwl, err := parseFilters(whitelist)
	if err != nil {
		return nil, err
	}
	ibl, nbl, err := parseFilters(blacklist)
	if err != nil {
		return nil, err
	}

	return &containerFilter{
		Enabled:        len(whitelist) > 0 || len(blacklist) > 0,
		ImageWhitelist: iwl,
		NameWhitelist:  nwl,
		ImageBlacklist: ibl,
		NameBlacklist:  nbl,
	}, nil
}

func parseFilters(filters []string) (imageFilters, nameFilters []*regexp.Regexp, err error) {
	for _, filter := range filters {
		switch {
		case strings.HasPrefix(filter, "image:"):
			pat := strings.TrimPrefix(filter, "image:")
			r, err := regexp.Compile(strings.TrimPrefix(pat, "image:"))
			if err != nil {
				return nil, nil, fmt.Errorf("invalid regex '%s': %s", pat, err)
			}
			imageFilters = append(imageFilters, r)
		case strings.HasPrefix(filter, "name:"):
			pat := strings.TrimPrefix(filter, "name:")
			r, err := regexp.Compile(pat)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid regex '%s': %s", pat, err)
			}
			nameFilters = append(nameFilters, r)
		}
	}
	return imageFilters, nameFilters, nil
}

// IsExcluded returns a bool indicating if the container should be excluded
// based on the filters in the containerFilter instance.
func (cf containerFilter) IsExcluded(container *Container) bool {
	return cf.computeIsExcluded(container.Name, container.Image)
}

func (cf containerFilter) computeIsExcluded(containerName, containerImage string) bool {
	if !cf.Enabled {
		return false
	}

	var excluded bool
	for _, r := range cf.ImageBlacklist {
		if r.MatchString(containerImage) {
			excluded = true
			break
		}
	}
	for _, r := range cf.NameBlacklist {
		if r.MatchString(containerName) {
			excluded = true
			break
		}
	}

	// Any excluded container could be whitelisted.
	if excluded {
		for _, r := range cf.ImageWhitelist {
			if r.MatchString(containerImage) {
				return false
			}
		}
		for _, r := range cf.NameWhitelist {
			if r.MatchString(containerName) {
				return false
			}
		}
	}
	return excluded
}

// Container represents a single Docker container on a machine
// and includes Cgroup-level statistics about the container.
type Container struct {
	Type     string
	ID       string
	EntityID string
	Name     string
	Image    string
	ImageID  string
	Created  int64
	State    string
	Health   string
	Pids     []int32
	Excluded bool

	CPULimit       float64
	MemLimit       uint64
	CPUNrThrottled uint64
	CPU            *CgroupTimesStat
	Memory         *CgroupMemStat
	IO             *CgroupIOStat
	Network        *NetworkStat
	StartedAt      int64

	// For internal use only
	cgroup *ContainerCgroup
}

// Inspect allows getting the full docker inspect of a Container
func (c *Container) Inspect(withSize bool) (types.ContainerJSON, error) {
	cj, err := Inspect(c.ID, withSize)
	return cj, err
}

// Inspect returns a docker inspect object for a given container ID.
// It tries to locate the container in the inspect cache before making the docker inspect call
func Inspect(id string, withSize bool) (types.ContainerJSON, error) {
	cacheKey := GetInspectCacheKey(id)
	var container types.ContainerJSON
	var err error
	var ok bool

	if cached, hit := cache.Cache.Get(cacheKey); hit {
		container, ok = cached.(types.ContainerJSON)
		if !ok {
			log.Errorf("invalid cache format, forcing a cache miss")
		}
	} else {
		if globalDockerUtil == nil {
			return types.ContainerJSON{}, fmt.Errorf("DockerUtil not initialized")
		}
		container, _, err = globalDockerUtil.cli.ContainerInspectWithRaw(context.Background(), id, withSize)
		// cache the inspect for 10 seconds to reduce pressure on the daemon
		cache.Cache.Set(cacheKey, container, 10*time.Second)
	}

	return container, err
}

type dockerNetwork struct {
	iface      string
	dockerName string
}

type dockerNetworks []dockerNetwork

func (a dockerNetworks) Len() int           { return len(a) }
func (a dockerNetworks) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a dockerNetworks) Less(i, j int) bool { return a[i].dockerName < a[j].dockerName }

// Config is an exported configuration object that is used when
// initializing the DockerUtil.
type Config struct {
	// CacheDuration is the amount of time we will cache the active docker
	// containers and cgroups. The actual raw metrics (e.g. MemRSS) will _not_
	// be cached but will be re-calculated on all calls to AllContainers.
	CacheDuration time.Duration
	// CollectNetwork enables network stats collection. This requires at least
	// one call to container.Inspect for new containers and reads from the
	// procfs for stats.
	CollectNetwork bool
	// Whitelist is a slice of filter strings in the form of key:regex where key
	// is either 'image' or 'name' and regex is a valid regular expression.
	Whitelist []string
	// Blacklist is the same as whitelist but for exclusion.
	Blacklist []string

	// internal use only
	filter *containerFilter
}

// dockerUtil wraps interactions with a local docker API.
type dockerUtil struct {
	cfg *Config
	cli *client.Client
	// tracks the last time we invalidate our internal caches
	lastInvalidate time.Time
	// networkMappings by container id
	networkMappings map[string][]dockerNetwork
	// image sha mapping cache
	imageNameBySha map[string]string
	sync.Mutex
}

//
// Expose module-level functions that will interact with a Singleton dockerUtil.

type ContainerListConfig struct {
	IncludeExited bool
	FlagExcluded  bool
}

func (cfg *ContainerListConfig) GetCacheKey() string {
	cacheKey := "dockerutil.containers"
	if cfg.IncludeExited {
		cacheKey += ".with_exited"
	} else {
		cacheKey += ".without_exited"
	}

	if cfg.FlagExcluded {
		cacheKey += ".with_excluded"
	} else {
		cacheKey += ".without_excluded"
	}

	return cacheKey
}

// GetInspectCacheKey returns the key to a given container ID inspect in the agent cache
func GetInspectCacheKey(ID string) string {
	return "dockerutil.containers." + ID
}

// AllContainers returns a slice of all containers.
func AllContainers(cfg *ContainerListConfig) ([]*Container, error) {
	if globalDockerUtil != nil {
		r, err := globalDockerUtil.containers(cfg)
		if err != nil {
			return nil, log.Errorf("unable to list Docker containers: %s", err)
		}
		return r, nil
	}
	return nil, nil
}

// AllImages returns a slice of all images.
func AllImages(includeIntermediate bool) ([]types.ImageSummary, error) {
	if globalDockerUtil != nil {
		return globalDockerUtil.dockerImages(includeIntermediate)
	}
	return nil, nil
}

// ResolveImageName resolves a docker image name, probably containing
// sha256 checksum as tag in a name:tag format string.
// This requires InitDockerUtil to be called before.
func ResolveImageName(image string) (string, error) {
	if globalDockerUtil != nil {
		return globalDockerUtil.extractImageName(image), nil
	}
	return "", errors.New("dockerutil not initialised")
}

// SplitImageName splits a valid image name (from ResolveImageName)
// into the name and tag parts.
func SplitImageName(image string) (string, string, error) {
	if image == "" {
		return "", "", errors.New("empty image name")
	}
	parts := strings.SplitN(image, ":", 2)
	if len(parts) < 2 {
		return image, "", errors.New("could not find tag")
	}
	return parts[0], parts[1], nil
}

// GetHostname returns the Docker hostname.
func GetHostname() (string, error) {
	if globalDockerUtil == nil {
		return "", ErrDockerNotAvailable
	}
	return globalDockerUtil.getHostname()
}

// IsContainerized returns True if we're running in the docker-dd-agent container.
func IsContainerized() bool {
	return os.Getenv("DOCKER_DD_AGENT") == "yes"
}

// ConnectToDocker connects to a local docker socket.
// Returns ErrDockerNotAvailable if the socket or mounts file is missing
// otherwise it returns either a valid client or an error.
func ConnectToDocker() (*client.Client, error) {
	// If we don't have a docker.sock then return a known error.
	sockPath := GetEnv("DOCKER_SOCKET_PATH", "/var/run/docker.sock")
	if !PathExists(sockPath) {
		return nil, ErrDockerNotAvailable
	}
	// The /proc/mounts file won't be availble on non-Linux systems
	// and we only support Linux for now.
	mountsFile := "/proc/mounts"
	if !PathExists(mountsFile) {
		return nil, ErrDockerNotAvailable
	}

	// Connect again using the known server version.
	cli, err := client.NewEnvClient()
	if err != nil {
		return nil, err
	}

	// TODO: remove this logic when "client.NegotiateAPIVersion" function is released by moby/docker
	serverVersion, err := detectServerAPIVersion()
	if err != nil || serverVersion == "" {
		log.Errorf("Could not determine docker server API version (using the client version): %s", err)
		return cli, nil
	}

	clientVersion := cli.ClientVersion()
	if versions.LessThan(serverVersion, clientVersion) {
		log.Debugf("Docker server APIVersion ('%s') is lower than the client ('%s'): using version from the server",
			serverVersion, clientVersion)
		cli.UpdateClientVersion(serverVersion)
	}
	return cli, nil
}

// IsAvailable returns true if Docker is available on this machine via a socket.
func IsAvailable() bool {
	if _, err := ConnectToDocker(); err != nil {
		if err != ErrDockerNotAvailable {
			log.Warnf("unable to connect to docker: %s", err)
		}
		return false
	}
	return true
}

// InitDockerUtil initializes the global dockerUtil singleton. This _must_ be
// called before accessing any of the top-level docker calls.
func InitDockerUtil(cfg *Config) error {
	cli, err := ConnectToDocker()
	if err != nil {
		return err
	}

	// Pre-parse the filter and use that internally.
	cfg.filter, err = newContainerFilter(cfg.Whitelist, cfg.Blacklist)
	if err != nil {
		return err
	}

	globalDockerUtil = &dockerUtil{
		cfg:             cfg,
		cli:             cli,
		networkMappings: make(map[string][]dockerNetwork),
		imageNameBySha:  make(map[string]string),
		lastInvalidate:  time.Now(),
	}
	return nil
}

// dockerImages returns a list of Docker info for images.
func (d *dockerUtil) dockerImages(includeIntermediate bool) ([]types.ImageSummary, error) {
	images, err := d.cli.ImageList(context.Background(), types.ImageListOptions{All: includeIntermediate})
	if err != nil {
		return nil, fmt.Errorf("unable to list docker images: %s", err)
	}
	return images, nil
}

// NeedInit returns true if InitDockerUtil has to be called
// before using the package
func NeedInit() bool {
	if globalDockerUtil == nil {
		return true
	}
	return false
}

// dockerContainers returns a list of Docker info for active containers using the
// Docker API. This requires the running user to be in the "docker" user group
// or have access to /tmp/docker.sock.
func (d *dockerUtil) dockerContainers(cfg *ContainerListConfig) ([]*Container, error) {
	containers, err := d.cli.ContainerList(context.Background(), types.ContainerListOptions{All: cfg.IncludeExited})
	if err != nil {
		return nil, fmt.Errorf("error listing containers: %s", err)
	}
	ret := make([]*Container, 0, len(containers))
	for _, c := range containers {
		if d.cfg.CollectNetwork {
			// FIXME: We might need to invalidate this cache if a containers networks are changed live.
			d.Lock()
			if _, ok := d.networkMappings[c.ID]; !ok {
				i, err := d.cli.ContainerInspect(context.Background(), c.ID)
				if err != nil && client.IsErrContainerNotFound(err) {
					d.Unlock()
					log.Debugf("error inspecting container %s: %s", c.ID, err)
					continue
				}
				d.networkMappings[c.ID] = findDockerNetworks(c.ID, i.State.Pid, c.NetworkSettings)
			}
			d.Unlock()
		}

		entityID := fmt.Sprintf("docker://%s", c.ID)
		container := &Container{
			Type:     "Docker",
			ID:       entityID[9:],
			EntityID: entityID,
			Name:     c.Names[0],
			Image:    d.extractImageName(c.Image),
			ImageID:  c.ImageID,
			Created:  c.Created,
			State:    c.State,
			Health:   parseContainerHealth(c.Status),
		}

		container.Excluded = d.cfg.filter.IsExcluded(container)
		if container.Excluded && !cfg.FlagExcluded {
			continue
		}
		ret = append(ret, container)
	}

	if d.lastInvalidate.Add(invalidationInterval).After(time.Now()) {
		d.invalidateCaches(containers)
	}

	return ret, nil
}

// containers gets a list of all containers on the current node using a mix of
// the Docker APIs and cgroups stats. We attempt to limit syscalls where possible.
func (d *dockerUtil) containers(cfg *ContainerListConfig) ([]*Container, error) {
	cacheKey := cfg.GetCacheKey()

	// Get the containers either from our cache or with API queries.
	var containers []*Container
	cached, hit := cache.Cache.Get(cacheKey)
	if hit {
		var ok bool
		containers, ok = cached.([]*Container)
		if !ok {
			log.Errorf("invalid cache format, forcing a cache miss")
			hit = false
		}
	} else {
		var cgByContainer map[string]*ContainerCgroup
		var err error

		cgByContainer, err = ScrapeAllCgroups()
		if err != nil {
			return nil, fmt.Errorf("could not get cgroups: %s", err)
		}

		containers, err = d.dockerContainers(cfg)
		if err != nil {
			return nil, fmt.Errorf("could not get docker containers: %s", err)
		}

		for _, container := range containers {
			if container.Excluded {
				continue
			}
			cgroup, ok := cgByContainer[container.ID]
			if !ok {
				continue
			}
			container.cgroup = cgroup
			container.CPULimit, err = cgroup.CPULimit()
			if err != nil {
				log.Debugf("cgroup cpu limit: %s", err)
			}
			container.MemLimit, err = cgroup.MemLimit()
			if err != nil {
				log.Debugf("cgroup cpu limit: %s", err)
			}
		}
		cache.Cache.Set(cacheKey, containers, d.cfg.CacheDuration)
	}

	// Fill in the latest statistics from the cgroups
	// Creating a new list of containers with copies so we don't lose
	// the previous state for calculations (e.g. last cpu).
	var err error
	newContainers := make([]*Container, 0, len(containers))
	for _, lastContainer := range containers {
		if (cfg.IncludeExited && lastContainer.State == ContainerExitedState) || lastContainer.Excluded {
			newContainers = append(newContainers, lastContainer)
			continue
		}

		container := &Container{}
		*container = *lastContainer

		cgroup := container.cgroup
		if cgroup == nil {
			log.Errorf("container id %s has an empty cgroup, skipping", container.ID[:12])
			continue
		}

		container.Memory, err = cgroup.Mem()
		if err != nil {
			log.Debugf("cgroup memory: %s", err)
			continue
		}
		container.CPU, err = cgroup.CPU()
		if err != nil {
			log.Debugf("cgroup cpu: %s", err)
			continue
		}
		container.CPUNrThrottled, err = cgroup.CPUNrThrottled()
		if err != nil {
			log.Debugf("cgroup cpuNrThrottled: %s", err)
			continue
		}
		container.IO, err = cgroup.IO()
		if err != nil {
			log.Debugf("cgroup i/o: %s", err)
			continue
		}

		if d.cfg.CollectNetwork {
			d.Lock()
			networks, ok := d.networkMappings[cgroup.ContainerID]
			d.Unlock()
			if ok && len(cgroup.Pids) > 0 {
				netStat, err := collectNetworkStats(cgroup.ContainerID, int(cgroup.Pids[0]), networks)
				if err != nil {
					log.Debugf("could not collect network stats for container %s: %s", container.ID, err)
					continue
				}
				container.Network = netStat
			}
		} else {
			container.Network = NullContainer.Network
		}

		startedAt, err := cgroup.ContainerStartTime()
		if err != nil {
			log.Debugf("failed to get container start time: %s", err)
			continue
		}
		container.StartedAt = startedAt
		container.Pids = cgroup.Pids

		newContainers = append(newContainers, container)
	}
	return newContainers, nil
}

func (d *dockerUtil) getHostname() (string, error) {
	info, err := d.cli.Info(context.Background())
	if err != nil {
		return "", fmt.Errorf("unable to get Docker info: %s", err)
	}
	return info.Name, nil
}

// extractImageName will resolve sha image name to their user-friendly name.
// For non-sha names we will just return the name as-is.
func (d *dockerUtil) extractImageName(image string) string {
	if !strings.HasPrefix(image, "sha256:") {
		return image
	}

	d.Lock()
	defer d.Unlock()
	if _, ok := d.imageNameBySha[image]; !ok {
		r, _, err := d.cli.ImageInspectWithRaw(context.Background(), image)
		if err != nil {
			// Only log errors that aren't "not found" because some images may
			// just not be available in docker inspect.
			if !client.IsErrNotFound(err) {
				log.Errorf("could not extract image %s name: %s", image, err)
			}
			d.imageNameBySha[image] = image
		}

		// Try RepoTags first and fall back to RepoDigest otherwise.
		if len(r.RepoTags) > 0 {
			d.imageNameBySha[image] = r.RepoTags[0]
		} else if len(r.RepoDigests) > 0 {
			// Digests formatted like quay.io/foo/bar@sha256:hash
			sp := strings.SplitN(r.RepoDigests[0], "@", 2)
			d.imageNameBySha[image] = sp[0]
		} else {
			d.imageNameBySha[image] = image
		}
	}
	return d.imageNameBySha[image]
}

func (d *dockerUtil) invalidateCaches(containers []types.Container) {
	liveContainers := make(map[string]struct{})
	liveImages := make(map[string]struct{})
	for _, c := range containers {
		liveContainers[c.ID] = struct{}{}
		liveImages[c.Image] = struct{}{}
	}
	d.Lock()
	for cid := range d.networkMappings {
		if _, ok := liveContainers[cid]; !ok {
			delete(d.networkMappings, cid)
		}
	}
	for image := range d.imageNameBySha {
		if _, ok := liveImages[image]; !ok {
			delete(d.imageNameBySha, image)
		}
	}
	d.Unlock()
}

func detectServerAPIVersion() (string, error) {
	host := os.Getenv("DOCKER_HOST")
	if host == "" {
		host = client.DefaultDockerHost
	}
	cli, err := client.NewClient(host, "", nil, nil)
	if err != nil {
		return "", err
	}

	// Create the client using the server's API version
	v, err := cli.ServerVersion(context.Background())
	if err != nil {
		return "", err
	}
	return v.APIVersion, nil
}

var hostNetwork = dockerNetwork{"eth0", "bridge"}

func findDockerNetworks(containerID string, pid int, netSettings *types.SummaryNetworkSettings) []dockerNetwork {
	// Verify that we aren't using an older version of Docker that does
	// not provider the network settings in container inspect.
	if netSettings == nil || netSettings.Networks == nil {
		log.Debugf("No network settings available from docker, defaulting to host network")
		return []dockerNetwork{hostNetwork}
	}

	var err error
	dockerGateways := make(map[string]int64)
	for netName, netConf := range netSettings.Networks {
		gw := netConf.Gateway
		if netName == "host" || gw == "" {
			log.Debugf("Empty network gateway, container %s is in network host mode, its network metrics are for the whole host", containerID)
			return []dockerNetwork{hostNetwork}
		}

		// Check if this is a CIDR or just an IP
		var ip net.IP
		if strings.Contains(gw, "/") {
			ip, _, err = net.ParseCIDR(gw)
			if err != nil {
				log.Warnf("Invalid gateway %s for container id %s: %s, skipping", gw, containerID, err)
				continue
			}
		} else {
			ip = net.ParseIP(gw)
			if ip == nil {
				log.Warnf("Invalid gateway %s for container id %s: %s, skipping", gw, containerID, err)
				continue
			}
		}

		// Convert IP to int64 for comparison to network routes.
		dockerGateways[netName] = int64(binary.BigEndian.Uint32(ip.To4()))
	}

	// Read contents of file. Handle missing or unreadable file in case container was stopped.
	procNetFile := HostProc(strconv.Itoa(int(pid)), "net", "route")
	if !PathExists(procNetFile) {
		log.Debugf("Missing %s for container %s", procNetFile, containerID)
		return nil
	}
	lines, err := ReadLines(procNetFile)
	if err != nil {
		log.Debugf("Unable to read %s for container %s", procNetFile, containerID)
		return nil
	}
	if len(lines) < 1 {
		log.Errorf("empty network file, unable to get docker networks: %s", procNetFile)
		return nil
	}

	networks := make([]dockerNetwork, 0)
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}
		if fields[0] == "00000000" {
			continue
		}
		dest, _ := strconv.ParseInt(fields[1], 16, 32)
		mask, _ := strconv.ParseInt(fields[7], 16, 32)
		for net, gw := range dockerGateways {
			if gw&mask == dest {
				networks = append(networks, dockerNetwork{fields[0], net})
			}
		}
	}
	sort.Sort(dockerNetworks(networks))
	return networks
}

func collectNetworkStats(containerID string, pid int, networks []dockerNetwork) (*NetworkStat, error) {
	procNetFile := HostProc(strconv.Itoa(int(pid)), "net", "dev")
	if !PathExists(procNetFile) {
		log.Debugf("Unable to read %s for container %s", procNetFile, containerID)
		return &NetworkStat{}, nil
	}
	lines, err := ReadLines(procNetFile)
	if err != nil {
		log.Debugf("Unable to read %s for container %s", procNetFile, containerID)
		return &NetworkStat{}, nil
	}
	if len(lines) < 2 {
		return nil, fmt.Errorf("invalid format for %s", procNetFile)
	}

	nwByIface := make(map[string]dockerNetwork)
	for _, nw := range networks {
		nwByIface[nw.iface] = nw
	}

	// Format:
	//
	// Inter-|   Receive                                                |  Transmit
	// face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
	// eth0:    1296      16    0    0    0     0          0         0        0       0    0    0    0     0       0          0
	// lo:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
	//
	stat := &NetworkStat{}
	for _, line := range lines[2:] {
		fields := strings.Fields(line)
		if len(fields) < 11 {
			continue
		}
		iface := fields[0][:len(fields[0])-1]

		if _, ok := nwByIface[iface]; ok {
			rcvd, _ := strconv.Atoi(fields[1])
			stat.BytesRcvd += uint64(rcvd)
			pktRcvd, _ := strconv.Atoi(fields[2])
			stat.PacketsRcvd += uint64(pktRcvd)
			sent, _ := strconv.Atoi(fields[9])
			stat.BytesSent += uint64(sent)
			pktSent, _ := strconv.Atoi(fields[10])
			stat.PacketsSent += uint64(pktSent)
		}
	}
	return stat, nil
}

var healthRe = regexp.MustCompile(`\(health: (\w+)\)`)

// Parse the health out of a container status. The format is either:
//  - 'Up 5 seconds (health: starting)'
//  - 'Up about an hour'
//
func parseContainerHealth(status string) string {
	// Avoid allocations in most cases by just checking for '('
	if strings.IndexByte(status, '(') == -1 {
		return ""
	}
	all := healthRe.FindAllStringSubmatch(status, -1)
	if len(all) < 1 || len(all[0]) < 2 {
		return ""
	}
	return all[0][1]
}
