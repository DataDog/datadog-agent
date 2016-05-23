package checks

import "github.com/op/go-logging"

var log = logging.MustGetLogger("datadog-agent")

// Check is an interface for types capable to run checks
type Check interface {
	Run() (CheckResult, error)
	String() string
}

// CheckResult wraps results from Python check
type CheckResult struct {
	// TODO add creation timestamp?
	Result string
	Error  string
}

// Runner waits for checks and run them as long as they arrive on the channel
func Runner(in <-chan Check, out chan<- CheckResult) {
	log.Debug("Ready to process checks...")
	for check := range in {
		// create call arguments
		log.Infof("Running check %s", check)
		// run the check
		result, err := check.Run()
		if err != nil {
			log.Errorf("Error running check %s: %s", check, err)
		}
		out <- result
	}
}
