// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package collectors

import (
	"context"
	"fmt"
	"path"
	"strings"

	configfilesdiscoveryimpl "github.com/DataDog/datadog-agent/comp/core/configfilesdiscovery/impl"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NginxIntegrationName is the Autodiscovery check name for NGINX.
const NginxIntegrationName = "nginx"

type nginxConfigCollector struct{}

// nginxEnvAllowlist contains documented, non-secret runtime settings from the
// official nginx image and Bitnami NGINX. The namespace is deliberately
// closed: NGINX_ENVSUBST_* controls generic template expansion and *_FILE
// values can indirectly load arbitrary content.
var nginxEnvAllowlist = map[string]struct{}{
	// Official nginx image.
	"NGINX_ENTRYPOINT_QUIET_LOGS":                {},
	"NGINX_ENTRYPOINT_WORKER_PROCESSES_AUTOTUNE": {},

	// Bitnami NGINX image.
	"NGINX_WORKER_PROCESSES":         {},
	"NGINX_HTTP_PORT_NUMBER":         {},
	"NGINX_HTTPS_PORT_NUMBER":        {},
	"NGINX_SKIP_SAMPLE_CERTS":        {},
	"NGINX_ENABLE_STREAM":            {},
	"NGINX_ENABLE_ABSOLUTE_REDIRECT": {},
	"NGINX_ENABLE_PORT_IN_REDIRECT":  {},
}

// NewNginx returns a collector for NGINX environment configuration.
func NewNginx() configfilesdiscoveryimpl.ConfigCollector {
	return nginxConfigCollector{}
}

// CanCollectFromProcess returns whether a process event identifies NGINX and
// can trigger the one-shot recollection fallback for its container.
func (nginxConfigCollector) CanCollectFromProcess(commandline configfilesdiscoveryimpl.TargetCommandline) bool {
	args := unwrapShellCommandline(commandline.Args)
	if len(args) == 0 {
		return false
	}
	if path.Base(args[0]) == "nginx" {
		return true
	}
	// NGINX rewrites its master process title after startup. Match only the
	// master title, not workers, so a container process event schedules one
	// recollection instead of one per worker.
	return strings.HasPrefix(args[0], "nginx: master process ") ||
		len(args) >= 3 && args[0] == "nginx:" && args[1] == "master" && args[2] == "process"
}

// Collect gathers selected NGINX environment metadata. NGINX configuration
// files are intentionally out of scope: the official images allow arbitrary
// template expansion, so a file collector needs its own data-safety design.
func (nginxConfigCollector) Collect(ctx context.Context, reader configfilesdiscoveryimpl.ConfigReader) (configfilesdiscoveryimpl.CollectedConfig, error) {
	envVars, err := readEnvVars(ctx, reader, includeNginxEnvVar)
	if err != nil {
		return configfilesdiscoveryimpl.CollectedConfig{}, fmt.Errorf("read nginx env vars: %w", err)
	}
	if len(envVars) == 0 {
		log.Debugf("config files discovery skipped nginx env var collection: no selected env vars detected")
		return configfilesdiscoveryimpl.CollectedConfig{}, nil
	}
	return configfilesdiscoveryimpl.CollectedConfig{EnvVars: envVars}, nil
}

func includeNginxEnvVar(name string) bool {
	if configfilesdiscoveryimpl.IsSecretEnvVarName(name) {
		return false
	}
	_, allowed := nginxEnvAllowlist[name]
	return allowed
}
