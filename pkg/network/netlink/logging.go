package netlink

import (
	"bufio"
	"io"
	"log"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	agentlog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// getLogger creates a log.Logger which forwards logs to the agent's logging package at DEBUG leve.
// Returns nil if the agent loggers level is above DEBUG.
func getLogger() *log.Logger {
	if level := strings.ToUpper(config.Datadog.GetString("log_level")); level != "DEBUG" {
		return nil
	}

	reader, writer := io.Pipe()

	flags := 0
	prefix := ""

	logger := log.New(writer, prefix, flags)

	go forwardLogs(reader)

	return logger
}

func forwardLogs(rd io.Reader) {
	scanner := bufio.NewScanner(rd)

	for scanner.Scan() {
		agentlog.Debugf("go-conntrack: %s", scanner.Text())
	}
}
