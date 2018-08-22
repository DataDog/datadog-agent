package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/procmatch"
)

func main() {
	fmt.Println(procmatch.Match(strings.Join(os.Args[1:], " ")))
}
