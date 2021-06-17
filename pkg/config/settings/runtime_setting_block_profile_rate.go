package settings

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/profiling"
)

// RuntimeBlockProfileRate wraps runtime.SetBlockProfileRate setting
type RuntimeBlockProfileRate (string)

// Name returns the name of the runtime setting
func (r RuntimeBlockProfileRate) Name() string {
	return string(r)
}

// Description returns the runtime setting's description
func (r RuntimeBlockProfileRate) Description() string {
	return "This setting controls the fraction of goroutine blocking events that are reported in the internal blocking profile"
}

// Hidden returns whether or not this setting is hidden from the list of runtime settings
func (r RuntimeBlockProfileRate) Hidden() bool {
	// Go runtime will start accumulating profile data as soon as this option is set to a
	// non-zero value. There is a risk that left on over a prolonged period of time, it
	// may negatively impact agent performance.
	return true
}

// Get returns the current value of the runtime setting
func (r RuntimeBlockProfileRate) Get() (interface{}, error) {
	return profiling.GetBlockProfileRate(), nil
}

// Set changes the value of the runtime setting
func (r RuntimeBlockProfileRate) Set(value interface{}) error {
	rate, err := GetInt(value)
	if err != nil {
		return err
	}

	profiling.SetBlockProfileRate(rate)
	config.Datadog.Set("internal_profiling.block_profile_rate", rate)

	return nil
}
