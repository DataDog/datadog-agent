// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/compliance/metrics"
	"github.com/DataDog/datadog-agent/pkg/compliance/utils"
	"github.com/DataDog/datadog-agent/pkg/util/jsonquery"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-go/v5/statsd"

	dockertypes "github.com/docker/docker/api/types"
	docker "github.com/docker/docker/client"

	auditrule "github.com/elastic/go-libaudit/rule"

	"github.com/shirou/gopsutil/v3/process"

	yamlv2 "gopkg.in/yaml.v2"
	yamlv3 "gopkg.in/yaml.v3"

	kubemetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeunstructured "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kubeschema "k8s.io/apimachinery/pkg/runtime/schema"
	kubedynamic "k8s.io/client-go/dynamic"
)

// inputsResolveTimeout is the timeout that is applied for inputs resolution of one
// Rule.
const inputsResolveTimeout = 5 * time.Second

// DockerProvider is a function returning a Docker client.
type DockerProvider func(context.Context) (docker.CommonAPIClient, error)

// KubernetesProvider is a function returning a Kubernetes client.
type KubernetesProvider func(context.Context) (kubedynamic.Interface, error)

// LinuxAuditProvider is a function returning a Linux Audit client.
type LinuxAuditProvider func(context.Context) (LinuxAuditClient, error)

// LinuxAuditClient is an interface that implements the capability of parsing
// Linux Audit rules.
type LinuxAuditClient interface {
	GetFileWatchRules() ([]*auditrule.FileWatchRule, error)
	Close() error
}

// DefaultDockerProvider returns the default Docker client.
func DefaultDockerProvider(ctx context.Context) (docker.CommonAPIClient, error) {
	return newDockerClient(ctx)
}

// DefaultLinuxAuditProvider returns the default Linux Audit client.
func DefaultLinuxAuditProvider(ctx context.Context) (LinuxAuditClient, error) {
	return newLinuxAuditClient()
}

// ResolverOptions is an options struct required to instantiate a Resolver
// instance.
type ResolverOptions struct {
	// Hostname is the name of the host running the resolver.
	Hostname string

	// HostRoot is the path to the mountpoint of host root filesystem. In case
	// the compliance module is run as part of a container.
	HostRoot string

	// HostRootPID sets the resolving context relative to a specific process
	// ID (optional)
	HostRootPID int32

	// StatsdClient is the statsd client used internally by the compliance
	// resolver (optional)
	StatsdClient *statsd.Client

	DockerProvider
	KubernetesProvider
	LinuxAuditProvider
}

// Resolver interface defines a generic method to resolve the inputs
// associated with a given rule. The Close() method should be called whenever
// the resolver is stopped being used to cleanup underlying resources.
type Resolver interface {
	ResolveInputs(ctx context.Context, rule *Rule) (ResolvedInputs, error)
	Close()
}

type defaultResolver struct {
	opts ResolverOptions

	procsCache         []*process.Process
	filesCache         []fileMeta
	pkgsCache          map[string]*packageInfo
	kubeClusterIDCache string

	dockerCl     docker.CommonAPIClient
	kubernetesCl kubedynamic.Interface
	linuxAuditCl LinuxAuditClient
}

type fileMeta struct {
	path  string
	data  []byte
	perms uint64
	user  string
	group string
}

