package main

import (
	"encoding/json"
	"github.com/kentaro/verity"
	"os"
)

func main() {
	verity, err := verity.Collect()

	if err != nil {
		panic(err)
	}

	buf, err := json.Marshal(verity)

	if err != nil {
		panic(err)
	}

	os.Stdout.Write(buf)
}
