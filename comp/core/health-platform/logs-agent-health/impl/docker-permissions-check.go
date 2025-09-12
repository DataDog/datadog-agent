// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package logsagenthealthimpl provides the implementation for the logs agent health checker sub-component.
package logsagenthealthimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	healthplatform "github.com/DataDog/datadog-agent/comp/core/health-platform/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// DockerLogsDir is the default Docker logs directory
	DockerLogsDir = "/var/lib/docker"
	// DockerSocketPath is the default Docker socket path
	DockerSocketPath = "/var/run/docker.sock"

	// IssueIDDockerFileTailingDisabled is the ID for the Docker file tailing disabled issue
	IssueIDDockerFileTailingDisabled = "docker-file-tailing-disabled"
	// IssueIDDockerSocketInaccessible is the ID for the Docker socket inaccessible issue
	IssueIDDockerSocketInaccessible = "docker-socket-inaccessible"
)

// DockerPermissionsCheck groups all Docker-related permission and access checks
type DockerPermissionsCheck struct{}

// NewDockerPermissionsCheck creates a new Docker permissions check
func NewDockerPermissionsCheck() *DockerPermissionsCheck {
	return &DockerPermissionsCheck{}
}

// isPermissionError checks if an error is permission-related using proper error type checking
func isPermissionError(err error) bool {
	return errors.Is(err, fs.ErrPermission) ||
		errors.Is(err, syscall.EACCES) ||
		errors.Is(err, syscall.EPERM)
}

// pingDocker performs an actual HTTP GET /_ping via the unix socket to test Docker API access
// Returns: ok=true if Docker is reachable, perm=true if permission error, err=any error
func pingDocker(sockPath string, timeout time.Duration) (ok bool, perm bool, err error) {
	dial := func(_ context.Context, _, _ string) (net.Conn, error) {
		return net.DialTimeout("unix", sockPath, timeout/2)
	}
	tr := &http.Transport{DialContext: dial}
	defer tr.CloseIdleConnections()

	client := &http.Client{Transport: tr, Timeout: timeout}

	req, err := http.NewRequest("GET", "http://unix/_ping", nil)
	if err != nil {
		return false, false, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := client.Do(req)
	if err != nil {
		return false, isPermissionError(err), err
	}
	defer resp.Body.Close()

	return resp.StatusCode == 200, false, nil
}

// dockerInfo represents Docker daemon information
type dockerInfo struct {
	LoggingDriver   string   `json:"LoggingDriver"`
	DockerRootDir   string   `json:"DockerRootDir"`
	SecurityOptions []string `json:"SecurityOptions"`
	// Rootless bool `json:"Rootless"` // on newer engines
}

// containerLite represents a minimal container info for listing
type containerLite struct {
	ID string `json:"Id"`
}

// containerInspect represents container inspection info for getting LogPath
type containerInspect struct {
	LogPath string `json:"LogPath"`
}

// isRootless checks if Docker is running in rootless mode
func isRootless(info dockerInfo) bool {
	for _, s := range info.SecurityOptions {
		if strings.Contains(s, "rootless") {
			return true
		}
	}
	return false
}

// httpClientOverUnix creates an HTTP client that connects via Unix socket
func httpClientOverUnix(sock string, timeout time.Duration) *http.Client {
	dial := func(_ context.Context, _, _ string) (net.Conn, error) {
		return net.DialTimeout("unix", sock, timeout/2)
	}
	tr := &http.Transport{DialContext: dial}
	client := &http.Client{Transport: tr, Timeout: timeout}
	// Note: Caller should call tr.CloseIdleConnections() when done
	return client
}

// getDockerInfo retrieves Docker daemon information via the socket
func getDockerInfo(sock string, timeout time.Duration) (dockerInfo, error) {
	var info dockerInfo
	c := httpClientOverUnix(sock, timeout)
	defer func() {
		if tr, ok := c.Transport.(*http.Transport); ok {
			tr.CloseIdleConnections()
		}
	}()

	req, err := http.NewRequest("GET", "http://unix/info", nil)
	if err != nil {
		return info, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := c.Do(req)
	if err != nil {
		return info, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return info, fmt.Errorf("docker /info status %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return info, err
	}
	return info, nil
}

// listContainerIDs retrieves a list of container IDs via the Docker socket
func listContainerIDs(sock string, timeout time.Duration, limit int) ([]string, error) {
	c := httpClientOverUnix(sock, timeout)
	defer func() {
		if tr, ok := c.Transport.(*http.Transport); ok {
			tr.CloseIdleConnections()
		}
	}()

	req, err := http.NewRequest("GET", fmt.Sprintf("http://unix/containers/json?limit=%d", limit), nil)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("docker /containers/json status %d", resp.StatusCode)
	}

	var items []containerLite
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(items))
	for _, it := range items {
		if it.ID != "" {
			ids = append(ids, it.ID)
		}
	}
	return ids, nil
}

// getContainerLogPath retrieves the LogPath for a specific container
func getContainerLogPath(sock, id string, timeout time.Duration) (string, error) {
	c := httpClientOverUnix(sock, timeout)
	defer func() {
		if tr, ok := c.Transport.(*http.Transport); ok {
			tr.CloseIdleConnections()
		}
	}()

	req, err := http.NewRequest("GET", "http://unix/containers/"+id+"/json", nil)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("inspect %s status %d", id, resp.StatusCode)
	}

	var ci containerInspect
	if err := json.NewDecoder(resp.Body).Decode(&ci); err != nil {
		return "", err
	}
	return ci.LogPath, nil
}

