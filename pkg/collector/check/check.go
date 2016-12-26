package check

import (
	"time"
)

// DefaultCheckInterval is the interval in seconds the scheduler should apply
// when no value was provided in Check configuration.
const DefaultCheckInterval time.Duration = 15

// ConfigData contains YAML code
type ConfigData []byte

// ConfigRawMap is the generic type to hold YAML configurations
type ConfigRawMap map[interface{}]interface{}

// Config is a generic container for configuration files
type Config struct {
	Name      string       // the name of the check
	Instances []ConfigData // array of Yaml configurations
}

// Check is an interface for types capable to run checks
type Check interface {
	Run() error                      // run the check
	Stop()                           // stop the check if it's running
	String() string                  // provide a printable version of the check name
	Configure(data ConfigData) error // configure the check from the outside
	InitSender()                     // initialize what's needed to send data to the aggregator
	Interval() time.Duration         // return the interval time for the check
	ID() string                      // provide a unique identifier for every check instance
}
