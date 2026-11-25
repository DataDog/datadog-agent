// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package remoteflags provides a Remote Flags system built on top of Remote Config
// that allows features to be dynamically enabled or disabled via remote configuration.
package remoteflags

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ConfigSetter is the minimal subset of pkg/config the Remote Flags client needs
// to apply flags that target a configuration field. It is provided by the caller
// to keep pkg/remoteflags a leaf package.
type ConfigSetter interface {
	Set(key string, value any, source model.Source)
	UnsetForSource(key string, source model.Source)
	GetSource(key string) model.Source
}

// DefaultHealthCheckInterval is the default interval between health checks after a flag is enabled.
const DefaultHealthCheckInterval = 10 * time.Second

const (
	// DefaultHealthCheckDuration is the default duration for health monitoring after flag activation
	DefaultHealthCheckDuration = 1 * time.Minute
	// DefaultFailuresBeforeRecover is the default number of consecutive health check failures before SafeRecover is called
	DefaultFailuresBeforeRecover = 1
)

// FlagName represents a remote flag identifier.
type FlagName string

// FlagValue represents the boolean value of a remote flag.
type FlagValue bool

// FlagHandler handles a single remote flag subscription.
// Components must implement this interface for each flag they want to subscribe to,
// providing compile-time enforcement that all required methods are implemented.
//
// The lifecycle is:
//   - OnChange is called when the flag value changes (or immediately if a value is already known at subscription time)
//   - If OnChange returns an error, SafeRecover is called to force the feature into a safe state
//   - After a flag is enabled (value=true), IsHealthy is called periodically to monitor the component
//   - If IsHealthy returns false for too many consecutive checks, SafeRecover is called
//   - OnNoConfig is called when Remote Config sends configurations but the flag is absent
//
// Delivery semantics:
//   - OnChange, OnNoConfig and SafeRecover calls for one handler are serialized:
//     they never run concurrently and are delivered in order, on a goroutine
//     owned by the client and outside the client lock. A handler may therefore
//     call Client.GetCurrentValue.
//   - OnNoConfig is asynchronous: it may run after the Remote Config update
//     that triggered it has returned.
//   - Delivery is latest-wins. Intermediate values may be coalesced: a change
//     equal to the last applied value is skipped, and a removal immediately
//     followed by the same value again may not be delivered.
//   - A handler that blocks delays only its own flag, never other flags.
//   - Client.Stop does not interrupt a handler call already in flight.
//   - After SafeRecover, the same value is not redelivered by itself; a genuine
//     new change or a removal followed by re-adding the flag is needed.
//   - IsHealthy is called from the health monitor goroutine, independently of
//     the serialized callbacks above.
type FlagHandler interface {
	// FlagName returns the name of the flag this handler listens to.
	FlagName() FlagName

	// OnChange is called when the flag value changes.
	// It is called:
	//   - Immediately after subscription if the flag value is already known
	//   - Whenever the flag value changes in Remote Config
	//
	// MUST return nil if the configuration change was successfully applied,
	// or an error if it failed. When an error is returned, SafeRecover will be called.
	OnChange(value FlagValue) error

	// OnNoConfig is called when the remote config client received configurations,
	// but the flag was not part of the flags list. This allows the handler to
	// handle the case where the flag is expected but not present.
	OnNoConfig()

	// SafeRecover is called when:
	//   - OnChange returns an error (configuration failed to apply properly)
	//   - IsHealthy returns false for the configured number of consecutive checks
	//
	// The failedValue parameter indicates which value failed to apply.
	// This method is responsible for forcing the feature into a safe, working state.
	// It should use independent logic to ensure safety, not retry the same operation.
	// It MUST be idempotent.
	SafeRecover(err error, failedValue FlagValue)

	// IsHealthy is called periodically after a flag is enabled (value=true) to verify
	// the component remains healthy. If this returns false for the configured number
	// of consecutive checks, SafeRecover will be called.
	IsHealthy() bool
}

// RemoteFlagSubscriber is implemented by components that want to subscribe to remote flags.
// This allows a single component to subscribe to multiple flags.
type RemoteFlagSubscriber interface {
	// Handlers returns the list of flag handlers for this component.
	Handlers() []FlagHandler
}

