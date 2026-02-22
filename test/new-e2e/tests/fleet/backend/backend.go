// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package backend contains a fake fleet backend for use in tests.
package backend

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/avast/retry-go/v4"
	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/nacl/box"
	"golang.org/x/mod/semver"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
)

// RemoteConfigState is the state of the remote config.
type RemoteConfigState struct {
	Packages      []RemoteConfigStatePackage `json:"remote_config_state"`
	SecretsPubKey string                     `json:"secrets_pub_key,omitempty"`
}

// RemoteConfigStatePackage is the state of a package in the remote config.
type RemoteConfigStatePackage struct {
	Package                 string `json:"package"`
	StableVersion           string `json:"stable_version"`
	ExperimentVersion       string `json:"experiment_version"`
	StableConfigVersion     string `json:"stable_config_version"`
	ExperimentConfigVersion string `json:"experiment_config_version"`
}

// Backend is the fake fleet backend.
type Backend struct {
	t    func() *testing.T
	host *environments.Host
}

// New creates a new Backend.
func New(t func() *testing.T, host *environments.Host) *Backend {
	return &Backend{t: t, host: host}
}

// FileOperationType is the type of operation to perform on the config.
type FileOperationType string

const (
	// FileOperationPatch patches the config at the given path with the given JSON patch (RFC 6902).
	FileOperationPatch FileOperationType = "patch"
	// FileOperationMergePatch merges the config at the given path with the given JSON merge patch (RFC 7396).
	FileOperationMergePatch FileOperationType = "merge-patch"
	// FileOperationDelete deletes the config at the given path.
	FileOperationDelete FileOperationType = "delete"
)

// ConfigOperations is the list of operations to perform on the config.
type ConfigOperations struct {
	DeploymentID   string          `json:"deployment_id"`
	FileOperations []FileOperation `json:"file_operations"`
}

// FileOperation is the operation to perform on a config.
type FileOperation struct {
	FileOperationType FileOperationType `json:"file_op"`
	FilePath          string            `json:"file_path"`
	Patch             json.RawMessage   `json:"patch,omitempty"`
}

// getSecretsPubKey gets the public key for the secrets.
func (b *Backend) getSecretsPubKey() (*[32]byte, error) {
	// Get public signing key
	rcStatus, err := b.RemoteConfigStatus()
	if err != nil {
		return nil, fmt.Errorf("error getting remote config status: %w", err)
	}
	publicKey, err := base64.StdEncoding.DecodeString(rcStatus.SecretsPubKey)
	if err != nil {
		return nil, fmt.Errorf("error decoding secrets public key: %w", err)
	}
	if len(publicKey) != 32 {
		return nil, fmt.Errorf("unexpected public key length: got %d, want 32", len(publicKey))
	}
	var pk [32]byte
	copy(pk[:], publicKey)
	return &pk, nil
}

// StartConfigExperiment starts a config experiment for the given package.
func (b *Backend) StartConfigExperiment(operations ConfigOperations, secrets map[string]string) error {
	b.t().Logf("Starting config experiment")
	rawOperations, err := json.Marshal(operations)
	if err != nil {
		return err
	}

	flags := []string{"datadog-agent", string(rawOperations)}
	if len(secrets) > 0 {
		publicKey, err := b.getSecretsPubKey()
		require.NoError(b.t(), err)
		// For each secret, encrypt it with the public key and add the flag to the list
		for key, secret := range secrets {
			encryptedSecret, err := box.SealAnonymous(nil, []byte(secret), publicKey, rand.Reader)
			if err != nil {
				return fmt.Errorf("error encrypting secret: %w", err)
			}
			flags = append(flags, fmt.Sprintf("--secret=%s=%s", key, base64.StdEncoding.EncodeToString(encryptedSecret)))
		}
	}

	output, err := b.runDaemonCommandWithRestart("start-config-experiment", flags...)
	if err != nil {
		return fmt.Errorf("%w, output: %s", err, output)
	}
	b.t().Logf("Config experiment started")
	return nil
}