// tryOpenOneLogViaInspect attempts to open at least one log file using container LogPath
// Returns: ok=true if able to open at least one log, perm=true if saw a permission error
func tryOpenOneLogViaInspect(sock string, ids []string) (ok bool, perm bool) {
	for i, id := range ids {
		if i >= 10 { // small cap to avoid too many attempts
			break
		}
		p, err := getContainerLogPath(sock, id, time.Second)
		if err != nil {
			if isPermissionError(err) {
				perm = true
			}
			continue
		}
		if p == "" {
			continue
		}
		f, err := os.Open(p)
		if err == nil {
			f.Close()
			return true, false
		}
		if isPermissionError(err) {
			perm = true
		}
		// ENOENT etc: keep trying others
	}
	return false, perm
}

// Name returns the name of this sub-check
func (d *DockerPermissionsCheck) Name() string {
	return "docker-permissions"
}

// Check performs Docker file tailing permission health checks
func (d *DockerPermissionsCheck) Check(_ context.Context) ([]healthplatform.Issue, error) {
	var issues []healthplatform.Issue

	// Check Docker socket accessibility
	if issue := d.checkDockerSocketAccess(); issue != nil {
		issues = append(issues, *issue)
	}

	// Check Docker file tailing permissions
	if issue := d.checkDockerFileTailing(); issue != nil {
		issues = append(issues, *issue)
	}

	return issues, nil
}

// resolveDockerSocketPath resolves the Docker socket path from configuration or environment
func (d *DockerPermissionsCheck) resolveDockerSocketPath() string {
	// Check environment variable first
	if dockerHost := os.Getenv("DOCKER_HOST"); dockerHost != "" {
		if strings.HasPrefix(dockerHost, "unix://") {
			return strings.TrimPrefix(dockerHost, "unix://")
		}
		// If it's TCP, return empty string to indicate socket checks are not applicable
		if strings.HasPrefix(dockerHost, "tcp://") {
			return ""
		}
	}

	// TODO: Add support for reading from Agent config when available
	// For now, use the default path
	return DockerSocketPath
}

// checkDockerSocketAccess checks if the Docker socket is accessible
func (d *DockerPermissionsCheck) checkDockerSocketAccess() *healthplatform.Issue {
	sockPath := d.resolveDockerSocketPath()

	// If DOCKER_HOST is TCP, skip socket permission checks
	if sockPath == "" {
		log.Debug("docker-permissions: DOCKER_HOST is TCP; skipping unix-socket permission checks")
		return nil
	}

	// Check if Docker socket exists
	if _, err := os.Stat(sockPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Socket doesn't exist, not a permission issue
			return nil
		}
		// If it's a permission error on the path itself, report it
		if isPermissionError(err) {
			return d.createDockerIssue("socket")
		}
		return nil
	}

	// Try a proper Docker API ping instead of just socket connection
	_, perm, _ := pingDocker(sockPath, 700*time.Millisecond)
	if perm {
		return d.createDockerIssue("socket")
	}
	// If not ok but not permission error, don't report (Docker might be down)

	return nil
}

