package checks

import "github.com/op/go-logging"

var log = logging.MustGetLogger("datadog-agent")

// ConfigData contains YAML code
type ConfigData []byte

// Check is an interface for types capable to run checks
type Check interface {
	Run() error
	String() string
	Configure(data ConfigData)
}

// Runner waits for checks and run them as long as they arrive on the channel
func Runner(in <-chan Check) {
	log.Debug("Ready to process checks...")
	for check := range in {
		// create call arguments
		log.Infof("Running check %s", check)
		// run the check
		err := check.Run()
		if err != nil {
			log.Errorf("Error running check %s: %s", check, err)
		}
	}
}
