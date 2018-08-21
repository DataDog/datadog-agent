package main

import (
	"bufio"
	"fmt"
	"os"
)

func main() {
	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	if text != "{\"version\": \"1.0\" , \"secrets\": [\"sec1\", \"sec2\"]}" {
		os.Exit(1)
	}
	fmt.Printf("{\"handle1\":{\"value\":\"input_password\"}}")
}
