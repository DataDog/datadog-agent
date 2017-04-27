package common

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/providers"
	"github.com/DataDog/datadog-agent/pkg/collector/py"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/ecs"
	"github.com/DataDog/datadog-agent/pkg/util/gce"
	log "github.com/cihub/seelog"
)

// GetConfigProviders builds a list of providers for checks' configurations, the sequence defines
// the precedence.
func GetConfigProviders(confdPath string) (plist []providers.ConfigProvider) {
	confSearchPaths := []string{
		confdPath,
		filepath.Join(GetDistPath(), "conf.d"),
	}

	// File Provider
	plist = append(plist, providers.NewFileConfigProvider(confSearchPaths))

	return plist
}

// GetCheckLoaders builds a list of check loaders, the sequence defines the precedence.
func GetCheckLoaders() []check.Loader {
	return []check.Loader{
		py.NewPythonCheckLoader(),
		core.NewGoCheckLoader(),
	}
}

// SetupConfig fires up the configuration system
func SetupConfig() {

	// set the paths where a config file is expected
	config.Datadog.AddConfigPath(defaultConfPath)
	config.Datadog.AddConfigPath(GetDistPath())

	// load the configuration
	err := config.Datadog.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("unable to load Datadog config file: %s", err))
	}

	// define defaults for the Agent
	config.Datadog.SetDefault("cmd_sock", "/tmp/agent.sock")
	config.Datadog.BindEnv("cmd_sock")
	config.Datadog.SetDefault("check_runners", int64(4))
}

// GetHostname retrieve the host name for the Agent, trying to query these
// environments/api, in order:
// * GCE
// * Docker
// * kubernetes
// * os
// * EC2
func GetHostname() string {
	var hostName string

	// try the name provided in the configuration file
	name := config.Datadog.GetString("hostname")
	err := util.ValidHostname(name)
	if err == nil {
		return name
	}

	log.Warnf("unable to get the hostname from the config file: %s", err)
	log.Warn("trying to determine a reliable host name automatically...")

	// GCE metadata
	log.Debug("GetHostname trying GCE metadata...")
	name, err = gce.GetHostname()
	if err == nil {
		return name
	}

	if isContainerized() {
		// Docker
		log.Debug("GetHostname trying Docker API...")
		name, err = docker.GetHostname()
		if err == nil && util.ValidHostname(name) == nil {
			hostName = name
		} else if isKubernetes() {
			log.Debug("GetHostname trying k8s...")
			// TODO
		}
	}

	if hostName == "" {
		// os
		log.Debug("GetHostname trying os...")
		name, err = os.Hostname()
		if err == nil {
			hostName = name
		}
	}

	/* at this point we've either the hostname from the os or an empty string */

	// We use the instance id if we're on an ECS cluster or we're on EC2
	// and the hostname is one of the default ones
	if ecs.IsInstance() || ec2.IsDefaultHostname(hostName) {
		log.Debug("GetHostname trying EC2 metadata...")
		instanceID, err := ec2.GetInstanceID()
		if err == nil {
			hostName = instanceID
		}
	}

	// If at this point we don't have a name, bail out
	if hostName == "" {
		panic(fmt.Errorf("Unable to reliably determine the host name. You can define one in the agent config file or in your hosts file"))
	}

	return hostName
}

// IsContainerized returns whether the Agent is running on a Docker container
func isContainerized() bool {
	return os.Getenv("DOCKER_DD_AGENT") == "yes"
}

// IsKubernetes returns whether the Agent is running on a kubernetes cluster
func isKubernetes() bool {
	return os.Getenv("KUBERNETES_PORT") != ""
}
