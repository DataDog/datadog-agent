package settings

import (
	"runtime"
)

// RuntimeMutexProfileFraction wraps runtime.SetMutexProfileFraction setting.
type RuntimeMutexProfileFraction (string)

// Name returns the name of the runtime setting
func (r RuntimeMutexProfileFraction) Name() string {
	return string(r)
}

// Description returns the runtime setting's description
func (r RuntimeMutexProfileFraction) Description() string {
	return "This setting controls the fraction of mutex contention events that are reported in the mutex profile"
}

// Hidden returns whether or not this setting is hidden from the list of runtime settings
func (r RuntimeMutexProfileFraction) Hidden() bool {
	return false
}

// Get returns the current value of the runtime setting
func (r RuntimeMutexProfileFraction) Get() (interface{}, error) {
	rate := runtime.SetMutexProfileFraction(-1) // docs: "To just read the current rate, pass rate < 0."
	return rate, nil
}

// Set changes the value of the runtime setting
func (r RuntimeMutexProfileFraction) Set(value interface{}) error {
	rate, err := GetInt(value)
	if err != nil {
		return err
	}

	runtime.SetMutexProfileFraction(rate)

	return nil
}
