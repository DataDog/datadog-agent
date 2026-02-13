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

// Common Remote Flags - Add your flags here as constants
const (
	// FlagEnableDemoAnalytics enables the demo analytics telemetry feature
	// This is used for demonstration purposes only
	FlagEnableDemoAnalytics FlagName = "enable_demo_analytics"
)

// FlagHandler handles a single flag subscription.
// Components have to implement this interface for each flag they want to subscribe to.
// Creates a compile-time enforcement that the necessary functions are implemented.
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

	// SafeRecover is called when an error occurs while applying a flag change.
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

// FlagChangeCallback is called when a flag's value changes.
// The value parameter contains the new flag value.
// The callback MUST return nil if the configuration change was successfully
// applied, or an error if it failed.
// When an error is returned, the remoteflags package will call the safeRecover
// callback to ensure the feature ends up in a correct state.
type FlagChangeCallback func(value FlagValue) error

// FlagSafeRecoverCallback is called when an error is reported applying a flag change.
// It can also be called by the Remote Flags system if the component reports unhealthy
// for a too long.
//
// This callback is responsible for forcing the feature into a safe, working state.
// It should use independent logic to ensure safety, not retry the same operation that failed.
type FlagSafeRecoverCallback func(err error, failedValue FlagValue)

// FlagNoConfigCallback is called when the remote config client received some configurations,
// but the flag was not part of the flags list.
type FlagNoConfigCallback func()

// FlagIsHealthyCallback is called periodically after a flag is enabled to verify
// the component remains healthy. Returns true if the component is healthy, false otherwise.
type FlagIsHealthyCallback func() bool

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
	flag              FlagName
	onChange          FlagChangeCallback
	onNoConfig        FlagNoConfigCallback
	safeRecover       FlagSafeRecoverCallback
	isHealthy         FlagIsHealthyCallback
	lastValue         *FlagValue         // Track last known value to detect changes
	cancelHealthCheck context.CancelFunc // Cancel ongoing health check if any
}

// Client is the Remote Flags client that manages flag subscriptions
// and integrates with the Remote Config system.
type Client struct {
	mu            sync.RWMutex
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
// This is the recommended way to subscribe to flags from components, as it provides
// compile-time enforcement that all required functions are implemented.
func (c *Client) SubscribeWithHandler(handler FlagHandler) error {
	if handler == nil {
		return errors.New("handler cannot be nil")
	}
	return c.Subscribe(
		handler.FlagName(),
		handler.OnChange,
		handler.OnNoConfig,
		handler.SafeRecover,
		handler.IsHealthy,
	)
}

// Subscribe registers a subscription to a remote flag using callbacks.
// For component-based subscriptions, prefer SubscribeWithHandler instead.
//
// The onChange callback is called:
//   - Immediately after subscription if the flag value is already known
//   - Whenever the flag value changes in Remote Config
//   - MUST return nil if the change was successfully applied, or an error if it failed
//
// The safeRecover callback is called when:
//   - onChange returns an error (configuration failed to apply properly)
//   - isHealthy returns false for the configured number of consecutive checks
//     This callback must force the feature into a safe, working state using independent logic
//   - It MUST be idempotent.
//
// The onNoConfig callback is called when:
//   - Remote Config sent configurations, but the flag matching a subscription
//     was not part of the returned configurations
//
// The isHealthy callback is called periodically after a flag is enabled (value=true)
// to verify the component remains healthy. If it returns false for the configured
// number of consecutive checks, safeRecover will be called.
//
// Returns an error if the subscription parameters are invalid.
func (c *Client) Subscribe(flag FlagName, onChange FlagChangeCallback, onNoConfig FlagNoConfigCallback, safeRecover FlagSafeRecoverCallback, isHealthy FlagIsHealthyCallback) error {
	if flag == "" {
		return errors.New("flag name cannot be empty")
	}
	if onChange == nil {
		return errors.New("onChange callback cannot be nil")
	}
	if onNoConfig == nil {
		return errors.New("onNoConfig callback cannot be nil")
	}
	if safeRecover == nil {
		return errors.New("safeRecover callback cannot be nil - you must provide error handling")
	}
	if isHealthy == nil {
		return errors.New("isHealthy callback cannot be nil - you must provide health checking")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	sub := &subscription{
		flag:        flag,
		onChange:    onChange,
		safeRecover: safeRecover,
		onNoConfig:  onNoConfig,
		isHealthy:   isHealthy,
	}

	c.subscriptions[flag] = append(c.subscriptions[flag], sub)

	// If we already have cached a value for this flag,
	// invoke the callback immediately
	if value, exists := c.currentValues[flag]; exists {
		// Create a Flag with default health check settings for initial subscription
		initialFlag := Flag{
			Name:  string(flag),
			Value: value,
		}
		go func(s *subscription, f Flag) {
			if err := s.onChange(f.Value); err != nil {
				applyErr := fmt.Errorf("failed to apply initial configuration for flag %s with value %v: %w", f.Name, f.Value, err)
				s.safeRecover(applyErr, f.Value)
			} else {
				// Only update lastValue if onChange succeeded
				successValue := f.Value
				c.mu.Lock()
				s.lastValue = &successValue
				c.mu.Unlock()
				// Start health monitoring (only runs if value=true)
				c.startHealthMonitor(s, f)
			}
		}(sub, initialFlag)
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
			// TODO(remy): can this goroutine block?
			go func(s *subscription, f Flag) {
				// Try to apply the configuration change
				if err := s.onChange(f.Value); err != nil {
					// If the change failed to apply, call safeRecover callback
					applyErr := fmt.Errorf("failed to apply configuration change for flag %s with value %v: %w", f.Name, f.Value, err)
					s.safeRecover(applyErr, f.Value)
				} else {
					// Only update lastValue if onChange succeeded
					// Create a new variable to ensure proper heap allocation
					successValue := f.Value
					c.mu.Lock()
					s.lastValue = &successValue
					c.mu.Unlock()
					// Start health monitoring (only runs if value=true)
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
		sub.onNoConfig()
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
				if !sub.isHealthy() {
					consecutiveFailures++
					log.Warnf("Remote flag %s: component unhealthy (failure %d/%d)", sub.flag, consecutiveFailures, failuresBeforeRecover)
					if consecutiveFailures >= failuresBeforeRecover {
						err := fmt.Errorf("component unhealthy for %d consecutive checks after flag %s was enabled", consecutiveFailures, sub.flag)
						sub.safeRecover(err, flag.Value)
						cancel()
						return
					}
				} else {
					if consecutiveFailures > 0 {
						log.Infof("Remote flag %s: component healthy again after %d failures", sub.flag, consecutiveFailures)
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
	c.mu.RLock()
	defer c.mu.RUnlock()

	value, exists := c.currentValues[flag]
	return value, exists
}

// TODO(remy): func (c *Client) GetCurrentValueBlocking(flag FlagName) (FlagValue, bool)
