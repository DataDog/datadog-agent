// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/py"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/sbinet/go-python"
)

func command(name string, arg ...string) ([]byte, error) {
	ctxt, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctxt, name, arg...)

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Start(); err != nil {
		return buf.Bytes(), err
	}

	if err := cmd.Wait(); err != nil {
		return buf.Bytes(), err
	}

	return buf.Bytes(), nil
}

type stickyLock struct {
	gstate python.PyGILState
	locked uint32 // Flag set to 1 if the lock is locked, 0 otherwise
}

func newStickyLock() *stickyLock {
	runtime.LockOSThread()
	return &stickyLock{
		gstate: python.PyGILState_Ensure(),
		locked: 1,
	}
}

func (sl *stickyLock) unlock() {
	atomic.StoreUint32(&sl.locked, 0)
	python.PyGILState_Release(sl.gstate)
	runtime.UnlockOSThread()
}

func main() {
	runtime.GOMAXPROCS(4)
	fmt.Printf("GOMAXPROCS: %v\n", runtime.GOMAXPROCS(0))
	var conf = flag.String("conf", "", "option path to datadog.yaml")
	var pythonScript = flag.String("py", "", "python script to run")
	var goroutines = flag.Int("goroutines", 100, "number of goroutines to start on each iteration")
	flag.Parse()

	flag.Usage = func() {
		fmt.Println("Execute python code that create subprocesses and some other goroutines that also create subprocesses.\n")

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

	py.Initialize()

	lock := newStickyLock()
	module := python.PyImport_ImportModule(*pythonScript)
	if module == nil {
		fmt.Print("ERROR IMPORTING PYTHON MODULE\n")
		os.Exit(1)
	}
	function := module.GetAttrString("function")
	if function == nil {
		fmt.Print("ERROR FINDING FUNCTION\n")
		os.Exit(1)
	}
	lock.unlock()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	j := 0
loop:
	for {
		select {
		case <-signalCh:
			break loop
		default:
			fmt.Printf("Iteration %v\n", j)
			wg := sync.WaitGroup{}

			for i := 0; i < *goroutines; i++ {
				wg.Add(1)
				go func() {

					lock := newStickyLock()

					emptyTuple := python.PyTuple_New(0)
					defer emptyTuple.DecRef()

					wg.Add(1)
					go func() {
						_, err := command("ss", "-atun")
						if err != nil {
							fmt.Printf("%s\n", err)
						}
						wg.Done()
					}()

					result := function.CallObject(emptyTuple)

					wg.Add(1)
					go func() {
						_, err := command("ss", "-atun")
						if err != nil {
							fmt.Printf("%s\n", err)
						}
						wg.Done()
					}()

					if result == nil {
						fmt.Print("ERROR EXECUTING FUNCTION\n")
						pyError, err := lock.getPythonError()
						if err != nil {
							fmt.Printf("error fetching py error: %v\n", err)
						} else {
							fmt.Print(pyError)
						}
					}
					wg.Done()
					lock.unlock()
				}()
				wg.Add(1)
				go func() {
					_, err := command("ss", "-atun")
					if err != nil {
						fmt.Printf("%s\n", err)
					}
					wg.Done()
				}()
			}

			wg.Wait()
			j++
		}
	}

	fmt.Print("Stop")
	time.Sleep(20 * time.Second)
}

func (sl *stickyLock) getPythonError() (string, error) {
	if atomic.LoadUint32(&sl.locked) != 1 {
		return "", fmt.Errorf("the stickyLock is unlocked, can't interact with python interpreter")
	}

	if python.PyErr_Occurred() == nil { // borrowed ref, no decref needed
		return "", fmt.Errorf("the error indicator is not set on the python interpreter")
	}

	ptype, pvalue, ptraceback := python.PyErr_Fetch() // new references, have to be decref'd
	defer python.PyErr_Clear()
	defer ptype.DecRef()
	defer pvalue.DecRef()
	defer ptraceback.DecRef()

	// Make sure exception values are normalized, as per python C API docs. No error to handle here
	python.PyErr_NormalizeException(ptype, pvalue, ptraceback)

	if ptraceback != nil && ptraceback.GetCPointer() != nil {
		// There's a traceback, try to format it nicely
		traceback := python.PyImport_ImportModule("traceback")
		formatExcFn := traceback.GetAttrString("format_exception")
		if formatExcFn != nil {
			defer formatExcFn.DecRef()
			pyFormattedExc := formatExcFn.CallFunction(ptype, pvalue, ptraceback)
			if pyFormattedExc != nil {
				defer pyFormattedExc.DecRef()
				pyStringExc := pyFormattedExc.Str()
				if pyStringExc != nil {
					defer pyStringExc.DecRef()
					return python.PyString_AsString(pyStringExc), nil
				}
			}
		}

		// If we reach this point, there was an error while formatting the exception
		return "", fmt.Errorf("can't format exception")
	}

	// we sometimes do not get a traceback but an error in pvalue
	if pvalue != nil && pvalue.GetCPointer() != nil {
		strPvalue := pvalue.Str()
		if strPvalue != nil {
			defer strPvalue.DecRef()
			return python.PyString_AsString(strPvalue), nil
		}
	}

	if ptype != nil {
		strPtype := ptype.Str()
		if strPtype != nil {
			defer strPtype.DecRef()
			return python.PyString_AsString(strPtype), nil
		}
	}

	return "", fmt.Errorf("unknown error")
}