// NewResolver returns the default inputs resolver that is able to resolve any
// kind of supported inputs. It holds a small cache for loaded file metadata
// and different client connexions that may be used for inputs resolution.
func NewResolver(ctx context.Context, opts ResolverOptions) Resolver {
	r := &defaultResolver{
		opts: opts,
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if opts.DockerProvider != nil {
		r.dockerCl, _ = opts.DockerProvider(ctx)
	}
	if opts.KubernetesProvider != nil {
		r.kubernetesCl, _ = opts.KubernetesProvider(ctx)
	}
	if opts.LinuxAuditProvider != nil {
		r.linuxAuditCl, _ = opts.LinuxAuditProvider(ctx)
	}
	return r
}

func (r *defaultResolver) Close() {
	if r.dockerCl != nil {
		r.dockerCl.Close()
		r.dockerCl = nil
	}
	if r.linuxAuditCl != nil {
		r.linuxAuditCl.Close()
		r.linuxAuditCl = nil
	}
	r.kubernetesCl = nil

	r.procsCache = nil
	r.filesCache = nil
	r.pkgsCache = nil
	r.kubeClusterIDCache = ""
}

func (r *defaultResolver) ResolveInputs(ctx context.Context, rule *Rule) (ResolvedInputs, error) {
	resolvingContext := ResolvingContext{
		RuleID:     rule.ID,
		Hostname:   r.opts.Hostname,
		InputSpecs: make(map[string]*InputSpec),
	}

	// We deactivate all docker rules, or kubernetes cluster rules if adequate
	// clients could not be setup.
	if rule.HasScope(DockerScope) && r.dockerCl == nil {
		return nil, ErrIncompatibleEnvironment
	}
	if rule.HasScope(KubernetesClusterScope) && r.kubernetesCl == nil {
		return nil, ErrIncompatibleEnvironment
	}

	if len(rule.InputSpecs) == 0 {
		return nil, fmt.Errorf("no inputs for rule %s", rule.ID)
	}

	ctx, cancel := context.WithTimeout(ctx, inputsResolveTimeout)
	defer cancel()

	// If a container ID is associated with this resolver instance, we try to
	// resolve the image metadata associated with the container to be part of
	// the resolved inputs.
	rootPath := r.opts.HostRoot
	if pid := r.opts.HostRootPID; pid > 0 {
		containerID, ok := getProcessContainerID(pid)
		if ok {
			rootPath, ok = getProcessRootPath(pid)
			if !ok {
				return nil, fmt.Errorf("could not resolve the root path to run the resolver for container ID=%q", resolvingContext.ContainerID)
			}
			resolvingContext.ContainerID = containerID
		}
	}

	resolved := make(map[string]interface{})
	for _, spec := range rule.InputSpecs {
		start := time.Now()

		var err error
		var resultType string
		var result interface{}
		var kubernetesCluster string

		switch {
		case spec.File != nil:
			resultType = "file"
			result, err = r.resolveFile(ctx, rootPath, *spec.File)
		case spec.Process != nil:
			resultType = "process"
			result, err = r.resolveProcess(ctx, *spec.Process)
		case spec.Group != nil:
			resultType = "group"
			result, err = r.resolveGroup(ctx, *spec.Group)
		case spec.Audit != nil:
			resultType = "audit"
			result, err = r.resolveAudit(ctx, *spec.Audit)
		case spec.Docker != nil:
			resultType = "docker"
			result, err = r.resolveDocker(ctx, *spec.Docker)
		case spec.KubeApiserver != nil:
			resultType = "kubernetes"
			result, err = r.resolveKubeApiserver(ctx, *spec.KubeApiserver)
			kubernetesCluster = r.resolveKubeClusterID(ctx)
		case spec.Package != nil:
			resultType = "package"
			result, err = r.resolvePackage(ctx, *spec.Package)
		case spec.Constants != nil:
			resultType = "constants"
			result = *spec.Constants
		default:
			return nil, fmt.Errorf("bad input spec")
		}

		tagName := resultType
		if spec.TagName != "" {
			tagName = spec.TagName
		}
		if err != nil {
			return nil, fmt.Errorf("could not resolve input spec %s(tagged=%q): %w", resultType, tagName, err)
		}

		if _, ok := resolved[tagName]; ok {
			return nil, fmt.Errorf("input with tag %q already set", tagName)
		}
		if _, ok := resolvingContext.InputSpecs[tagName]; ok {
			return nil, fmt.Errorf("input with tag %q already set", tagName)
		}

		resolvingContext.InputSpecs[tagName] = spec
		if kubernetesCluster != "" {
			resolvingContext.KubernetesCluster = kubernetesCluster
		}

		if r, ok := result.([]interface{}); ok && reflect.ValueOf(r).IsNil() {
			result = nil
		}
		if result != nil {
			resolved[tagName] = result
		}

		if statsdClient := r.opts.StatsdClient; statsdClient != nil && resultType != "constants" {
			tags := []string{
				"rule_id:" + rule.ID,
				"rule_input_type:" + resultType,
				"agent_version:" + version.AgentVersion,
			}
			if err := statsdClient.Count(metrics.MetricInputsHits, 1, tags, 1.0); err != nil {
				log.Errorf("failed to send input metric: %v", err)
			}
			if err := statsdClient.Timing(metrics.MetricInputsDuration, time.Since(start), tags, 1.0); err != nil {
				log.Errorf("failed to send input metric: %v", err)
			}
		}
	}

	return NewResolvedInputs(resolvingContext, resolved)
}

func (r *defaultResolver) pathNormalize(rootPath, path string) string {
	if rootPath != "" {
		return filepath.Join(rootPath, path)
	}
	return path
}

func (r *defaultResolver) pathRelative(rootPath, path string) string {
	if rootPath != "" {
		p, err := filepath.Rel(rootPath, path)
		if err != nil {
			return path
		}
		return string(os.PathSeparator) + p
	}
	return path
}

func (r *defaultResolver) pathNormalizeToHostRoot(path string) string {
	return r.pathNormalize(r.opts.HostRoot, path)
}

func (r *defaultResolver) getFileMeta(path string) (*fileMeta, error) {
	const maxFilesCached = 8
	for _, f := range r.filesCache {
		if f.path == path {
			return &f, nil
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	perms := uint64(info.Mode() & os.ModePerm)
	var data []byte
	if !info.IsDir() {
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, err
		}
	}
	file := &fileMeta{
		path:  path,
		data:  data,
		perms: perms,
		user:  utils.GetFileUser(info),
		group: utils.GetFileGroup(info),
	}
	r.filesCache = append(r.filesCache, *file)
	if len(r.filesCache) > maxFilesCached {
		r.filesCache = r.filesCache[1:]
	}
	return file, nil
}

var processFlagBuiltinReg = regexp.MustCompile(`process\.flag\("(\S+)",\s*"(\S+)"\)`)

func (r *defaultResolver) resolveFile(ctx context.Context, rootPath string, spec InputSpecFile) (result interface{}, err error) {
	path := strings.TrimSpace(spec.Path)
	if matches := processFlagBuiltinReg.FindStringSubmatch(path); len(matches) == 3 {
		processName, processFlag := matches[1], matches[2]
		result, err = r.resolveFileFromProcessFlag(ctx, rootPath, processName, processFlag, spec.Parser)
	} else if spec.Glob != "" && path == "" {
		result, err = r.resolveFileGlob(ctx, rootPath, spec.Glob, spec.Parser)
	} else if strings.Contains(path, "*") {
		result, err = r.resolveFileGlob(ctx, rootPath, path, spec.Parser)
	} else {
		result, err = r.resolveFilePath(ctx, rootPath, path, spec.Parser)
	}
	if errors.Is(err, os.ErrPermission) ||
		errors.Is(err, os.ErrNotExist) ||
		errors.Is(err, os.ErrClosed) {
		result, err = nil, nil
	}
	return
}

func (r *defaultResolver) resolveFilePath(ctx context.Context, rootPath, path, parser string) (interface{}, error) {
	path = r.pathNormalize(rootPath, path)
	file, err := r.getFileMeta(path)
	if err != nil {
		return nil, err
	}
	var content interface{}
	if len(file.data) > 0 {
		switch parser {
		case "yaml":
			err = yamlv3.Unmarshal(file.data, &content)
			if err != nil {
				err = yamlv2.Unmarshal(file.data, &content)
			}
			if err == nil {
				content = jsonquery.NormalizeYAMLForGoJQ(content)
			}
		case "json":
			err = json.Unmarshal(file.data, &content)
		case "raw":
			content = string(file.data)
		default:
			content = ""
		}
		if err != nil {
			return nil, err
		}
	}
	return map[string]interface{}{
		"path":        r.pathRelative(rootPath, path),
		"glob":        "",
		"permissions": file.perms,
		"user":        file.user,
		"group":       file.group,
		"content":     content,
	}, nil
}

func (r *defaultResolver) resolveFileFromProcessFlag(ctx context.Context, rootPath, name, flag, parser string) (interface{}, error) {
	procs, err := r.getProcs(ctx)
	if err != nil {
		return nil, err
	}
	var proc *process.Process
	for _, p := range procs {
		n, _ := p.Name()
		if n == name {
			proc = p
			break
		}
	}
	if proc == nil {
		return nil, nil
	}

	cmdLine, err := proc.CmdlineSlice()
	if err != nil {
		return nil, nil
	}

	flags := parseCmdlineFlags(cmdLine)
	path, ok := flags[flag]
	if !ok {
		return nil, nil
	}
	return r.resolveFilePath(ctx, rootPath, path, parser)
}

func (r *defaultResolver) resolveFileGlob(ctx context.Context, rootPath, glob, parser string) (interface{}, error) {
	paths, _ := filepath.Glob(r.pathNormalize(rootPath, glob)) // We ignore errors from Glob which are never I/O errors
	var resolved []interface{}
	for _, path := range paths {
		path = r.pathRelative(rootPath, path)
		file, err := r.resolveFilePath(ctx, rootPath, path, parser)
		if err != nil {
			continue
		}
		if f, ok := file.(map[string]interface{}); ok {
			f["glob"] = glob
		}
		resolved = append(resolved, file)
	}
	return resolved, nil
}

func (r *defaultResolver) resolveProcess(ctx context.Context, spec InputSpecProcess) (interface{}, error) {
	procs, err := r.getProcs(ctx)
	if err != nil {
		return nil, err
	}
	var resolved []interface{}
	for _, p := range procs {
		n, _ := p.Name()
		if n != spec.Name {
			continue
		}
		cmdLine, err := p.CmdlineSlice()
		if err != nil {
			return nil, err
		}
		var envs []string
		if len(spec.Envs) > 0 {
			envs, err = p.Environ()
			// NOTE(pierre): security-agent may be executed without the capabilities to get /proc/<pid>/environ
			if err != nil && !os.IsPermission(err) {
				return nil, err
			}
		}
		exe, _ := p.Exe()
		resolved = append(resolved, map[string]interface{}{
			"name":    spec.Name,
			"pid":     p.Pid,
			"exe":     exe,
			"cmdLine": cmdLine,
			"flags":   parseCmdlineFlags(cmdLine),
			"envs":    parseEnvironMap(envs, spec.Envs),
		})
	}
	return resolved, nil
}

func (r *defaultResolver) getProcs(ctx context.Context) ([]*process.Process, error) {
	if r.procsCache == nil {
		procs, err := process.ProcessesWithContext(ctx)
		if err != nil {
			return nil, err
		}
		r.procsCache = procs
	}
	return r.procsCache, nil
}

func (r *defaultResolver) resolveGroup(ctx context.Context, spec InputSpecGroup) (interface{}, error) {
	f, err := os.Open("/etc/group")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	prefix := spec.Name + ":"
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		parts := strings.SplitN(string(line), ":", 4)
		if len(parts) != 4 {
			return nil, fmt.Errorf("malformed group file format")
		}
		gid, err := strconv.Atoi(parts[2])
		if err != nil {
			return nil, fmt.Errorf("failed to parse group ID for %s: %w", spec.Name, err)
		}
		users := strings.Split(parts[3], ",")
		return map[string]interface{}{
			"name":  spec.Name,
			"users": users,
			"id":    gid,
		}, nil
	}
	return nil, nil
}

func (r *defaultResolver) resolveAudit(ctx context.Context, spec InputSpecAudit) (interface{}, error) {
	cl := r.linuxAuditCl
	if cl == nil {
		return nil, ErrIncompatibleEnvironment
	}
	normPath := r.pathNormalizeToHostRoot(spec.Path)
	if _, err := os.Stat(normPath); os.IsNotExist(err) {
		return nil, nil
	}
	rules, err := cl.GetFileWatchRules()
	if err != nil {
		return nil, err
	}
	var resolved []interface{}
	for _, rule := range rules {
		if rule.Path == spec.Path {
			permissions := ""
			for _, p := range rule.Permissions {
				switch p {
				case auditrule.ReadAccessType:
					permissions += "r"
				case auditrule.WriteAccessType:
					permissions += "w"
				case auditrule.ExecuteAccessType:
					permissions += "e"
				case auditrule.AttributeChangeAccessType:
					permissions += "a"
				}
			}
			resolved = append(resolved, map[string]interface{}{
				"path":        spec.Path,
				"enabled":     true,
				"permissions": permissions,
			})
		}
	}

	return resolved, nil
}

func (r *defaultResolver) resolveDocker(ctx context.Context, spec InputSpecDocker) (interface{}, error) {
	cl := r.dockerCl
	if cl == nil {
		return nil, ErrIncompatibleEnvironment
	}

	var resolved []interface{}
	switch spec.Kind {
	case "image":
		list, err := cl.ImageList(ctx, dockertypes.ImageListOptions{All: true})
		if err != nil {
			return nil, err
		}
		for _, im := range list {
			image, _, err := cl.ImageInspectWithRaw(ctx, im.ID)
			if err != nil {
				return nil, err
			}
			resolved = append(resolved, map[string]interface{}{
				"id":      image.ID,
				"tags":    image.RepoTags,
				"inspect": image,
			})
		}
	case "container":
		list, err := cl.ContainerList(ctx, dockertypes.ContainerListOptions{All: true})
		if err != nil {
			return nil, err
		}
		for _, cn := range list {
			container, _, err := cl.ContainerInspectWithRaw(ctx, cn.ID, false)
			if err != nil {
				return nil, err
			}
			resolved = append(resolved, map[string]interface{}{
				"id":      container.ID,
				"name":    container.Name,
				"image":   container.Image,
				"inspect": container,
			})
		}
	case "network":
		networks, err := cl.NetworkList(ctx, dockertypes.NetworkListOptions{})
		if err != nil {
			return nil, err
		}
		for _, nw := range networks {
			resolved = append(resolved, map[string]interface{}{
				"id":      nw.ID,
				"name":    nw.Name,
				"inspect": nw,
			})
		}
	case "info":
		info, err := cl.Info(ctx)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, map[string]interface{}{
			"inspect": info,
		})
	case "version":
		version, err := cl.ServerVersion(ctx)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, map[string]interface{}{
			"version":       version.Version,
			"apiVersion":    version.APIVersion,
			"platform":      version.Platform.Name,
			"experimental":  version.Experimental,
			"os":            version.Os,
			"arch":          version.Arch,
			"kernelVersion": version.KernelVersion,
		})
	default:
		return nil, fmt.Errorf("unsupported docker object kind '%q'", spec.Kind)
	}

	return resolved, nil
}

