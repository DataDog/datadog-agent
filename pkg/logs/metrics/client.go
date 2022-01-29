package metrics

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-go/statsd"
)

var dogstatsdClient = (*statsd.Client)(nil)

// InitDogStatsdClient initializes the grobal DogStatsd client.
func InitDogStatsdClient(conf config.Config) error {
	var err error
	dogstatsdClient, err = statsd.New(findAddr(conf))
	if err != nil {
		return err
	}
	return nil
}

func findAddr(conf config.Config) string {
	host := "localhost"
	if conf.IsSet("bind_host") {
		host = conf.GetString("bind_host")
	}

	port := 8125
	if conf.IsSet("dogstatsd_port") {
		port = conf.GetInt("dogstatsd_port")
	}
	return fmt.Sprintf("%s:%d", host, port)
}

// Count calls Count on the global Client, if set.
func Count(name string, value int64, tags []string, rate float64) error {
	if dogstatsdClient == nil {
		return nil // no-op
	}
	return dogstatsdClient.Count(name, value, tags, rate)
}

// Histogram calls Histogram on the global Client, if set.
func Histogram(name string, value float64, tags []string, rate float64) error {
	if dogstatsdClient == nil {
		return nil // no-op
	}
	return dogstatsdClient.Histogram(name, value, tags, rate)
}
