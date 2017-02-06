package check

import (
	"sync"
	"time"
)

// DefaultCheckInterval is the interval in seconds the scheduler should apply
// when no value was provided in Check configuration.
const DefaultCheckInterval time.Duration = 15 * time.Second

// ConfigData contains YAML code
type ConfigData []byte

// ConfigRawMap is the generic type to hold YAML configurations
type ConfigRawMap map[interface{}]interface{}

// Config is a generic container for configuration files
type Config struct {
	Name       string       // the name of the check
	Instances  []ConfigData // array of Yaml configurations
	InitConfig ConfigData   // the init_config in Yaml (python check only)
}

// Check is an interface for types capable to run checks
type Check interface {
	Run() error                                             // run the check
	Stop()                                                  // stop the check if it's running
	String() string                                         // provide a printable version of the check name
	Configure(data ConfigData, initConfig ConfigData) error // configure the check from the outside
	InitSender()                                            // initialize what's needed to send data to the aggregator
	Interval() time.Duration                                // return the interval time for the check
	ID() ID                                                 // provide a unique identifier for every check instance
}

// Stats holds basic runtime statistics about check instances
type Stats struct {
	CheckName         string
	CheckID           string
	TotalRuns         uint64
	TotalErrors       uint64
	ExecutionTimes    [32]int64 // circular buffer of recent run durations, most recent at [(TotalRuns+31) % 32]
	LastExecutionTime int64     // most recent run duration, provided for convenience
	LastError         string    // last occurred error message, if any
	UpdateTimestamp   int64     // latest update to this instance, unix timestamp in seconds
	m                 sync.Mutex
}

func newStats(c Check) *Stats {
	return &Stats{
		CheckID:   c.ID(),
		CheckName: c.String(),
	}
}

func (cs *Stats) add(t time.Duration, err error) {
	cs.m.Lock()
	defer cs.m.Unlock()

	// store execution times in Milliseconds
	tms := t.Nanoseconds() / 1e6
	cs.LastExecutionTime = tms
	cs.ExecutionTimes[cs.TotalRuns] = tms
	cs.TotalRuns = (cs.TotalRuns + 1) % 32
	if err != nil {
		cs.TotalErrors++
		cs.LastError = err.Error()
	}
	cs.UpdateTimestamp = time.Now().Unix()
}
