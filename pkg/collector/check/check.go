package check

import (
	"bytes"
	"hash/fnv"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultCheckInterval is the interval in seconds the scheduler should apply
	// when no value was provided in Check configuration.
	DefaultCheckInterval time.Duration = 15 * time.Second
)

var (
	tplVarRegex = regexp.MustCompile(`%%.+?%%`)

	tplVars = []string{
		"host",
		"pid",
		"port",
		"container-name",
		"tags",
	}
)

// ConfigData contains YAML code
type ConfigData []byte

// ConfigRawMap is the generic type to hold YAML configurations
type ConfigRawMap map[interface{}]interface{}

// Config is a generic container for configuration files
type Config struct {
	Name          string       // the name of the check
	Instances     []ConfigData // array of Yaml configurations
	InitConfig    ConfigData   // the init_config in Yaml (python check only)
	ADIdentifiers []string     // the list of AutoDiscovery identifiers (optional)
}

// Check is an interface for types capable to run checks
type Check interface {
	Run() error                                    // run the check
	Stop()                                         // stop the check if it's running
	String() string                                // provide a printable version of the check name
	Configure(config, initConfig ConfigData) error // configure the check from the outside
	Interval() time.Duration                       // return the interval time for the check
	ID() ID                                        // provide a unique identifier for every check instance
	GetWarnings() []error                          // return the last warning registered by the check
}

// Stats holds basic runtime statistics about check instances
type Stats struct {
	CheckName         string
	CheckID           ID
	TotalRuns         uint64
	TotalErrors       uint64
	TotalWarnings     uint64
	ExecutionTimes    [32]int64 // circular buffer of recent run durations, most recent at [(TotalRuns+31) % 32]
	LastExecutionTime int64     // most recent run duration, provided for convenience
	LastError         string    // last occurred error message, if any
	LastWarnings      []string  // last occured warnings, if any
	UpdateTimestamp   int64     // latest update to this instance, unix timestamp in seconds
	m                 sync.Mutex
}

// NewStats returns a new check stats instance
func NewStats(c Check) *Stats {
	return &Stats{
		CheckID:   c.ID(),
		CheckName: c.String(),
	}
}

// Add tracks a new execution time
func (cs *Stats) Add(t time.Duration, err error, warnings []error) {
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
	if len(warnings) != 0 {
		cs.LastWarnings = []string{}
		for _, w := range warnings {
			cs.TotalWarnings++
			cs.LastWarnings = append(cs.LastWarnings, w.Error())
		}
	}
	cs.UpdateTimestamp = time.Now().Unix()
}

// Equal determines whether the passed config is the same
func (c *Config) Equal(config *Config) bool {
	if config == nil {
		return false
	}

	return c.Digest() == config.Digest()
}

// String YAML representation of the config
func (c *Config) String() string {
	var yamlBuff bytes.Buffer

	yamlBuff.Write([]byte("init_config:\n"))
	if c.InitConfig != nil {
		yamlBuff.Write([]byte("- "))
		strInit := strings.Split(string(c.InitConfig[:]), "\n")
		for i, line := range strInit {
			if i > 0 {
				yamlBuff.Write([]byte("  "))
			}
			yamlBuff.Write([]byte(line))
			yamlBuff.Write([]byte("\n"))
		}
	}

	yamlBuff.Write([]byte("instances:\n"))
	for _, instance := range c.Instances {
		strInst := strings.Split(string(instance[:]), "\n")
		yamlBuff.Write([]byte("- "))
		for i, line := range strInst {
			if i > 0 {
				yamlBuff.Write([]byte("  "))
			}
			yamlBuff.Write([]byte(line))
			yamlBuff.Write([]byte("\n"))
		}
	}

	return yamlBuff.String()
}

// IsTemplate returns if the config has AD identifiers and template variables
func (c *Config) IsTemplate() bool {
	// a template must have at least an AD identifier
	if len(c.ADIdentifiers) == 0 {
		return false
	}

	// init_config containing template tags
	if tplVarRegex.Match(c.InitConfig) {
		return true
	}

	// any of the instances containing template tags
	for _, inst := range c.Instances {
		if tplVarRegex.Match(inst) {
			return true
		}
	}

	return false
}

// GetTemplateVariables returns a slice of raw template variables
// it found in a config template.
// FIXME: only extracts from the first instance. Do we need more?
func (c *Config) GetTemplateVariables() (vars [][]byte) {
	if len(c.Instances) == 0 {
		return vars
	}
	return tplVarRegex.FindAll(c.Instances[0], -1)
}

// Digest returns an hash value representing the data stored in this configuration
func (c *Config) Digest() string {
	h := fnv.New64()
	h.Write([]byte(c.Name))
	for _, i := range c.Instances {
		h.Write([]byte(i))
	}
	h.Write([]byte(c.InitConfig))
	for _, i := range c.ADIdentifiers {
		h.Write([]byte(i))
	}

	return strconv.FormatUint(h.Sum64(), 16)
}
