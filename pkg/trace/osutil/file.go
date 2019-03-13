package osutil

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/trace/flags"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Exists reports whether the given path exists.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// Exit prints the message and exits the program with status code 1.
func Exit(msg string) {
	if flags.Info || flags.Version {
		fmt.Println(msg)
	} else {
		log.Error(msg)
		log.Flush()
	}
	os.Exit(1)
}

// Exitf prints the formatted text and exits the program with status code 1.
func Exitf(format string, args ...interface{}) {
	if flags.Info || flags.Version {
		fmt.Printf(format, args...)
		fmt.Print("")
	} else {
		log.Errorf(format, args...)
		log.Flush()
	}
	os.Exit(1)
}
