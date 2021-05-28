package statsd

import (
	"fmt"

	"github.com/DataDog/datadog-go/statsd"
)

// Client is a global Statsd client. When a client is configured via Configure,
// that becomes the new global Statsd client in the package.
var Client *statsd.Client

// Configure creates a statsd client from a dogweb.ini style config file and set it to the global Statsd.
func Configure(host string, port int) error {
	client, err := statsd.New(fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return err
	}

	Client = client
	return nil
}
