package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 3 || os.Args[1] != "arg1" || os.Args[2] != "arg2" {
		os.Exit(1)
	}
	fmt.Printf("{\"handle1\":{\"value\":\"arg_password\"}}")
}