// PromoteConfigExperiment promotes a config experiment for the given package.
func (b *Backend) PromoteConfigExperiment() error {
	b.t().Logf("Promoting config experiment")
	output, err := b.runDaemonCommandWithRestart("promote-config-experiment", "datadog-agent")
	if err != nil {
		return fmt.Errorf("%w, output: %s", err, output)
	}
	b.t().Logf("Config experiment promoted")
	return nil
}

// StopConfigExperiment stops a config experiment for the given package.
func (b *Backend) StopConfigExperiment() error {
	b.t().Logf("Stopping config experiment")
	output, err := b.runDaemonCommandWithRestart("stop-config-experiment", "datadog-agent")
	if err != nil {
		return fmt.Errorf("%w, output: %s", err, output)
	}
	b.t().Logf("Config experiment stopped")
	return nil
}

// StartExperiment starts an update experiment for the given package.
func (b *Backend) StartExperiment(pkg string, version string) error {
	b.t().Logf("Starting update experiment for package %s version %s", pkg, version)
	err := b.setCatalog()
	if err != nil {
		return fmt.Errorf("error setting catalog: %w", err)
	}
	output, err := b.runDaemonCommandWithRestart("start-experiment", pkg, version)
	if err != nil {
		return fmt.Errorf("%w, output: %s", err, output)
	}
	b.t().Logf("Experiment started")
	return nil
}

// PromoteExperiment promotes an update experiment for the given package.
func (b *Backend) PromoteExperiment(pkg string) error {
	b.t().Logf("Promoting update experiment for package %s", pkg)
	// On Windows the daemon does not restart after promote: the experiment binary
	// is already running and simply becomes stable in-place. Only wait for a
	// PID change on Linux where the stable service is restarted.
	var (
		output string
		err    error
	)
	if b.host.RemoteHost.OSFamily == e2eos.WindowsFamily {
		output, err = b.runDaemonCommand("promote-experiment", pkg)
	} else {
		output, err = b.runDaemonCommandWithRestart("promote-experiment", pkg)
	}
	if err != nil {
		return fmt.Errorf("%w, output: %s", err, output)
	}
	b.t().Logf("Experiment promoted")
	return nil
}

// StopExperiment stops an update experiment for the given package.
func (b *Backend) StopExperiment(pkg string) error {
	b.t().Logf("Stopping update experiment for package %s", pkg)
	output, err := b.runDaemonCommandWithRestart("stop-experiment", pkg)
	if err != nil {
		return fmt.Errorf("%w, output: %s", err, output)
	}
	b.t().Logf("Experiment stopped")
	return nil
}

// RemoteConfigStatusPackage returns the status of the remote config for a given package.
func (b *Backend) RemoteConfigStatusPackage(packageName string) (RemoteConfigStatePackage, error) {
	status, err := b.RemoteConfigStatus()
	if err != nil {
		return RemoteConfigStatePackage{}, err
	}
	for _, pkg := range status.Packages {
		if pkg.Package == packageName {
			return pkg, nil
		}
	}
	return RemoteConfigStatePackage{}, fmt.Errorf("package %s not found", packageName)
}

// RemoteConfigStatus returns the status of the remote config.
func (b *Backend) RemoteConfigStatus() (RemoteConfigState, error) {
	status, err := b.runDaemonCommand("rc-status")
	if err != nil {
		return RemoteConfigState{}, err
	}
	var remoteConfigState RemoteConfigState
	err = json.Unmarshal([]byte(status), &remoteConfigState)
	if err != nil {
		return RemoteConfigState{}, err
	}
	return remoteConfigState, nil
}

// Branch is the branch of the package.
type Branch string

const (
	// BranchStable is the stable branch of the package.
	BranchStable Branch = "stable"
	// BranchTesting is the testing branch of the package.
	BranchTesting Branch = "testing"
)

type catalogEntry struct {
	Package string `json:"package"`
	Version string `json:"version"`
	URL     string `json:"url"`

	branch Branch
}

// Catalog is the catalog of available packages.
type Catalog struct {
	packages []catalogEntry
}

// Latest returns the latest version for a given package and branch.
func (c *Catalog) Latest(branch Branch, pkg string) string {
	return c.LatestMinus(branch, pkg, 0)
}

