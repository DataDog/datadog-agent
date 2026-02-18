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
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Health check constants
const (
	// HealthCheckInterval is the interval between health checks after a flag is enabled
	HealthCheckInterval = 10 * time.Second
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
	Name                             string    `json:"name"`
	Value                            FlagValue `json:"value"`
	HealthCheckDurationSeconds       int       `json:"health_check_duration_seconds,omitempty"`
	HealthCheckFailuresBeforeRecover int       `json:"health_check_failures_before_recover,omitempty"`
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

// subscription represents an active subscription to a remote flag.
type subscription struct {
	handler           FlagHandler
	lastValue         *FlagValue         // Track last known value to detect changes
	cancelHealthCheck context.CancelFunc // Cancel ongoing health check if any
}

// Client is the Remote Flags client that manages flag subscriptions
// and integrates with the Remote Config system.
type Client struct {
	mu            sync.Mutex
	subscriptions map[FlagName][]*subscription
	currentValues map[FlagName]FlagValue
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewClient creates a new Remote Flags client.
func NewClient() *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		subscriptions: make(map[FlagName][]*subscription),
		currentValues: make(map[FlagName]FlagValue),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Stop cancels all health monitors and cleans up resources.
// This should be called when the client is no longer needed.
func (c *Client) Stop() {
	c.cancel()
}

// SubscribeWithHandler registers a subscription using the FlagHandler interface.
// This provides compile-time enforcement that all required functions are implemented.
func (c *Client) SubscribeWithHandler(handler FlagHandler) error {
	if handler == nil {
		return errors.New("handler cannot be nil")
	}
	flag := handler.FlagName()
	if flag == "" {
		return errors.New("flag name cannot be empty")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	sub := &subscription{
		handler: handler,
	}

	c.subscriptions[flag] = append(c.subscriptions[flag], sub)

	// If we already have cached a value for this flag,
	// invoke the callback immediately via notifyChange
	if value, exists := c.currentValues[flag]; exists {
		c.notifyChange(Flag{Name: string(flag), Value: value})
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
			flagName := FlagName(flag.Name)
			processedFlags[flagName] = struct{}{}

			// Check if the value changed
			oldValue, existed := c.currentValues[flagName]
			if !existed || oldValue != flag.Value {
				c.currentValues[flagName] = flag.Value
				c.notifyChange(flag)
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
			// TODO(remy): should we call the no data callback forever if the flags never come back
			// during this lifecycle of the agent?
			c.notifyNoConfig(flagName)

			// go through all subs of this flag name and remove their last value.
			for _, sub := range c.subscriptions[flagName] {
				sub.lastValue = nil
			}
		}
	}
}

// notifyChange notifies all subscribers of a flag value change.
// Must be called with lock held.
func (c *Client) notifyChange(flag Flag) {
	flagName := FlagName(flag.Name)
	subs, exists := c.subscriptions[flagName]
	if !exists {
		return
	}

	for _, sub := range subs {
		// only notify if the value actually changed from last known value
		if sub.lastValue == nil || *sub.lastValue != flag.Value {
			go func(s *subscription, f Flag) {
				if err := s.handler.OnChange(f.Value); err != nil {
					applyErr := fmt.Errorf("remote flag %s (value=%v): %w", f.Name, f.Value, err)
					s.handler.SafeRecover(applyErr, f.Value)
				} else {
					successValue := f.Value
					c.mu.Lock()
					s.lastValue = &successValue
					c.mu.Unlock()
					c.startHealthMonitor(s, f)
				}
			}(sub, flag)
		}
	}
}

// notifyNoConfig notifies all subscribers that we properly established
// a communication with Remote Config, but no configuration was present for this flag.
// Must be called with lock held.
// TODO(remy): should this one provide the last value received if any?
func (c *Client) notifyNoConfig(flag FlagName) {
	subs, exists := c.subscriptions[flag]
	if !exists {
		return
	}

	for _, sub := range subs {
		// Cancel ongoing health check if any
		if sub.cancelHealthCheck != nil {
			sub.cancelHealthCheck()
			sub.cancelHealthCheck = nil
		}
		sub.handler.OnNoConfig()
	}
}

// startHealthMonitor starts a goroutine that periodically checks the health of a subscription
// after a flag is enabled (value=true). If the component becomes unhealthy for the configured
// number of consecutive checks, SafeRecover is called.
func (c *Client) startHealthMonitor(sub *subscription, flag Flag) {
	// Only monitor when enabling (value=true)
	if !flag.Value {
		return
	}

	// Create context with timeout, derived from client's parent context
	ctx, cancel := context.WithTimeout(c.ctx, flag.HealthCheckDuration())

	// Store cancel func to stop previous monitor if new change comes in
	c.mu.Lock()
	if sub.cancelHealthCheck != nil {
		sub.cancelHealthCheck() // Cancel previous health check
	}
	sub.cancelHealthCheck = cancel
	c.mu.Unlock()

	go func() {
		ticker := time.NewTicker(HealthCheckInterval)
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
					log.Warnf("Remote flag %s: component unhealthy (failure %d/%d)", sub.handler.FlagName(), consecutiveFailures, failuresBeforeRecover)
					if consecutiveFailures >= failuresBeforeRecover {
						err := fmt.Errorf("remote flag %s: unhealthy for %d checks", sub.handler.FlagName(), consecutiveFailures)
						sub.handler.SafeRecover(err, flag.Value)
						cancel()
						return
					}
				} else {
					if consecutiveFailures > 0 {
						log.Infof("Remote flag %s: component healthy again after %d failures", sub.handler.FlagName(), consecutiveFailures)
					}
					consecutiveFailures = 0 // Reset on healthy check
				}
			}
		}
	}()
}

// GetCurrentValue returns the current value of a flag.
// Returns the value and true if the flag is known, or FlagValue(false) and false if unknown.
func (c *Client) GetCurrentValue(flag FlagName) (FlagValue, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	value, exists := c.currentValues[flag]
	return value, exists
}

// TODO(remy): func (c *Client) GetCurrentValueBlocking(flag FlagName) (FlagValue, bool)
