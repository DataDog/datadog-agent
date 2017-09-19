package main

import (
	"bufio"
	"errors"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

func SetUp() error {
	// Manually fetch the API key (can't use config file because agent might not be running yet)
	file, e := os.Open("/etc/dd-agent/datadog.yaml")
	if e != nil {
		return errors.New("Can't open datadog.yaml: " + e.Error())
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.Fields(scanner.Text())

		if len(line) != 0 && line[0] == "api_key:" {
			if len(line) == 1 {
				return errors.New("no API key is set.")
			}

			apiKey = line[1]
			log.Println("API key: " + apiKey)
			break
		}
	}
	if e = scanner.Err(); e != nil {
		return e
	}

	// Make sure $DATADOG_ROOT is set (necessary for sending commands to the agent)
	env := os.Getenv("DATADOG_ROOT")
	if len(env) == 0 {
		return errors.New("$DATADOG_ROOT environment variable is not set.")
	}

	return nil
}

func ProcessCommand(w http.ResponseWriter, command string) {
	switch command {

	case "check_running":
		running, e := IsAgentRunning()
		if e != nil {
			w.Write([]byte("Error: " + e.Error()))
			log.Printf("Error: " + e.Error())
		} else if running {
			w.Write([]byte("Agent is running."))
		} else {
			w.Write([]byte("Agent is not running."))
		}

	case "start_agent":
		e := StartAgent()
		if e != nil {
			w.Write([]byte("Error: " + e.Error()))
			log.Printf("Error: " + e.Error())
		} else {
			w.Write([]byte("Agent has started."))
		}

	case "stop_agent":
		e := StopAgent()
		if e != nil {
			w.Write([]byte("Error: " + e.Error()))
			log.Printf("Error: " + e.Error())
		} else {
			w.Write([]byte("Agent has stopped."))
		}

	case "get_status":
		res, e := GetAgentStatus()
		if e != nil {
			w.Write([]byte("Response: " + res + " Error: " + e.Error()))
			log.Printf("Error: " + e.Error())
		} else {
			w.Write([]byte(res))
		}

	default:
		w.Write([]byte("Received unknown command: " + command))
		log.Printf("Received unknown command: %v ", command)
	}
}

func IsAgentRunning() (bool, error) {
	res, e := exec.Command("sh", "-c", "$DATADOG_ROOT/datadog-agent/bin/agent/agent status").Output()

	if e.Error() == "exit status 255" && strings.Contains(string(res), "Could not reach agent:") {
		return false, nil
	} else if e != nil {
		return false, e
	}
	return true, e
}

func StartAgent() error {
	res, e := exec.Command("sh", "-c", "$DATADOG_ROOT/datadog-agent/bin/agent/agent start > /tmp/agent.log 2> /tmp/err.log &").Output()
	log.Printf(string(res))

	// TODO: Make sure it started correctly

	return e
}

func StopAgent() error {

	// TODO: Check OS to determine how to kill

	return error(nil)
}

func GetAgentStatus() (string, error) {
	res, e := exec.Command("sh", "-c", "$DATADOG_ROOT/datadog-agent/bin/agent/agent status").Output()
	return string(res), e
}