// LatestMinus returns the version that is N versions behind the latest, for a given package and branch.
func (c *Catalog) LatestMinus(branch Branch, pkg string, minus int) string {
	var currentMinor string
	for _, entry := range c.packages {
		if entry.Package != pkg || entry.branch != branch {
			continue
		}
		if currentMinor == "" {
			currentMinor = semver.MajorMinor(entry.Version)
		}
		if semver.MajorMinor(entry.Version) != currentMinor {
			minus--
		}
		if minus == 0 {
			return entry.Version
		}
	}
	panic(fmt.Errorf("package %s %s %d not found", pkg, branch, minus))
}

func (b *Backend) setCatalog() error {
	b.t().Logf("Setting catalog")
	catalog := b.Catalog()
	serializedCatalog, err := json.Marshal(struct {
		Packages []catalogEntry `json:"packages"`
	}{
		Packages: catalog.packages,
	})
	if err != nil {
		return err
	}
	output, err := b.runDaemonCommand("set-catalog", string(serializedCatalog))
	if err != nil {
		return fmt.Errorf("%w, output: %s", err, output)
	}
	return nil
}

var cachedCatalog *Catalog
var cachedCatalogOnce sync.Once

// Catalog returns the catalog.
func (b *Backend) Catalog() *Catalog {
	cachedCatalogOnce.Do(func() {
		catalog, err := b.getCatalog()
		if err != nil {
			b.t().Fatalf("error getting catalog: %v", err)
			return
		}
		cachedCatalog = catalog
	})
	if cachedCatalog == nil {
		b.t().Fatalf("catalog is nil")
	}
	return cachedCatalog
}

func (b *Backend) getCatalog() (*Catalog, error) {
	var catalog Catalog

	urls := []string{"installtesting.datad0g.com/agent-package:pipeline-" + os.Getenv("E2E_PIPELINE_ID")}
	var prodTags []string
	err := retry.Do(func() error {
		var err error
		prodTags, err = getImagesTags("install.datadoghq.com/agent-package")
		return err
	}, retry.Attempts(10), retry.Delay(1*time.Second), retry.DelayType(retry.FixedDelay))
	if err != nil {
		return nil, err
	}
	for _, tag := range prodTags {
		urls = append(urls, "install.datadoghq.com/agent-package:"+tag)
	}
	for _, url := range urls {
		var version string
		err := retry.Do(func() error {
			var err error
			version, err = getImageVersion(url)
			return err
		}, retry.Attempts(10), retry.Delay(1*time.Second), retry.DelayType(retry.FixedDelay))
		if err != nil {
			return nil, err
		}
		var branch Branch
		switch {
		case strings.HasPrefix(url, "installtesting.datad0g.com"):
			url = strings.Replace(url, "installtesting.datad0g.com", "installtesting.datad0g.com.internal.dda-testing.com", 1)
			branch = BranchTesting
		case strings.HasPrefix(url, "install.datadoghq.com"):
			url = strings.Replace(url, "install.datadoghq.com", "install.datadoghq.com.internal.dda-testing.com", 1)
			branch = BranchStable
		default:
			return nil, fmt.Errorf("unsupported URL: %s", url)
		}
		catalog.packages = append(catalog.packages, catalogEntry{
			Package: "datadog-agent",
			Version: version,
			URL:     "oci://" + url,
			branch:  branch,
		})
	}
	sort.Slice(catalog.packages, func(i, j int) bool {
		return semver.Compare("v"+catalog.packages[i].Version, "v"+catalog.packages[j].Version) > 0
	})
	cachedCatalog = &catalog
	return cachedCatalog, nil
}

func (b *Backend) runDaemonCommandWithRestart(command string, args ...string) (string, error) {
	var originalPID int
	err := retry.Do(func() error {
		var err error
		originalPID, err = b.getDaemonPID()
		return err
	}, retry.Attempts(10), retry.Delay(1*time.Second), retry.DelayType(retry.FixedDelay))
	if err != nil {
		return "", err
	}
	output, err := b.runDaemonCommand(command, args...)
	if err != nil {
		return "", err
	}
	err = retry.Do(func() error {
		newPID, err := b.getDaemonPID()
		if err != nil {
			return err
		}
		if newPID == originalPID {
			return fmt.Errorf("daemon PID %d is still running", newPID)
		}
		return nil
	}, retry.Attempts(30), retry.Delay(15*time.Second), retry.DelayType(retry.FixedDelay))
	if err != nil {
		return "", fmt.Errorf("error waiting for daemon to restart: %w", err)
	}
	return output, nil
}

