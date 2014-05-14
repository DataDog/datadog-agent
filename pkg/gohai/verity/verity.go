package main

import (
	"encoding/json"
	"github.com/DataDog/verity"
	"os"
)

func main() {
	verity_collected, err := verity.Collect()

	if err != nil {
		panic(err)
	}

	buf, err := json.Marshal(verity_collected)

	if err != nil {
		panic(err)
	}

	os.Stdout.Write(buf)
}
