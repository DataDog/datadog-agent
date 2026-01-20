// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package remoteflags provides a Remote Flags system built on top of Remote Config
// that allows features to be dynamically enabled or disabled via remote configuration.
package remoteflags

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

// Flag represents a single flag with its name and value.
type Flag struct {
	Name  string    `json:"name"`
	Value FlagValue `json:"value"`
}

// FlagConfig represents the JSON structure of a remote flag configuration.
// It contains an array of flags, each with a name and value.
type FlagConfig struct {
	Flags []Flag `json:"flags"`
}

// subscription represents an active subscription to a remote flag.
type subscription struct {
	flag        FlagName
	onChange    FlagChangeCallback
	onNoConfig  FlagNoConfigCallback
	safeRecover FlagSafeRecoverCallback
	lastValue   *FlagValue // Track last known value to detect changes
}

// Client is the Remote Flags client that manages flag subscriptions
// and integrates with the Remote Config system.
type Client struct {
	mu            sync.RWMutex
	subscriptions map[FlagName][]*subscription
	currentValues map[FlagName]FlagValue
}

// NewClient creates a new Remote Flags client.
func NewClient() *Client {
	return &Client{
		subscriptions: make(map[FlagName][]*subscription),
		currentValues: make(map[FlagName]FlagValue),
	}
}

// SubscribeWithHandler registers a subscription using the FlagHandler interface.
// This is the recommended way to subscribe to flags from components, as it provides
// compile-time enforcement that all required functions are implemented.
func (c *Client) SubscribeWithHandler(handler FlagHandler) error {
	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}
	return c.Subscribe(
		handler.FlagName(),
		handler.OnChange,
		handler.OnNoConfig,
		handler.SafeRecover,
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
//     This callback must force the feature into a safe, working state using independent logic
//   - It MUST be idempotent.
//
// The onNoConfig callback is called when:
//   - Remote Config sent configurations, but the flag matching a subscription
//     was not part of the returned configurations
//
// Returns an error if the subscription parameters are invalid.
func (c *Client) Subscribe(flag FlagName, onChange FlagChangeCallback, onNoConfig FlagNoConfigCallback, safeRecover FlagSafeRecoverCallback) error {
	if flag == "" {
		return fmt.Errorf("flag name cannot be empty")
	}
	if onChange == nil {
		return fmt.Errorf("onChange callback cannot be nil")
	}
	if onNoConfig == nil {
		return fmt.Errorf("onNoConfig callback cannot be nil")
	}
	if safeRecover == nil {
		return fmt.Errorf("safeRecover callback cannot be nil - you must provide error handling")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	sub := &subscription{
		flag:        flag,
		onChange:    onChange,
		safeRecover: safeRecover,
		onNoConfig:  onNoConfig,
	}

	c.subscriptions[flag] = append(c.subscriptions[flag], sub)

	// If we already have cached a value for this flag,
	// invoke the callback immediately
	if value, exists := c.currentValues[flag]; exists {
		go func(s *subscription, val FlagValue) {
			if err := s.onChange(val); err != nil {
				applyErr := fmt.Errorf("failed to apply initial configuration for flag %s with value %v: %w", flag, val, err)
				s.safeRecover(applyErr, val)
			} else {
				// Only update lastValue if onChange succeeded
				successValue := val
				c.mu.Lock()
				s.lastValue = &successValue
				c.mu.Unlock()
			}
		}(sub, value)
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
				c.notifyChange(flagName, flag.Value)
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
func (c *Client) notifyChange(flag FlagName, newValue FlagValue) {
	subs, exists := c.subscriptions[flag]
	if !exists {
		return
	}

	for _, sub := range subs {
		// only notify if the value actually changed from last known value
		if sub.lastValue == nil || *sub.lastValue != newValue {
			// TODO(remy): can this goroutine block?
			go func(s *subscription, value FlagValue) {
				// Try to apply the configuration change
				if err := s.onChange(value); err != nil {
					// If the change failed to apply, call safeRecover callback
					applyErr := fmt.Errorf("failed to apply configuration change for flag %s with value %v: %w", flag, value, err)
					s.safeRecover(applyErr, value)
				} else {
					// Only update lastValue if onChange succeeded
					// Create a new variable to ensure proper heap allocation
					successValue := value
					c.mu.Lock()
					s.lastValue = &successValue
					c.mu.Unlock()
				}
			}(sub, newValue)
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
		sub.onNoConfig()
	}
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