// Flag represents a single flag with its name and value.
type Flag struct {
	Name    string    `json:"name"`
	Enabled FlagValue `json:"enabled"`
	// ConfigurationField is the optional config key (e.g. "feature.x.enabled")
	// the flag is bound to. When set and a ConfigSetter is configured, the
	// client mirrors the flag value into pkg/config under SourceRC.
	ConfigurationField string `json:"configuration_field,omitempty"`
	// OverrideLocal controls whether the flag is allowed to overwrite a value
	// already set by a user-provided source (file, env, fleet policies, CLI...).
	// Defaults to false: local user configuration wins.
	OverrideLocal bool `json:"override_local,omitempty"`
	// Version is the sequence number of this flag value. The client only applies
	// a flag whose Version is strictly greater than the last applied Version for
	// that flag. A Version of 0 (omitted) is treated as unversioned and applied
	// unconditionally.
	Version                          int `json:"version,omitempty"`
	HealthCheckDurationSeconds       int `json:"health_check_duration_seconds,omitempty"`
	HealthCheckFailuresBeforeRecover int `json:"health_check_failures_before_recover,omitempty"`
}

// HealthCheckDuration returns the duration for health monitoring.
// Returns DefaultHealthCheckDuration if not specified in the config.
func (f Flag) HealthCheckDuration() time.Duration {
	if f.HealthCheckDurationSeconds == 0 {
		return DefaultHealthCheckDuration
	}
	return time.Duration(f.HealthCheckDurationSeconds) * time.Second
}

// FailuresBeforeRecover returns the number of consecutive health check failures
// required before SafeRecover is called.
// Returns DefaultFailuresBeforeRecover if not specified in the config.
func (f Flag) FailuresBeforeRecover() int {
	if f.HealthCheckFailuresBeforeRecover == 0 {
		return DefaultFailuresBeforeRecover
	}
	return f.HealthCheckFailuresBeforeRecover
}

// FlagConfig represents the JSON structure of a remote flag configuration.
// It contains an array of flags, each with a name and value.
type FlagConfig struct {
	Flags []Flag `json:"flags"`
}

// eventKind identifies the kind of callback an event requests.
type eventKind int

const (
	eventChange eventKind = iota
	eventNoConfig
	eventRecover
)

// event is a unit of work delivered to a single subscription's handler.
type event struct {
	kind eventKind
	flag Flag
	// gen is the monitor generation a recover event was raised for.
	gen uint64
	// cfgGen is the configuration field generation observed when the event was created.
	cfgGen uint64
	err    error
}

// subscription represents an active subscription to a remote flag.
type subscription struct {
	handler   FlagHandler
	lastValue atomic.Pointer[FlagValue] // nil means no value has been applied yet

	// qmu guards the delivery slots below. Lock order is Client.mu then qmu.
	qmu            sync.Mutex
	pending        *event // latest change or noconfig event, latest wins
	recoverPending *event // latest recover event, kept apart so it is never overwritten
	running        bool   // true while a worker goroutine owns delivery

	// Guarded by Client.mu.
	cancelHealthCheck context.CancelFunc // Cancel ongoing health check if any
	monitorGen        uint64             // bumped whenever the current monitor is invalidated
	appliedCfgGen     uint64             // configuration field generation of the applied value; a recover rolls back only if it is still current
}

// Client is the Remote Flags client that manages flag subscriptions
// and integrates with the Remote Config system.
//
// Handler callbacks are delivered per subscription: they are serialized (never
// concurrent for one subscription), run off the client lock (a handler may call
// back into the client), and are asynchronous with respect to OnUpdate and
// SubscribeWithHandler, which only enqueue. Delivery is latest-wins: intermediate
// states may be coalesced. A change equal to the last applied value is skipped,
// and a removal immediately followed by the same value again may not be delivered
// at all. A hung handler delays only the flag it subscribes to. Stop prevents
// further deliveries but does not interrupt a handler call already in flight.
//
// After SafeRecover runs, the last applied value is cleared but the flag's current
// value is kept, so the same value is not redelivered by itself. Redelivery needs
// a genuine new change, or a removal followed by the flag coming back; each then
// costs one update plus one health failure threshold (a bounded flap).
type Client struct {
	mu                  sync.Mutex
	subscriptions       map[FlagName][]*subscription
	currentValues       map[FlagName]FlagValue
	currentFlags        map[FlagName]Flag
	configGens          map[string]uint64
	lastVersions        map[FlagName]int
	configSetter        ConfigSetter
	healthCheckInterval time.Duration
	ctx                 context.Context
	cancel              context.CancelFunc
}

