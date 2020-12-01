// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build stresstests

package tests

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"runtime/pprof"
	"testing"
	"text/tabwriter"
	"time"

	"github.com/google/pprof/driver"
)

// StressOpts defines Stresser options
type StressOpts struct {
	KeepProfile bool
	ReportFile  string
	DiffBase    string
	TopFrom     string
	Duration    time.Duration
}

// StressFlag implements pprof Flag interface
type StressFlag struct {
	Path string
	Top  string
	From string
}

// Bool implements pprof Flag interface
func (s *StressFlag) Bool(name string, def bool, usage string) *bool {
	v := def

	switch name {
	case "top":
		v = true
	}

	return &v
}

// Int implements pprof Flag interface
func (s *StressFlag) Int(name string, def int, usage string) *int {
	v := def
	return &v
}

// Float64 implements pprof Flag interface
func (s *StressFlag) Float64(name string, def float64, usage string) *float64 {
	v := def
	return &v
}

// String implements pprof Flag interface
func (s *StressFlag) String(name string, def string, usage string) *string {
	v := def

	switch name {
	case "output":
		v = s.Top
	case "show_from":
		v = s.From
	}

	return &v
}

// StringList implements pprof Flag interface
func (s *StressFlag) StringList(name string, def string, usage string) *[]*string {
	v := []*string{&def}
	return &v
}

// ExtraUsage implements pprof Flag interface
func (s *StressFlag) ExtraUsage() string {
	return ""
}

// AddExtraUsage implements pprof Flag interface
func (s *StressFlag) AddExtraUsage(eu string) {}

// Parse implements pprof Flag interface
func (s *StressFlag) Parse(usage func()) []string {
	return []string{s.Path}
}

// StressReport defines a Stresser report
type StressReport struct {
	Duration      time.Duration
	Iteration     int
	BaseIteration int `json:",omitempty"`
	Extras        map[string]struct {
		Value float64
		Unit  string
	} `json:",omitempty"`
	Top []byte `json:",omitempty"`
}

// AddMetric add custom metrics to the report
func (s *StressReport) AddMetric(name string, value float64, unit string) {
	if s.Extras == nil {
		s.Extras = map[string]struct {
			Value float64
			Unit  string
		}{}
	}
	s.Extras[name] = struct {
		Value float64
		Unit  string
	}{
		Value: value,
		Unit:  unit,
	}
}

// Delta returns the delta between the base and the currrent report in percentage
func (s *StressReport) Delta() float64 {
	if s.BaseIteration != 0 {
		return float64(s.Iteration-s.BaseIteration) * 100.0 / float64(s.BaseIteration)
	}

	return 0
}

// Print prints the report in a human readable format
func (s *StressReport) Print() {
	fmt.Println("----- Stress Report -----")
	w := tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', tabwriter.AlignRight)
	fmt.Fprintf(w, "%s\t\t%d iterations\t%15.4f ns/iteration", s.Duration, s.Iteration, float64(s.Duration.Nanoseconds())/float64(s.Iteration))
	if s.Extras != nil {
		for _, metric := range s.Extras {
			fmt.Fprintf(w, "\t%15.4f %s", metric.Value, metric.Unit)
		}
	}

	if delta := s.Delta(); delta != 0 {
		fmt.Fprintf(w, "\t%15.4f %%iterations", delta)
	}

	fmt.Fprintln(w)
	w.Flush()

	fmt.Println()
	fmt.Println("----- Profiling Report -----")
	fmt.Println(string(s.Top))
}

// Write the report information for delta computation
func (s *StressReport) Write(filename string) error {
	if filename == "" {
		file, err := ioutil.TempFile("/tmp", "stress-report-")
		if err != nil {
			return err
		}
		file.Close()

		filename = file.Name()
	}

	fmt.Printf("Writing state in %s\n", filename)

	j, _ := json.Marshal(s)
	err := ioutil.WriteFile(filename, j, 0644)
	if err != nil {
		return err
	}

	return nil
}

// Parse previous report
func (s *StressReport) Parse(filename string) error {
	jsonFile, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer jsonFile.Close()

	data, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, s); err != nil {
		return err
	}

	return nil
}

// StressIt starts the stress test
func StressIt(t *testing.T, pre, post, fnc func() error, opts StressOpts) (StressReport, error) {
	proFile, err := ioutil.TempFile("/tmp", "stress-profile-")
	if err != nil {
		t.Fatal(err)
	}

	if !opts.KeepProfile {
		defer os.Remove(proFile.Name())
	} else {
		fmt.Printf("Generating profile in %s\n", proFile.Name())
	}

	topFile, err := ioutil.TempFile("/tmp", "stress-top-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(topFile.Name())

	if err := pre(); err != nil {
		t.Fatal(err)
	}

	if err := pprof.StartCPUProfile(proFile); err != nil {
		t.Fatal(err)
	}

	done := make(chan bool)
	var iteration int

	start := time.Now()

	go func() {
		time.Sleep(opts.Duration)
		done <- true
	}()

LOOP:
	for {
		select {
		case <-done:
			break LOOP
		default:
			err = fnc()
			iteration++

			if err != nil {
				break LOOP
			}
		}
	}

	pprof.StopCPUProfile()
	proFile.Close()

	if err := post(); err != nil {
		t.Fatal(err)
	}

	// generate report
	flagSet := &StressFlag{Path: proFile.Name(), Top: topFile.Name(), From: opts.TopFrom}

	if err := driver.PProf(&driver.Options{Flagset: flagSet}); err != nil {
		t.Fatal(err)
	}

	topData, err := ioutil.ReadFile(topFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	report := StressReport{
		Duration:  time.Now().Sub(start),
		Iteration: iteration,
		Top:       topData,
	}

	if opts.DiffBase != "" {
		var baseReport StressReport
		if err := baseReport.Parse(diffBase); err != nil {
			t.Fatal(err)
		}
		report.BaseIteration = baseReport.Iteration
	}

	// save report for further comparison
	if err := report.Write(opts.ReportFile); err != nil {
		t.Fatal(err)
	}

	return report, err
}
