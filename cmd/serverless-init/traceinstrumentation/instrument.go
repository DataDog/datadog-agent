package traceinstrumentation

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// AutoInstrumentTracer searches the filesystem for a trace library, and
// automatically sets the correct environment variables.
func AutoInstrumentTracer() {
	// user has specifically disabled this
	// if !shouldAutoInstrument {
	// 	return
	// }

	dir, err := dirExists("/dd_tracer/node/")
	if err != nil {
		log.Debugf("Error checking if directory exists: %v", err)
		return
	}

	if !dir {
		log.Debug("Skipping tracer instrumentation as tracer library wasn't found")
		return
	}

	log.Debug("Instrumenting Node.js tracer")

	//
	currNodePath := os.Getenv("NODE_PATH")
	os.Setenv("NODE_PATH", addToString(currNodePath, ":", "/dd_tracer/node/"))

	currNodeOptions := os.Getenv("NODE_OPTIONS")
	os.Setenv("NODE_OPTIONS", addToString(currNodeOptions, " ", "--require dd-trace/init"))

	os.Setenv("DD_TRACE_PROPAGATION_STYLE", "datadog")
}

func dirExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func addToString(path string, separator string, token string) string {
	if path == "" {
		return token
	}

	return path + separator + token
}