// NewClient creates a new Remote Flags client.
func NewClient() *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		subscriptions:       make(map[FlagName][]*subscription),
		currentValues:       make(map[FlagName]FlagValue),
		currentFlags:        make(map[FlagName]Flag),
		configGens:          make(map[string]uint64),
		lastVersions:        make(map[FlagName]int),
		healthCheckInterval: DefaultHealthCheckInterval,
		ctx:                 ctx,
		cancel:              cancel,
	}
}

// WithHealthCheckInterval overrides the interval between health checks. Must be
// called before any subscription starts a health monitor; intended for tests.
func (c *Client) WithHealthCheckInterval(d time.Duration) *Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.healthCheckInterval = d
	return c
}

// WithConfigSetter wires a ConfigSetter into the client. When set, flags carrying
// a ConfigurationField mirror their value into pkg/config under SourceRC, subject
// to the OverrideLocal precedence rule.
func (c *Client) WithConfigSetter(setter ConfigSetter) *Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.configSetter = setter
	return c
}

// shouldApplyConfig reports whether a remote flag is allowed to overwrite the
// value at a configuration field. With OverrideLocal=false the flag must not
// clobber a value already provided by another source: user-facing ones (file,
// env, fleet policies, CLI) carry user intent, and agent-runtime values often
// encode deliberate context-specific decisions (e.g. a subcommand disabling a
// feature it cannot run with). Only Default and our own SourceRC writes are
// freely overridable.
func shouldApplyConfig(currentSource model.Source, overrideLocal bool) bool {
	if overrideLocal {
		return true
	}
	switch currentSource {
	case model.SourceDefault, model.SourceUnknown, model.SourceRC, "":
		return true
	default:
		return false
	}
}

// Stop cancels all health monitors and cleans up resources. Queued events are
// dropped and no further callbacks start; a handler call already in flight is
// not interrupted. This should be called when the client is no longer needed.
func (c *Client) Stop() {
	c.cancel()
}

