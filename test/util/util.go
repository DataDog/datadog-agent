package util

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// RandomString generates a random string of the given size
func RandomString(size int) string {
	b := make([]byte, size)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

// TimeNowNano returns Unix time with nanosecond precision
func TimeNowNano() float64 {
	return float64(time.Now().UnixNano()) / float64(time.Second)
}

// InitLogging inits default logger
func InitLogging(level string) error {
	err := config.SetupLogger(level, "", "", false, true, false)
	if err != nil {
		return fmt.Errorf("Unable to initiate logger: %s", err)
	}

	return nil
}

// SetHostname sets the hostname
func SetHostname(hostname string) {
	mockConfig := config.NewMock()
	mockConfig.Set("hostname", hostname)
}