func (r *defaultResolver) resolveKubeClusterID(ctx context.Context) string {
	if r.kubeClusterIDCache == "" {
		cl := r.kubernetesCl
		if cl == nil {
			return ""
		}

		resourceDef := cl.Resource(kubeschema.GroupVersionResource{
			Resource: "namespaces",
			Version:  "v1",
		})
		resource, err := resourceDef.Get(ctx, "kube-system", kubemetav1.GetOptions{})
		if err != nil {
			return ""
		}
		r.kubeClusterIDCache = string(resource.GetUID())
	}
	return r.kubeClusterIDCache
}

func (r *defaultResolver) resolveKubeApiserver(ctx context.Context, spec InputSpecKubeapiserver) (interface{}, error) {
	cl := r.kubernetesCl
	if cl == nil {
		return nil, ErrIncompatibleEnvironment
	}

	if len(spec.Kind) == 0 {
		return nil, fmt.Errorf("cannot run Kubeapiserver check, resource kind is empty")
	}

	if len(spec.APIRequest.Verb) == 0 {
		return nil, fmt.Errorf("cannot run Kubeapiserver check, action verb is empty")
	}

	if len(spec.Version) == 0 {
		spec.Version = "v1"
	}

	resourceSchema := kubeschema.GroupVersionResource{
		Group:    spec.Group,
		Resource: spec.Kind,
		Version:  spec.Version,
	}

	resourceDef := cl.Resource(resourceSchema)
	var resourceAPI kubedynamic.ResourceInterface
	if len(spec.Namespace) > 0 {
		resourceAPI = resourceDef.Namespace(spec.Namespace)
	} else {
		resourceAPI = resourceDef
	}

	var items []kubeunstructured.Unstructured
	api := spec.APIRequest
	switch api.Verb {
	case "get":
		if len(api.ResourceName) == 0 {
			return nil, fmt.Errorf("unable to use 'get' apirequest without resource name")
		}
		resource, err := resourceAPI.Get(ctx, spec.APIRequest.ResourceName, kubemetav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("unable to get Kube resource:'%v', ns:'%s' name:'%s', err: %v",
				resourceSchema, spec.Namespace, api.ResourceName, err)
		}
		items = []kubeunstructured.Unstructured{*resource}
	case "list":
		list, err := resourceAPI.List(ctx, kubemetav1.ListOptions{
			LabelSelector: spec.LabelSelector,
			FieldSelector: spec.FieldSelector,
		})
		if err != nil {
			return nil, fmt.Errorf("unable to list Kube resources:'%v', ns:'%s' name:'%s', err: %v",
				resourceSchema, spec.Namespace, api.ResourceName, err)
		}
		items = list.Items
	}

	resolved := make([]interface{}, 0, len(items))
	for _, resource := range items {
		resolved = append(resolved, map[string]interface{}{
			"kind":      resource.GetObjectKind().GroupVersionKind().Kind,
			"group":     resource.GetObjectKind().GroupVersionKind().Group,
			"version":   resource.GetObjectKind().GroupVersionKind().Version,
			"namespace": resource.GetNamespace(),
			"name":      resource.GetName(),
			"resource":  resource,
		})
	}
	return resolved, nil
}

