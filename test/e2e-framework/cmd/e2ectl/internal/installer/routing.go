// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/receiver"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/receivers"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/standalone"
	"go.yaml.in/yaml/v3"
)

// RoutingConsumer is explicit opt-in: custom installers must not ignore a stock
// selector. RoutingApplier is separate from install/update (no build implied).
type RoutingConsumer interface {
	PrepareRouting(*config.File, envstore.Entry) (*receivers.Plan, error)
}
type RoutingApplier interface {
	ApplyRouting(*config.File, envstore.Entry) error
}

func (*Binary) PrepareRouting(c *config.File, e envstore.Entry) (*receivers.Plan, error) {
	return PrepareRouting(c, e, true)
}
func (*Kubernetes) PrepareRouting(c *config.File, e envstore.Entry) (*receivers.Plan, error) {
	return PrepareRouting(c, e, true)
}
func (*HostScript) PrepareRouting(c *config.File, e envstore.Entry) (*receivers.Plan, error) {
	return PrepareRouting(c, e, false)
}

func ValidateReceiver(inst Installer, cfg *config.File) error {
	if cfg.Agent.Receiver == nil {
		return nil
	}
	if _, ok := inst.(RoutingConsumer); !ok {
		return fmt.Errorf("installer %s does not consume agent.receiver", inst.ID())
	}
	return receiver.Validate(cfg.Agent.Receiver)
}

// PrepareRouting projects persisted facts without initializing the old Agent.
// Infrastructure intent is invariant across installation and routing changes.
func PrepareRouting(cfg *config.File, entry envstore.Entry, separateNetwork bool) (*receivers.Plan, error) {
	stored, err := entry.LoadConfig()
	if err != nil {
		return nil, err
	}
	var oldEnv, newEnv any
	if yaml.Unmarshal(stored.Environment.Section, &oldEnv) != nil || yaml.Unmarshal(cfg.Environment.Section, &newEnv) != nil {
		return nil, fmt.Errorf("invalid stored environment config")
	}
	// Raw stored sections may omit defaults; compare source-level infrastructure
	// only when both have materialized sections. Fixture/base always compare.
	if cfg.Environment.Base != stored.Environment.Base || cfg.FakeIntakeEnabled() != stored.FakeIntakeEnabled() {
		return nil, fmt.Errorf("installation cannot change provisioned environment or fixture intent")
	}
	if !reflect.DeepEqual(oldEnv, newEnv) && cfg.Environment.SectionNode != nil && stored.Environment.SectionNode != nil {
		return nil, fmt.Errorf("installation cannot change infrastructure settings")
	}
	facts := receiver.Facts{SeparateNetwork: separateNetwork}
	if stored.FakeIntakeEnabled() {
		var fi outputs.FakeintakeOutput
		if err := provisioner.ReadSnapshotResource(entry.SnapshotPath(), "fakeIntake", &fi); err != nil {
			return nil, fmt.Errorf("expected fakeintake fixture missing: %w", err)
		}
		facts.FakeIntake = &fi
	}
	p, err := receiver.Resolve(cfg.Agent.Receiver, facts)
	if err != nil {
		return nil, err
	}
	if p == nil {
		if separateNetwork && facts.FakeIntake == nil {
			return nil, fmt.Errorf("no-fixture installation requires an explicit receiver")
		}
		fmt.Fprintln(os.Stderr, "warning: legacy receiver routing has partial coverage and may use native credentials/forwarders")
	}
	// Do not bypass an unsupported RC cache transition through full install.
	_, metadata, err := provisioner.ReadSnapshotFile(entry.SnapshotPath())
	if err != nil {
		return nil, err
	}
	var previous RoutingState
	if raw := metadata["_agent_routing"]; raw != nil {
		if err := json.Unmarshal(raw, &previous); err != nil {
			return nil, err
		}
	}
	if entry.Meta.AgentInstalled || previous.Phase != "" {
		oldPolicy := "unknown"
		if previous.Plan != nil {
			oldPolicy = previous.Plan.RemoteConfig
		}
		newPolicy := "unknown"
		if p != nil {
			newPolicy = p.RemoteConfig
		}
		if oldPolicy == "native" && p != nil && previous.Plan != nil && (p.Site != previous.Plan.Site || p.APIKeyRef != previous.Plan.APIKeyRef) {
			return nil, fmt.Errorf("native RC site/credential transition is not implemented; recreate the Agent environment")
		}
		if oldPolicy != newPolicy {
			return nil, fmt.Errorf("RC policy/cache transition %s -> %s is not implemented; recreate the Agent environment", oldPolicy, newPolicy)
		}
	}
	return p, nil
}

