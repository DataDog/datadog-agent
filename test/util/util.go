package util

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func RandomString(size int) string {
	b := make([]byte, size)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

func TimeNowNano() float64 {
	return float64(time.Now().UnixNano()) / float64(time.Second) // Unix time with nanosecond precision
}

func InitLogging(level string) error {
	err := config.SetupLogger(level, "", "", false, false, "")
	if err != nil {
		return fmt.Errorf("Unable to initiate logger: %s", err)
	}

	return nil
}

func SetHostname(hostname string) {
	config.Datadog.Set("hostname", hostname)
}