// createDockerIssue creates a generic Docker permission issue
func (d *DockerPermissionsCheck) createDockerIssue(issueType string, dockerRootDir ...string) *healthplatform.Issue {
	sockPath := d.resolveDockerSocketPath()

	// Use provided Docker root dir or fall back to default
	dockerDir := DockerLogsDir
	if len(dockerRootDir) > 0 && dockerRootDir[0] != "" {
		dockerDir = dockerRootDir[0]
	}
	var id, name, title, description, extra, integrationFeature, severity, remediationSummary, scriptFilename, scriptContent string
	var remediationSteps []healthplatform.RemediationStep
	var tags []string

	switch issueType {
	case "socket":
		id = IssueIDDockerSocketInaccessible
		name = "docker_socket_inaccessible"
		title = "Docker Socket Not Accessible"
		description = fmt.Sprintf("The agent cannot access the Docker socket at %s due to permission restrictions. This prevents the agent from collecting Docker metrics and container information.", sockPath)
		severity = "high"
		extra = fmt.Sprintf("Docker socket %s is not accessible due to permission restrictions. The agent cannot collect Docker metrics or container information.", sockPath)
		integrationFeature = "docker"
		remediationSummary = "Add the dd-agent user to the docker group to enable Docker socket access"
		remediationSteps = []healthplatform.RemediationStep{
			{Order: 1, Text: "Add the dd-agent user to the docker group: usermod -aG docker dd-agent"},
			{Order: 2, Text: "Verify the user was added to the docker group: groups dd-agent"},
			{Order: 3, Text: "Restart the datadog-agent service: systemctl restart datadog-agent"},
			{Order: 4, Text: "Verify Docker socket access by checking agent logs"},
		}
		scriptFilename = "update-agent-perm-1"
		scriptContent = "usermod -aG docker dd-agent && systemctl restart datadog-agent"
		tags = []string{"docker", "socket", "permissions", "access-control", "metrics"}

	case "file-tailing":
		id = IssueIDDockerFileTailingDisabled
		name = "docker_file_tailing_disabled"
		title = "Host Agent Cannot Tail Docker Log Files"
		description = fmt.Sprintf("Docker file tailing is enabled by default but cannot work on this host install. The directory %s is owned by the root group, causing the agent to fall back to socket tailing. This becomes problematic with high volume Docker logs as socket tailing can hit limits.", dockerDir)
		severity = "medium"
		extra = fmt.Sprintf("Docker logs directory %s is not accessible due to permission restrictions. The agent will fall back to socket tailing, which may hit limits with high volume logs.", dockerDir)
		integrationFeature = "logs"
		remediationSummary = "Grant minimal access to Docker log files using ACLs (recommended) or add dd-agent to root group as last resort"
		remediationSteps = []healthplatform.RemediationStep{
			{Order: 1, Text: "RECOMMENDED: Grant minimal access using ACLs (safer than root group):"},
			{Order: 2, Text: fmt.Sprintf("sudo setfacl -Rm g:dd-agent:rx %s/containers", dockerDir)},
			{Order: 3, Text: fmt.Sprintf("sudo setfacl -Rm g:dd-agent:r %s/containers/*/*.log", dockerDir)},
			{Order: 4, Text: fmt.Sprintf("sudo setfacl -Rdm g:dd-agent:rx %s/containers", dockerDir)},
			{Order: 5, Text: "Restart the datadog-agent service: systemctl restart datadog-agent"},
			{Order: 6, Text: "Verify Docker file tailing is working by checking agent logs"},
			{Order: 7, Text: "⚠️  LAST RESORT: If ACLs don't work, add dd-agent to root group (gives root privileges):"},
			{Order: 8, Text: "usermod -aG root dd-agent && systemctl restart datadog-agent"},
		}
		scriptFilename = "update-agent-perm-2"
		scriptContent = fmt.Sprintf("setfacl -Rm g:dd-agent:rx %s/containers && setfacl -Rm g:dd-agent:r %s/containers/*/*.log && setfacl -Rdm g:dd-agent:rx %s/containers && systemctl restart datadog-agent", dockerDir, dockerDir, dockerDir)
		tags = []string{"docker", "logs", "permissions", "file-tailing", "socket-tailing", "host-install"}

	default:
		log.Errorf("Unknown Docker issue type: %s", issueType)
		return nil
	}

	issue := &healthplatform.Issue{
		ID:                 id,
		IssueName:          name,
		Title:              title,
		Description:        description,
		Category:           "permissions",
		Location:           "logs-agent",
		Severity:           severity,
		DetectedAt:         "", // Will be filled by the platform
		Integration:        nil,
		Extra:              extra,
		IntegrationFeature: integrationFeature,
		Remediation: &healthplatform.Remediation{
			Summary: remediationSummary,
			Steps:   remediationSteps,
			Script: &healthplatform.Script{
				Language:     "bash",
				Filename:     scriptFilename,
				RequiresRoot: true,
				Content:      scriptContent,
			},
		},
		Tags: tags,
	}

	return issue
}

// checkDockerFileTailing checks if Docker file tailing is disabled due to permission issues
func (d *DockerPermissionsCheck) checkDockerFileTailing() *healthplatform.Issue {
	sock := d.resolveDockerSocketPath()

	// If DOCKER_HOST is TCP, skip file tailing checks
	if sock == "" {
		log.Debug("docker-permissions: DOCKER_HOST is TCP; skipping file-tailing permission checks")
		return nil
	}

	// If Docker isn't reachable: don't flag a *permissions* issue here
	info, err := getDockerInfo(sock, 1*time.Second)
	if err != nil {
		if isPermissionError(err) {
			return d.createDockerIssue("file-tailing", info.DockerRootDir)
		}
		return nil
	}

	// Skip file tailing checks for rootless Docker
	if isRootless(info) {
		log.Debug("docker-permissions: Docker is running in rootless mode; skipping file-tailing permission checks")
		return nil
	}

	// Only relevant if Docker uses a file-based driver
	if info.LoggingDriver != "json-file" && info.LoggingDriver != "local" {
		return nil
	}

	// Get a sample of container IDs (limit to 3 for efficiency)
	ids, err := listContainerIDs(sock, 1*time.Second, 3)
	if err != nil {
		if isPermissionError(err) {
			return d.createDockerIssue("file-tailing", info.DockerRootDir)
		}
		return nil
	}
	if len(ids) == 0 {
		// No containers means no log files to check; don't call this a permission issue
		return nil
	}

	// Try to open at least one log using container LogPath
	ok, perm := tryOpenOneLogViaInspect(sock, ids)
	if ok {
		return nil
	}
	if perm {
		return d.createDockerIssue("file-tailing", info.DockerRootDir)
	}
	return nil
}
