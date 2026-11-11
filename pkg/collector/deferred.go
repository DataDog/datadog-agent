// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package collector provides the implementation of the collector
package collector

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// DepInstaller installs and removes the OCI package carrying the third-party
// python dependencies of integrations.
type DepInstaller interface {
	InstallDepPackage(pkg, version string) error
	RemoveDepPackage(pkg string) error
}

// depsDeclarer is implemented by loaders that know which third-party
// dependencies a check requires (the python loader, backed by the dep
// manifest).
type depsDeclarer interface {
	DeclaredDeps(config integration.Config) []string
}

type deferredConfig struct {
	config     integration.Config
	deps       []string
	attempts   int
	deferredAt time.Time
}

func onDemandDepsEnabled() bool {
	return pkgconfigsetup.Datadog().GetBool("on_demand_python_deps.enabled")
}

func deferredRetryInterval() time.Duration {
	seconds := pkgconfigsetup.Datadog().GetInt("on_demand_python_deps.retry_interval")
	if seconds < 1 {
		seconds = 1
	}
	return time.Duration(seconds) * time.Second
}

func deferredMaxAttempts() int {
	attempts := pkgconfigsetup.Datadog().GetInt("on_demand_python_deps.max_attempts")
	if attempts < 1 {
		attempts = 1
	}
	return attempts
}

// declaredDeps returns the dependencies declared by any loader for this
// config, or nil.
func (s *CheckScheduler) declaredDeps(config integration.Config) []string {
	for _, loader := range s.loaders {
		if declarer, ok := loader.(depsDeclarer); ok {
			if deps := declarer.DeclaredDeps(config); len(deps) > 0 {
				return deps
			}
		}
	}
	return nil
}

func (s *CheckScheduler) startDeferredLoop() {
	s.deferredStop = make(chan struct{})
	s.deferredWg.Add(1)
	go s.deferredLoop()
}

func (s *CheckScheduler) deferredLoop() {
	defer s.deferredWg.Done()
	ticker := time.NewTicker(deferredRetryInterval())
	defer ticker.Stop()
	for {
		select {
		case <-s.deferredStop:
			return
		case <-ticker.C:
			s.retryDeferred()
		}
	}
}

// retryDeferred re-attempts to load every deferred config, runs the checks
// that became loadable, and gives up on configs that exhausted their attempts.
func (s *CheckScheduler) retryDeferred() {
	s.m.RLock()
	configs := make([]integration.Config, 0, len(s.deferredConfigs))
	for _, d := range s.deferredConfigs {
		configs = append(configs, d.config)
	}
	s.m.RUnlock()
	if len(configs) == 0 {
		return
	}

	checks := s.GetChecksFromConfigs(configs, true)
	if coll, ok := s.collector.Get(); ok {
		for _, c := range checks {
			if !IsCheckAllowed(c.String(), pkgconfigsetup.Datadog()) {
				continue
			}
			if _, err := coll.RunCheck(c); err != nil {
				log.Errorf("Unable to run deferred Check %s: %v", c, err)
				errorStats.setRunError(c.ID(), err.Error())
			}
		}
	}

	maxAttempts := deferredMaxAttempts()
	s.m.Lock()
	for digest, d := range s.deferredConfigs {
		d.attempts++
		if d.attempts >= maxAttempts {
			delete(s.deferredConfigs, digest)
			log.Errorf("Check %s still cannot load, dependencies %v missing after %d attempts, giving up", d.config.Name, d.deps, d.attempts)
			errorStats.setLoaderError(d.config.Name, "python", "deferred dependencies never became importable")
		} else {
			s.deferredConfigs[digest] = d
		}
	}
	s.m.Unlock()
}

// triggerDepInstall fires one deduplicated install request for the
// dependency package. The request is fire-and-forget: completion is noticed
// by the retry loop re-probing the deferred configs.
func (s *CheckScheduler) triggerDepInstall(deps []string) {
	if s.depInstaller == nil || len(deps) == 0 {
		return
	}
	s.depInstallMu.Lock()
	triggered := s.depInstallTriggered
	s.depInstallTriggered = true
	s.depInstallMu.Unlock()
	if triggered {
		return
	}
	go func() {
		pkg := pkgconfigsetup.Datadog().GetString("on_demand_python_deps.dep_package_name")
		if err := s.depInstaller.InstallDepPackage(pkg, version.AgentPackageVersion); err != nil {
			log.Errorf("Could not request installation of dependency package %s: %v", pkg, err)
		}
		s.depInstallMu.Lock()
		s.depInstallTriggered = false
		s.depInstallMu.Unlock()
	}()
}

// removeOrphanDeps removes the dependency package when no scheduled or
// deferred config still declares a dependency on it. The removal itself is
// async; callers do not need to hold the scheduler mutex.
func (s *CheckScheduler) removeOrphanDeps(removedDeps []string) {
	if s.depInstaller == nil || len(removedDeps) == 0 {
		return
	}
	for _, deps := range s.configDeps {
		if len(deps) > 0 {
			return
		}
	}
	for _, d := range s.deferredConfigs {
		if len(d.deps) > 0 {
			return
		}
	}
	pkg := pkgconfigsetup.Datadog().GetString("on_demand_python_deps.dep_package_name")
	go func() {
		if err := s.depInstaller.RemoveDepPackage(pkg); err != nil {
			log.Errorf("Could not request removal of dependency package %s: %v", pkg, err)
		}
	}()
}

func (s *CheckScheduler) deferredStatus() []map[string]interface{} {
	s.m.RLock()
	defer s.m.RUnlock()
	status := make([]map[string]interface{}, 0, len(s.deferredConfigs))
	for _, d := range s.deferredConfigs {
		status = append(status, map[string]interface{}{
			"check":    d.config.Name,
			"deps":     d.deps,
			"attempts": d.attempts,
		})
	}
	return status
}
