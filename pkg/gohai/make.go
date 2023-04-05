// +build ignore

// Builds Gohai with version information
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

func commandOutput(name string, args ...string) string {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		log.Fatal(err)
	}

	return strings.TrimSpace(string(out))
}

func main() {
	gobin := "go"
	if len(os.Args) > 1 {
		gobin = os.Args[1]
	}
	date := time.Now().Format(time.RFC3339)
	commit := commandOutput("git", "rev-parse", "--short", "HEAD")
	branch := commandOutput("git", "rev-parse", "--abbrev-ref", "HEAD")
	version := commandOutput(gobin, "version")

	// expected go_version output: go version go1.9.2 darwin/amd64
	versionRune := []rune(strings.Split(version, " ")[2])
	goVersion := string(versionRune[2:])

	var ldflags string
	ldflags = fmt.Sprintf("-X main.buildDate=%s -X main.gitCommit=%s -X main.gitBranch=%s -X main.goVersion=%s", date, commit, branch, goVersion)

	cmd := exec.Command(gobin, []string{"build", "-a", "-ldflags", ldflags}...)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		os.Exit(1)
	}
}