func bindAPIKey(p *receivers.Plan) (string, error) {
	if p != nil && p.APIKeyRef == "" {
		return receivers.DummyAPIKey, nil
	}
	key, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	if err != nil {
		return "", fmt.Errorf("resolving selected Agent API credential failed")
	}
	if key == "" {
		return "", fmt.Errorf("selected Agent API credential is empty")
	}
	return key, nil
}

// RoutingState is independent of infrastructure readiness and artifact outputs.
// No raw errors are persisted: transports may include credential-bearing output.
type RoutingState struct {
	Generation string          `json:"generation"`
	Phase      string          `json:"phase"`
	Plan       *receivers.Plan `json:"plan,omitempty"`
	Readiness  string          `json:"readiness"`
	Delivery   string          `json:"delivery"`
}

func RoutingStatus(entry envstore.Entry) (RoutingState, error) {
	_, meta, err := provisioner.ReadSnapshotFile(entry.SnapshotPath())
	if err != nil {
		return RoutingState{}, err
	}
	var state RoutingState
	if raw := meta["_agent_routing"]; raw != nil {
		err = json.Unmarshal(raw, &state)
	}
	return state, err
}

// WithRoutingState publishes in-progress before mutation, then applied/failed.
// A failed operation never leaves an old applied record as apparent authority.
func WithRoutingState(inst Installer, cfg *config.File, entry envstore.Entry, operation func() error) error {
	unlock, err := LockAgentOperation(entry)
	if err != nil {
		return err
	}
	defer unlock()
	consumer, ok := inst.(RoutingConsumer)
	if !ok {
		if cfg.Agent.Receiver != nil {
			return fmt.Errorf("installer does not consume receiver")
		}
		return operation()
	}
	state := RoutingState{Generation: time.Now().UTC().Format(time.RFC3339Nano), Phase: "applying", Readiness: "unknown", Delivery: "unverified"}
	// Resolution is still pure inventory work. Installers preflight their raw input
	// before this wrapper is invoked and resolve credentials before activation.
	p, err := consumer.PrepareRouting(cfg, entry)
	if err != nil {
		return err
	}
	state.Plan = p
	if err := provisioner.UpdateSnapshotResources(entry.SnapshotPath(), nil, map[string]any{"_agent_routing": state}); err != nil {
		return err
	}
	err = operation()
	if err != nil {
		state.Phase = "failed"
	} else {
		state.Phase = "applied"
		state.Readiness = "ready"
	}
	saveErr := provisioner.UpdateSnapshotResources(entry.SnapshotPath(), nil, map[string]any{"_agent_routing": state})
	if err != nil {
		return err
	}
	return saveErr
}

// attachHostForInstall deliberately excludes Agent: an unhealthy old client
// cannot prevent repair. It retains the fixture and its binding in the snapshot.
func attachHostForInstall(entry envstore.Entry) (*environments.Host, error) {
	var out outputs.HostOutput
	if err := provisioner.ReadSnapshotResource(entry.SnapshotPath(), "remoteHost", &out); err != nil {
		return nil, err
	}
	host := &components.RemoteHost{HostOutput: out}
	if err := host.Init(standalone.NewContext(entry.Dir)); err != nil {
		return nil, err
	}
	env := &environments.Host{RemoteHost: host}
	var fi outputs.FakeintakeOutput
	if err := provisioner.ReadSnapshotResource(entry.SnapshotPath(), "fakeIntake", &fi); err == nil {
		env.FakeIntake = &components.FakeIntake{FakeintakeOutput: fi}
	}
	return env, nil
}