// SubscribeWithHandler registers a subscription using the FlagHandler interface.
// This provides compile-time enforcement that all required functions are implemented.
func (c *Client) SubscribeWithHandler(handler FlagHandler) error {
	if handler == nil {
		return errors.New("handler cannot be nil")
	}
	flag := FlagName(strings.ToLower(string(handler.FlagName())))
	if flag == "" {
		return errors.New("flag name cannot be empty")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	sub := &subscription{
		handler: handler,
	}

	c.subscriptions[flag] = append(c.subscriptions[flag], sub)

	// If we already have cached a value for this flag, enqueue a change event
	// to the new subscription only (existing subscribers already got it).
	if enabled, exists := c.currentValues[flag]; exists {
		f, ok := c.currentFlags[flag]
		if !ok {
			f = Flag{Name: string(flag), Enabled: enabled}
		}
		f.Enabled = enabled
		c.enqueue(sub, event{kind: eventChange, flag: f, cfgGen: c.configGens[f.ConfigurationField]})
	}

	return nil
}

// OnUpdate implements the Remote Config listener interface.
// This is called by the Remote Config client when flag configurations are updated.
func (c *Client) OnUpdate(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Track which flags were seen in the configuration
	processedFlags := make(map[FlagName]struct{})

	// Process each config
	for configPath, rawConfig := range updates {
		// Parse the configuration
		var flagConfig FlagConfig
		if err := json.Unmarshal(rawConfig.Config, &flagConfig); err != nil {
			log.Warnf("Failed to parse Remote Flag config %s: %v", configPath, err)
			applyStateCallback(configPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: fmt.Sprintf("JSON parsing error: %v", err),
			})
			continue
		}

		// Process each flag in the array
		for _, flag := range flagConfig.Flags {
			flag.Name = strings.ToLower(flag.Name)
			flagName := FlagName(flag.Name)
			processedFlags[flagName] = struct{}{}

			// Enforce version sequencing: drop strictly older or equal flag versions.
			// version 0 (omitted) bypasses the check.
			if flag.Version > 0 {
				if lastVersion, seen := c.lastVersions[flagName]; seen && flag.Version <= lastVersion {
					log.Debugf("Remote flag %s: ignoring version %d (last applied %d)", flag.Name, flag.Version, lastVersion)
					continue
				}
				c.lastVersions[flagName] = flag.Version
			}

			oldValue, existed := c.currentValues[flagName]
			changed := !existed || oldValue != flag.Enabled

			// Mirror to pkg/config when a configuration_field is provided. If a
			// non-overridable source already populates that field and the flag
			// did not opt in via override_local, drop the flag entirely.
			if flag.ConfigurationField != "" {
				if c.configSetter == nil {
					log.Debugf("Remote flag %s: configuration_field %q declared but no ConfigSetter is wired; the field will not be mirrored into pkg/config", flag.Name, flag.ConfigurationField)
				} else {
					currentSource := c.configSetter.GetSource(flag.ConfigurationField)
					if !shouldApplyConfig(currentSource, flag.OverrideLocal) {
						log.Infof("Remote flag %s: not applying to %q (source %q has precedence; set override_local=true to force)", flag.Name, flag.ConfigurationField, currentSource)
						continue
					}
					c.configSetter.Set(flag.ConfigurationField, bool(flag.Enabled), model.SourceRC)
					// Only a value change makes older events stale: a repeat of the
					// same value queues nothing, so the running monitor must still
					// be able to roll the field back.
					if changed {
						c.configGens[flag.ConfigurationField]++
					}
				}
			}

			if changed {
				c.currentValues[flagName] = flag.Enabled
				c.currentFlags[flagName] = flag
				c.notifyChange(flag)
			} else {
				// Same value: keep the metadata a late subscriber receives current.
				c.currentFlags[flagName] = flag
			}
		}

		// Report successful application
		applyStateCallback(configPath, state.ApplyStatus{
			State: state.ApplyStateAcknowledged,
		})
	}

	// Check for removed flags (flags we have subscriptions for but weren't in this update)
	// It is an inconsistent behaviour: it means we previously received a value for this flag,
	// but that it's not part the configurations we receive from Remote Config anymore. It's better
	// to still deal with it.
	for flagName := range c.subscriptions {
		if _, exists := processedFlags[flagName]; !exists {
			// flag was not part of the configurations anymore
			// Remove from currentValues so that when it comes back, it will trigger a notification
			// even if the value is the same as before
			delete(c.currentValues, flagName)
			delete(c.currentFlags, flagName)
			// TODO(remy): should we call the no data callback forever if the flags never come back
			// during this lifecycle of the agent?
			c.notifyNoConfig(flagName)
		}
	}
}

// notifyChange enqueues a change event for every subscriber of the flag.
// Must be called with c.mu held. Deduplication happens at dequeue time.
func (c *Client) notifyChange(flag Flag) {
	cfgGen := c.configGens[flag.ConfigurationField]
	for _, sub := range c.subscriptions[FlagName(flag.Name)] {
		c.enqueue(sub, event{kind: eventChange, flag: flag, cfgGen: cfgGen})
	}
}

// notifyNoConfig enqueues a noconfig event for every subscriber of the flag,
// meaning we properly established a communication with Remote Config, but no
// configuration was present for this flag.
// Must be called with c.mu held.
// TODO(remy): should this one provide the last value received if any?
func (c *Client) notifyNoConfig(flag FlagName) {
	for _, sub := range c.subscriptions[flag] {
		c.enqueue(sub, event{kind: eventNoConfig, flag: Flag{Name: string(flag)}})
	}
}

// enqueue places ev in the subscription's slot (latest wins, recover has its own
// slot) and starts a worker if none is running. It never blocks and never takes
// c.mu; it may be called with c.mu held (lock order is c.mu then qmu).
func (c *Client) enqueue(sub *subscription, ev event) {
	if c.ctx.Err() != nil {
		return
	}
	sub.qmu.Lock()
	defer sub.qmu.Unlock()
	if ev.kind == eventRecover {
		sub.recoverPending = &ev
	} else {
		sub.pending = &ev
	}
	if !sub.running {
		sub.running = true
		go c.runWorker(sub)
	}
}

