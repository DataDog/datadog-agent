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

// nginxDefaultConfigPaths contains the main configuration file path shipped by
// the official nginx image and by Bitnami NGINX. The two distributions are not
// expected to coexist in a single container, so both live in one group.
var nginxDefaultConfigPaths = []string{
	"/etc/nginx/nginx.conf",
	"/opt/bitnami/nginx/conf/nginx.conf",
}

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

// Collect gathers the main NGINX configuration file and selected environment
// metadata.
//
// Config content is collected as-is: the official and Bitnami images support
// substituting environment variables into templated config files via
// envsubst, and this collector does not attempt to detect or redact that.
// This is the same known limitation the env var allowlist above already
// guards against for NGINX_ENVSUBST_* and *_FILE values individually; a value
// substituted into the config file itself is subject to whatever redaction
// the platform already applies to config file payloads generally, not to any
// nginx-specific logic here.
func (nginxConfigCollector) Collect(ctx context.Context, reader configfilesdiscoveryimpl.ConfigReader) (configfilesdiscoveryimpl.CollectedConfig, error) {
	envVars, envErr := readEnvVars(ctx, reader, includeNginxEnvVar)
	if envErr != nil {
		log.Debugf("config files discovery skipped nginx env var collection: %v", envErr)
		envVars = nil
	}

	selection, err := selectConfigFile(
		ctx,
		reader,
		nginxGetConfigArgFromCommandline,
		nginxMatchesCommandline,
		"",
		nginxDefaultConfigPaths,
	)
	if err != nil {
		return configfilesdiscoveryimpl.CollectedConfig{}, fmt.Errorf("collect nginx config file: %w", err)
	}
	if selection == nil {
		// Without a config file, env vars are the only NGINX config source.
		// Return the error so the scheduler retries.
		if envErr != nil {
			return configfilesdiscoveryimpl.CollectedConfig{}, fmt.Errorf("read nginx env vars: %w", envErr)
		}
		if len(envVars) == 0 {
			log.Debugf("config files discovery skipped nginx config collection: no config file or selected env vars detected")
			return configfilesdiscoveryimpl.CollectedConfig{}, nil
		}

		log.Debugf("config files discovery collected nginx env vars without a config file")
		return configfilesdiscoveryimpl.CollectedConfig{EnvVars: envVars}, nil
	}

	// agent-payload has no NGINX config format; leave PayloadFormat unset
	// (PAYLOAD_FORMAT_UNKNOWN), matching how other formatless config files are
	// reported.
	return configfilesdiscoveryimpl.CollectedConfig{
		ConfigFiles: []configfilesdiscoveryimpl.ConfigFile{selection.file},
		EnvVars:     envVars,
	}, nil
}

func includeNginxEnvVar(name string) bool {
	if configfilesdiscoveryimpl.IsSecretEnvVarName(name) {
		return false
	}
	_, allowed := nginxEnvAllowlist[name]
	return allowed
}

// nginxGetConfigArgFromCommandline returns the explicit config file argument
// passed to nginx via -c.
func nginxGetConfigArgFromCommandline(args []string) (string, bool) {
	nginxArgs, ok := nginxGetArgs(args)
	if !ok {
		return "", false
	}
	return nginxGetConfigArg(nginxArgs)
}

// nginxMatchesCommandline always returns false. redis and kafka treat "the
// process matched but named no explicit config argument" as proof no file is
// read, which correctly blocks default-path guessing for them. NGINX is
// different: it always loads its compiled-in default path
// (/etc/nginx/nginx.conf) when started without -c, so recognizing the
// invocation itself must not block nginxDefaultConfigPaths below.
func nginxMatchesCommandline(_ []string) bool {
	return false
}

// nginxGetArgs returns the arguments following the nginx invocation itself,
// handling a normal argv, the official image's docker-entrypoint.sh wrapper,
// and NGINX's rewritten master process title (which repeats the original
// invocation after a fixed prefix).
func nginxGetArgs(args []string) ([]string, bool) {
	args = unwrapShellCommandline(args)
	if len(args) == 0 {
		return nil, false
	}
	switch {
	case path.Base(args[0]) == "nginx":
		return args[1:], true
	case len(args) > 1 && path.Base(args[0]) == "docker-entrypoint.sh" && path.Base(args[1]) == "nginx":
		// The official image's entrypoint always execs the given command
		// (default or overridden) after templating/setup, so Docker's
		// reported Path/Args still show the entrypoint script wrapping it.
		return args[2:], true
	case strings.HasPrefix(args[0], "nginx: master process "):
		fields := strings.Fields(strings.TrimPrefix(args[0], "nginx: master process "))
		if len(fields) == 0 || path.Base(fields[0]) != "nginx" {
			return nil, false
		}
		return fields[1:], true
	case len(args) >= 4 && args[0] == "nginx:" && args[1] == "master" && args[2] == "process" && path.Base(args[3]) == "nginx":
		return args[4:], true
	default:
		return nil, false
	}
}

// nginxGetConfigArg returns the -c config file argument. nginx also accepts
// -g, -e, and -p with their own values, but only -c identifies a config file
// this collector can read.
func nginxGetConfigArg(nginxArgs []string) (string, bool) {
	for i, arg := range nginxArgs {
		if arg == "-c" && i+1 < len(nginxArgs) {
			return nginxArgs[i+1], true
		}
	}
	return "", false
}
