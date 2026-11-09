// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscoveryimpl

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// secretOrigins records the secret origins used by each config. Each section of a config (init_config, each instance,
// metrics and logs) is resolved under its own origin "<digest>;<section>", so the secrets component reports where in
// the config each secret appears. A nil *secretOrigins records nothing.
type secretOrigins struct {
	mu       sync.Mutex
	byDigest map[string]map[string]struct{}
}

func newSecretOrigins() *secretOrigins {
	return &secretOrigins{byDigest: map[string]map[string]struct{}{}}
}

func (s *secretOrigins) add(digest string, origin string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byDigest[digest] == nil {
		s.byDigest[digest] = map[string]struct{}{}
	}
	s.byDigest[digest][origin] = struct{}{}
}

// remove forgets the origins used by the config with the given digest, and returns them
func (s *secretOrigins) remove(digest string) []string {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	origins := slices.Collect(maps.Keys(s.byDigest[digest]))
	delete(s.byDigest, digest)
	return origins
}

// secretOrigin returns the secret origin of a section of the config with the given digest
func secretOrigin(digest string, section string) string {
	return digest + ";" + section
}

// secretOriginDigest returns the digest of the config a secret origin belongs to
func secretOriginDigest(origin string) string {
	digest, _, _ := strings.Cut(origin, ";")
	return digest
}

func decryptConfig(conf integration.Config, secretResolver secrets.Component, digest string, origins *secretOrigins) (integration.Config, error) {
	if pkgconfigsetup.Datadog().GetBool("secret_backend_skip_checks") {
		log.Tracef("'secret_backend_skip_checks' is enabled, not decrypting configuration %q", conf.Name)
		return conf, nil
	}

	resolve := func(data integration.Data, section string) (integration.Data, error) {
		origin := secretOrigin(digest, section)
		origins.add(digest, origin)
		// Pass check name to secret component
		if conf.Name != "" {
			secretResolver.SetOriginName(origin, conf.Name+";"+section)
		}
		return secretResolver.Resolve(data, origin, conf.ImageName, conf.PodNamespace, false)
	}

	var err error

	// init_config is shared by all instances — any failure drops the entire config.
	conf.InitConfig, err = resolve(conf.InitConfig, "init_config")
	if err != nil {
		conf.Instances = nil
		return conf, fmt.Errorf("error while decrypting secrets in 'init_config': %s", err)
	}

	// instances — failing instances are skipped so surviving ones are still scheduled.
	var instanceErr error
	instances := make([]integration.Data, 0, len(conf.Instances))
	for i, inputInstance := range conf.Instances {
		decryptedInstance, err := resolve(inputInstance, "instances."+strconv.Itoa(i))
		if err != nil {
			instanceErr = fmt.Errorf("error while decrypting secrets in an instance: %s", err)
			continue
		}
		instances = append(instances, decryptedInstance)
	}
	conf.Instances = instances

	// metrics
	conf.MetricConfig, err = resolve(conf.MetricConfig, "metrics")
	if err != nil {
		conf.Instances = nil
		return conf, fmt.Errorf("error while decrypting secrets in 'metrics': %s", err)
	}

	// logs
	conf.LogsConfig, err = resolve(conf.LogsConfig, "logs")
	if err != nil {
		conf.Instances = nil
		return conf, fmt.Errorf("error while decrypting secrets in 'logs': %s", err)
	}

	return conf, instanceErr
}
