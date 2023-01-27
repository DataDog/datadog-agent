package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Datadog/datadog-agent/test/fake-intake/fakeintake"
)

func main() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	done := make(chan bool, 1)

	fmt.Println("⌛️ Starting fake intake")
	fi := fakeintake.NewFakeIntake()
	fmt.Println("🏃 Fake intake running")

	go func() {
		<-sigs
		fmt.Println("Stopping fake intake")
		err := fi.Stop()
		if err != nil {
			fmt.Println("Error stopping fake intake, ", err)
		}
		done <- true
	}()

	<-done
	fmt.Println("Fake intake is stopped")
	fmt.Println("👋 Bye bye")
}