const (
	apkDb     = "/lib/apk/db/installed"
	dpkgDb    = "/var/lib/dpkg/status"
	dpkgDbDir = "/var/lib/dpkg/status.d"
)

var rpmDbs = []string{
	"/usr/lib/sysimage/rpm/rpmdb.sqlite",
	"/var/lib/rpm/rpmdb.sqlite",
	"/usr/lib/sysimage/rpm/Packages.db",
	"/var/lib/rpm/Packages.db",
	"/usr/lib/sysimage/rpm/Packages",
	"/var/lib/rpm/Packages",
}

func (r *defaultResolver) resolvePackage(ctx context.Context, spec InputSpecPackage) (pkg *packageInfo, err error) {
	if len(spec.Names) == 0 {
		return nil, nil
	}

	if r.pkgsCache != nil {
		for _, name := range spec.Names {
			if p, ok := r.pkgsCache[name]; ok {
				return p, nil
			}
		}
	}

	defer func() {
		if pkg != nil {
			if r.pkgsCache == nil {
				r.pkgsCache = make(map[string]*packageInfo)
			}
			r.pkgsCache[pkg.Name] = pkg
		}
	}()

	// apk
	apkPath := r.pathNormalizeToHostRoot(apkDb)
	if pkg := findApkPackage(apkPath, spec.Names); pkg != nil {
		return pkg, nil
	}

	// dpkg
	dpkgPath := r.pathNormalizeToHostRoot(dpkgDb)
	if pkg := findDpkgPackage(dpkgPath, spec.Names); pkg != nil {
		return pkg, nil
	}
	dpkgDirPath := r.pathNormalizeToHostRoot(dpkgDbDir)
	if files, _ := os.ReadDir(dpkgDirPath); len(files) > 0 {
		for _, entry := range files {
			dpkgPath := filepath.Join(dpkgDirPath, entry.Name())
			if pkg := findDpkgPackage(dpkgPath, spec.Names); pkg != nil {
				return pkg, nil
			}
		}
	}

	// rpm
	for _, path := range rpmDbs {
		rpmPath := r.pathNormalizeToHostRoot(path)
		if pkg := findRpmPackage(rpmPath, spec.Names); pkg != nil {
			return pkg, nil
		}
	}

	return nil, nil
}

func parseCmdlineFlags(cmdline []string) map[string]string {
	flagsMap := make(map[string]string, 0)
	pendingFlagValue := false
	for i, arg := range cmdline {
		if strings.HasPrefix(arg, "-") {
			parts := strings.SplitN(arg, "=", 2)
			// We have -xxx=yyy, considering the flag completely resolved
			if len(parts) == 2 {
				flagsMap[parts[0]] = parts[1]
			} else {
				flagsMap[parts[0]] = ""
				pendingFlagValue = true
			}
		} else {
			if pendingFlagValue {
				flagsMap[cmdline[i-1]] = arg
			} else {
				flagsMap[arg] = ""
			}
		}
	}
	return flagsMap
}

func parseEnvironMap(envs, filteredEnvs []string) map[string]string {
	envsMap := make(map[string]string, len(filteredEnvs))
	if len(filteredEnvs) == 0 {
		return envsMap
	}
	for _, envValue := range envs {
		for _, envName := range filteredEnvs {
			prefix := envName + "="
			if strings.HasPrefix(envValue, prefix) {
				envsMap[envName] = strings.TrimPrefix(envValue, prefix)
			} else if envValue == envName {
				envsMap[envName] = ""
			}
		}
	}
	return envsMap
}
