package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/collector/py"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/sbinet/go-python"
)

func main() {
	var conf = flag.String("conf", "", "option path to datadog.yaml")
	var pythonScript = flag.String("py", "", "python script to run")
	flag.Parse()

	flag.Usage = func() {
		fmt.Println("This binary execute a python script in the context of the Datadog Agent.\n" +
			"This includes synthetic modules (Go module bind to Python), logging facilities, configuration setup, ...\n")

		fmt.Printf("Usage: %s [-conf datadog.yaml] -py PYTHON_FILE -- [ARGS FOR THE PYTHON SCRIPT]...\n", os.Args[0])
		flag.PrintDefaults()
	}

	if *pythonScript == "" {
		flag.Usage()
		os.Exit(1)
	}

	if *conf != "" {
		config.Datadog.SetConfigFile(*conf)
		confErr := config.Datadog.ReadInConfig()
		if confErr != nil {
			fmt.Printf("unable to parse Datadog config file, running with env variables: %s", confErr)
		}
	}

	runtime.LockOSThread()
	py.Initialize()
	gstate := python.PyGILState_Ensure()

	python.PySys_SetArgv(append([]string{*pythonScript}, flag.Args()...))
	err := python.PyRun_SimpleFile(*pythonScript)
	if err != nil {
		fmt.Printf("%s\n", err)
	}

	python.PyGILState_Release(gstate)
	runtime.UnlockOSThread()

	if err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
