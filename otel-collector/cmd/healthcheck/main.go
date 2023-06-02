package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
)

func main() {

	const host = "127.0.0.1" // default host
	usedPort := "13133"      // default port
	const path = "/"         //default path
	generateCmd := flag.NewFlagSet("generate", flag.ExitOnError)
	port := generateCmd.String("port", usedPort, "Specify collector health-check port")

	if len(os.Args) > 1 {
		err := generateCmd.Parse(os.Args[1:])
		if err != nil {
			log.Fatalf("%s", err)
		}
	}

	validationErr := validatePort(*port)
	if validationErr != nil {
		log.Fatalf("%s", validationErr)
	}

	status, healthCheckError := executeHealthCheck(host, port, path)

	if healthCheckError != nil {
		log.Fatalf(healthCheckError.Error())
	}

	log.Printf("%s", status)

}

func executeHealthCheck(host string, port *string, path string) (string, error) {

	resp, err := http.Get(fmt.Sprint("http://", host, ":", *port, path))
	if err != nil {
		return "", fmt.Errorf("unable to retrieve health status: %s", err.Error())
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("STATUS: %d", resp.StatusCode)
	}
	return fmt.Sprintf("STATUS: %d", resp.StatusCode), nil
}

// validatePort checks if the port configuration is valid
func validatePort(port string) error {

	portInt, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("invalid port: %w", err)
	}
	if portInt < 1 || portInt > 65535 {
		return fmt.Errorf("port outside of range [1:65535]: %d", portInt)
	}
	return nil

}
