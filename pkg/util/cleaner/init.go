package cleaner

import "github.com/DataDog/datadog-agent/pkg/util/log"

func init() {
	// set the scrubber for the log module early in process startup
	log.Scrubber = CredentialsCleanerBytes
}
