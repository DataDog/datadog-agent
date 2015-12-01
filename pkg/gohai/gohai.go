package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/DataDog/gohai/cpu"
	"github.com/DataDog/gohai/filesystem"
	"github.com/DataDog/gohai/memory"
	"github.com/DataDog/gohai/network"
	"github.com/DataDog/gohai/platform"
)

type Collector interface {
	Name() string
	Collect() (interface{}, error)
}

var collectors = []Collector{
	&cpu.Cpu{},
	&filesystem.FileSystem{},
	&memory.Memory{},
	&network.Network{},
	&platform.Platform{},
}

var options struct {
	version bool
}

// version information filled in at build time
var (
	buildDate string
	gitCommit string
	gitBranch string
	goVersion string
)

func Collect() (result map[string]interface{}, err error) {
	result = make(map[string]interface{})

	for _, collector := range collectors {
		c, err := collector.Collect()
		if err != nil {
			log.Printf("[%s] %s", collector.Name(), err)
			continue
		}
		result[collector.Name()] = c
	}

	result["gohai"] = versionMap()

	return
}

func versionMap() (result map[string]interface{}) {
	result = make(map[string]interface{})

	result["git_hash"] = gitCommit
	result["git_branch"] = gitBranch
	result["build_date"] = buildDate
	result["go_version"] = goVersion

	return
}

func versionString() string {
	var buf bytes.Buffer

	if gitCommit != "" {
		fmt.Fprintf(&buf, "Git hash: %s\n", gitCommit)
	}
	if gitBranch != "" {
		fmt.Fprintf(&buf, "Git branch: %s\n", gitBranch)
	}
	if buildDate != "" {
		fmt.Fprintf(&buf, "Build date: %s\n", buildDate)
	}
	if goVersion != "" {
		fmt.Fprintf(&buf, "Go Version: %s\n", goVersion)
	}

	return buf.String()
}

func init() {
	flag.BoolVar(&options.version, "version", false, "Show version information and exit")
	flag.Parse()
}

func main() {
	if options.version {
		fmt.Printf("%s", versionString())
		os.Exit(0)
	}

	gohai, err := Collect()

	if err != nil {
		panic(err)
	}

	buf, err := json.Marshal(gohai)

	if err != nil {
		panic(err)
	}

	os.Stdout.Write(buf)
}