// runWorker delivers queued events for one subscription, recover first, until
// both slots are empty. Handlers run outside every lock.
func (c *Client) runWorker(sub *subscription) {
	clean := false
	defer func() {
		if !clean {
			// A handler panicked: release ownership so delivery can resume.
			sub.qmu.Lock()
			sub.running = false
			sub.qmu.Unlock()
		}
	}()
	for {
		sub.qmu.Lock()
		if c.ctx.Err() != nil {
			sub.pending, sub.recoverPending = nil, nil
			sub.running = false
			sub.qmu.Unlock()
			clean = true
			return
		}
		ev := sub.recoverPending
		if ev != nil {
			sub.recoverPending = nil
		} else {
			ev = sub.pending
			sub.pending = nil
		}
		if ev == nil {
			sub.running = false
			sub.qmu.Unlock()
			clean = true
			return
		}
		sub.qmu.Unlock()
		c.handleEvent(sub, *ev)
	}
}

// handleEvent applies one event. Only the subscription's worker calls it.
func (c *Client) handleEvent(sub *subscription, ev event) {
	switch ev.kind {
	case eventChange:
		// Dedup before touching the monitor: a skipped change leaves it alone.
		if last := sub.lastValue.Load(); last != nil && *last == ev.flag.Enabled {
			// The applied value is unchanged but the field may have been Set again
			// since: let the active monitor roll back the latest write.
			c.refreshAppliedCfgGen(sub, ev.cfgGen)
			return
		}
		gen := c.invalidateMonitor(sub)
		c.refreshAppliedCfgGen(sub, ev.cfgGen)
		if err := sub.handler.OnChange(ev.flag.Enabled); err != nil {
			sub.lastValue.Store(nil)
			applyErr := fmt.Errorf("remote flag %s (enabled=%v): %w", ev.flag.Name, ev.flag.Enabled, err)
			c.recoverFlag(sub, ev, applyErr)
			return
		}
		successValue := ev.flag.Enabled
		sub.lastValue.Store(&successValue)
		c.startHealthMonitor(sub, ev, gen)
	case eventNoConfig:
		c.invalidateMonitor(sub)
		sub.lastValue.Store(nil)
		sub.handler.OnNoConfig()
	case eventRecover:
		c.mu.Lock()
		stale := sub.monitorGen != ev.gen
		ev.cfgGen = sub.appliedCfgGen
		c.mu.Unlock()
		if stale {
			return
		}
		c.recoverFlag(sub, ev, ev.err)
		sub.lastValue.Store(nil)
		c.startRecoveryMonitor(sub, ev, ev.gen)
	}
}

// refreshAppliedCfgGen records cfgGen as the configuration field generation the
// applied value corresponds to. It never moves backwards.
func (c *Client) refreshAppliedCfgGen(sub *subscription, cfgGen uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cfgGen > sub.appliedCfgGen {
		sub.appliedCfgGen = cfgGen
	}
}

// enqueueRecover queues a recover event raised by the monitor of generation gen.
// An event from a monitor that has been invalidated since is dropped, so it can
// never replace the pending recover of a newer monitor.
func (c *Client) enqueueRecover(sub *subscription, ev event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if sub.monitorGen != ev.gen {
		return
	}
	c.enqueue(sub, ev)
}

// invalidateMonitor cancels the current monitor, if any, and returns the new
// monitor generation.
func (c *Client) invalidateMonitor(sub *subscription) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	sub.monitorGen++
	if sub.cancelHealthCheck != nil {
		sub.cancelHealthCheck()
		sub.cancelHealthCheck = nil
	}
	return sub.monitorGen
}

// installMonitor records cancel as the subscription's monitor if gen is still
// current; otherwise it cancels immediately and reports false.
func (c *Client) installMonitor(sub *subscription, gen uint64, cancel context.CancelFunc) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if sub.monitorGen != gen {
		cancel()
		return false
	}
	if sub.cancelHealthCheck != nil {
		sub.cancelHealthCheck()
	}
	sub.cancelHealthCheck = cancel
	return true
}