func (b *Backend) runDaemonCommand(command string, args ...string) (string, error) {
	var baseCommand string
	var sanitizeCharacter string
	switch b.host.RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		sanitizeCharacter = `\"`
		baseCommand = "sudo datadog-installer daemon"
		_, err := b.host.RemoteHost.Execute(baseCommand + " --help")
		if err != nil {
			if !strings.Contains(err.Error(), "unknown command") {
				return "", err
			}
			baseCommand = "sudo DD_BUNDLED_AGENT=installer datadog-agent daemon"
		}
	case e2eos.WindowsFamily:
		sanitizeCharacter = "\\`\""
		baseCommand = `& "C:\Program Files\Datadog\Datadog Agent\bin\datadog-installer.exe" daemon`
	default:
		return "", fmt.Errorf("unsupported OS family: %v", b.host.RemoteHost.OSFamily)
	}

	err := retry.Do(func() error {
		_, err := b.host.RemoteHost.Execute(baseCommand + " rc-status")
		return err
	})
	if err != nil {
		return "", fmt.Errorf("error waiting for daemon to be ready: %w", err)
	}

	var sanitizedArgs []string
	for _, arg := range args {
		arg = `"` + strings.ReplaceAll(arg, `"`, sanitizeCharacter) + `"`
		sanitizedArgs = append(sanitizedArgs, arg)
	}
	return b.host.RemoteHost.Execute(fmt.Sprintf("%s %s %s", baseCommand, command, strings.Join(sanitizedArgs, " ")))
}

func (b *Backend) getDaemonPID() (int, error) {
	var pid string
	var err error
	switch b.host.RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		// bugfix for https://major.io/p/systemd-in-fedora-22-failed-to-restart-service-access-denied/
		if b.host.RemoteHost.OSFlavor == e2eos.CentOS && b.host.RemoteHost.OSVersion == e2eos.CentOS7.Version {
			_, err := b.host.RemoteHost.Execute("sudo systemctl daemon-reexec")
			if err != nil {
				return 0, fmt.Errorf("error reexecuting systemd: %w", err)
			}
		}
		pid, err = b.host.RemoteHost.Execute(`sudo systemctl show -p MainPID datadog-agent-installer.service | cut -d= -f2`)
		pidExp, errExp := b.host.RemoteHost.Execute(`sudo systemctl show -p MainPID datadog-agent-installer-exp.service | cut -d= -f2`)
		pid = strings.TrimSpace(pid)
		pidExp = strings.TrimSpace(pidExp)
		if err != nil || errExp != nil {
			return 0, fmt.Errorf("error getting daemon PID: %w, %w", err, errExp)
		}
		if pidExp != "0" {
			pid = pidExp
		}
	case e2eos.WindowsFamily:
		pid, err = b.host.RemoteHost.Execute(`(Get-CimInstance Win32_Service -Filter "Name='Datadog Installer'").ProcessId`)
		pid = strings.TrimSpace(pid)
	default:
		return 0, fmt.Errorf("unsupported OS family: %v", b.host.RemoteHost.OSFamily)
	}
	if err != nil {
		return 0, err
	}
	if pid == "0" {
		return 0, errors.New("daemon PID is 0")
	}
	return strconv.Atoi(pid)
}

func getImageVersion(ref string) (string, error) {
	p := v1.Platform{
		OS:           "linux",
		Architecture: "amd64",
	}
	raw, err := crane.Manifest(ref, crane.WithPlatform(&p))
	if err != nil {
		return "", err
	}
	var m v1.Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", err
	}
	version, ok := m.Annotations["com.datadoghq.package.version"]
	if !ok {
		return "unknown", nil
	}
	return version, nil
}

func getImagesTags(src string) ([]string, error) {
	tags, err := crane.ListTags(src)
	if err != nil {
		return nil, err
	}
	return tags, nil
}
