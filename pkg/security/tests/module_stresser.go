// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build stresstests

package tests

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
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

// StressReports represents a map of StressReport
type StressReports map[string]*StressReport

// StressReport defines a Stresser report
type StressReport struct {
	Duration      time.Duration
	Iteration     int
	BaseIteration int `json:",omitempty"`
	Extras        map[string]struct {
		Value float64
		Unit  string
	} `json:",omitempty"`
	TopCPU []byte `json:"-"`
	TopMem []byte `json:"-"`
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
func (s *StressReport) Print(t *testing.T) {
	fmt.Printf("----- Stress Report for %s -----\n", t.Name())
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
	fmt.Printf("----- Profiling Report CPU for %s -----\n", t.Name())
	fmt.Println(string(s.TopCPU))
	fmt.Println()

	fmt.Println()
	fmt.Printf("----- Profiling Report Memory for %s -----\n", t.Name())
	fmt.Println(string(s.TopMem))
	fmt.Println()
}

// Save writes the report information for delta computation
func (s *StressReport) Save(filename string, name string) error {
	var reports StressReports
	if err := reports.Load(filename); err != nil {
		reports = map[string]*StressReport{
			name: s,
		}
	} else {
		reports[name] = s
	}

	fmt.Printf("Writing reports in %s\n", filename)

	j, _ := json.Marshal(reports)
	return os.WriteFile(filename, j, 0644)
}

// Load previous report
func (s *StressReports) Load(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, s)
}

func getTopData(filename string, from string, size int) ([]byte, error) {
	topFile, err := os.CreateTemp("/tmp", "stress-top-")
	if err != nil {
		return nil, err
	}
	defer os.Remove(topFile.Name())

	flagSet := &StressFlag{Path: filename, Top: topFile.Name(), From: from}

	if err := driver.PProf(&driver.Options{Flagset: flagSet}); err != nil {
		return nil, err
	}

	file, err := os.Open(topFile.Name())
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)

	var topLines []string
	for scanner.Scan() {
		topLines = append(topLines, scanner.Text())
		if len(topLines) > size {
			break
		}
	}
	file.Close()

	return []byte(strings.Join(topLines, "\n")), nil
}

// StressIt starts the stress test
func StressIt(t *testing.T, pre, post, fnc func() error, opts StressOpts) (StressReport, error) {
	var report StressReport

	proCPUFile, err := os.CreateTemp("/tmp", "stress-cpu-")
	if err != nil {
		t.Error(err)
		return report, err
	}

	if !opts.KeepProfile {
		defer os.Remove(proCPUFile.Name())
	} else {
		fmt.Printf("Generating CPU profile in %s\n", proCPUFile.Name())
	}

	if pre != nil {
		if err := pre(); err != nil {
			t.Error(err)
			return report, err
		}
	}

	if err := pprof.StartCPUProfile(proCPUFile); err != nil {
		t.Error(err)
		return report, err
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

	duration := time.Now().Sub(start)

	pprof.StopCPUProfile()
	proCPUFile.Close()

	runtime.GC()
	proMemFile, err := os.CreateTemp("/tmp", "stress-mem-")
	if err != nil {
		t.Error(err)
		return report, err
	}

	if !opts.KeepProfile {
		defer os.Remove(proMemFile.Name())
	} else {
		fmt.Printf("Generating Memory profile in %s\n", proMemFile.Name())
	}

	if err := pprof.WriteHeapProfile(proMemFile); err != nil {
		t.Error(err)
		return report, err
	}

	if post != nil {
		if err := post(); err != nil {
			t.Error(err)
			return report, err
		}
	}

	topDataCPU, err := getTopData(proCPUFile.Name(), opts.TopFrom, 50)
	if err != nil {
		t.Error(err)
		return report, err
	}

	topDataMem, err := getTopData(proMemFile.Name(), opts.TopFrom, 50)
	if err != nil {
		t.Error(err)
		return report, err
	}

	report = StressReport{
		Duration:  duration,
		Iteration: iteration,
		TopCPU:    topDataCPU,
		TopMem:    topDataMem,
	}

	if opts.DiffBase != "" {
		var baseReports StressReports
		if err := baseReports.Load(opts.DiffBase); err != nil {
			t.Log(err)
		} else {
			baseReport, exists := baseReports[t.Name()]
			if exists {
				report.BaseIteration = baseReport.Iteration
			}
		}
	}

	// save report for further comparison
	if opts.ReportFile != "" {
		if err := report.Save(opts.ReportFile, t.Name()); err != nil {
			t.Error(err)
			return report, err
		}
	}

	return report, err
}