// recoverFlag rolls back a flag's effect on pkg/config (when applicable) and
// then invokes the handler's SafeRecover. The Unset happens first so the handler
// observes the reverted configuration when it runs. The Unset is skipped when the
// field was Set again since the event was created, so a newer value is not wiped;
// SafeRecover always runs.
func (c *Client) recoverFlag(sub *subscription, ev event, err error) {
	field := ev.flag.ConfigurationField
	if field != "" {
		c.mu.Lock()
		if c.configSetter != nil && c.configGens[field] == ev.cfgGen {
			c.configSetter.UnsetForSource(field, model.SourceRC)
		}
		c.mu.Unlock()
	}
	sub.handler.SafeRecover(err, ev.flag.Enabled)
}

// startHealthMonitor starts a goroutine that periodically checks the health of a subscription
// after a flag is enabled (value=true). If the component becomes unhealthy for the configured
// number of consecutive checks, a recover event is enqueued; the worker then calls SafeRecover
// and starts a recovery probe to validate that the recovery was successful.
func (c *Client) startHealthMonitor(sub *subscription, ev event, gen uint64) {
	flag := ev.flag
	// Only monitor when enabling (value=true)
	if !flag.Enabled {
		return
	}

	// Create context with timeout, derived from client's parent context
	ctx, cancel := context.WithTimeout(c.ctx, flag.HealthCheckDuration())
	if !c.installMonitor(sub, gen, cancel) {
		return
	}

	go func() {
		ticker := time.NewTicker(c.healthCheckInterval)
		defer ticker.Stop()

		consecutiveFailures := 0
		failuresBeforeRecover := flag.FailuresBeforeRecover()

		for {
			select {
			case <-ctx.Done():
				return // Monitoring period ended or client stopped
			case <-ticker.C:
				if !sub.handler.IsHealthy() {
					consecutiveFailures++
					log.Debugf("Remote flag %s: component unhealthy (failure %d/%d)", sub.handler.FlagName(), consecutiveFailures, failuresBeforeRecover)
					if consecutiveFailures >= failuresBeforeRecover {
						err := fmt.Errorf("remote flag %s: unhealthy for %d checks", sub.handler.FlagName(), consecutiveFailures)
						rev := ev
						rev.kind = eventRecover
						rev.gen = gen
						rev.err = err
						c.enqueueRecover(sub, rev)
						cancel()
						return
					}
				} else {
					if consecutiveFailures > 0 {
						log.Debugf("Remote flag %s: component healthy again after %d failures", sub.handler.FlagName(), consecutiveFailures)
					}
					consecutiveFailures = 0 // Reset on healthy check
				}
			}
		}
	}()
}

// startRecoveryMonitor starts a goroutine that probes the health of a subscription after
// SafeRecover has been called, to validate that the recovery was successful.
// It uses the same probe timeout as the initial health monitor.
// If the component is still unhealthy after the recovery probe window, we log and give up.
func (c *Client) startRecoveryMonitor(sub *subscription, ev event, gen uint64) {
	ctx, cancel := context.WithTimeout(c.ctx, ev.flag.HealthCheckDuration())
	if !c.installMonitor(sub, gen, cancel) {
		return
	}

	go func() {
		ticker := time.NewTicker(c.healthCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Debugf("Remote flag %s: recovery probe ended without confirming healthy state", sub.handler.FlagName())
				return
			case <-ticker.C:
				if sub.handler.IsHealthy() {
					log.Debugf("Remote flag %s: component healthy after recovery", sub.handler.FlagName())
					cancel()
					return
				}
				log.Debugf("Remote flag %s: component still unhealthy after recovery", sub.handler.FlagName())
			}
		}
	}()
}

// GetCurrentValue returns the current value of a flag.
// Returns the value and true if the flag is known, or FlagValue(false) and false if unknown.
func (c *Client) GetCurrentValue(flag FlagName) (FlagValue, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	value, exists := c.currentValues[FlagName(strings.ToLower(string(flag)))]
	return value, exists
}

// TODO(remy): func (c *Client) GetCurrentValueBlocking(flag FlagName) (FlagValue, bool)
