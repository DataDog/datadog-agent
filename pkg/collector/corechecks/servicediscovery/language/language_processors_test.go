// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package language

import (
	"os"
	"testing"
)

func Test_hasScript(t *testing.T) {
	data := []struct {
		name string
		file string
		want bool
	}{
		{
			name: "works",
			file: "yes_script.sh",
			want: true,
		},
		{
			name: "fails",
			file: "not_a_script.txt",
			want: false,
		},
	}
	for _, d := range data {
		t.Run(d.name, func(t *testing.T) {
			r, err := os.Open("testdata/hasScript/" + d.file)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			result := hasScript(r, "bash")
			if result != d.want {
				t.Errorf("got %t, want %t", result, d.want)
			}
		})
	}
}

func TestPythonScript(t *testing.T) {
	p := PythonScript{}
	if p.Language() != Python {
		t.Errorf("got %s, want %s", p.Language(), Python)
	}
	data := []struct {
		name string
		pi   ProcessInfo
		want bool
	}{
		{
			name: "works",
			pi: ProcessInfo{
				Args: []string{"testdata/python/yes_script.sh"},
				Envs: []string{"PATH=/usr/bin"},
			},
			want: true,
		},
		{
			name: "fails",
			pi: ProcessInfo{
				Args: []string{"testdata/python/not_a_script.sh"},
				Envs: []string{"PATH=/usr/bin"},
			},
			want: false,
		},
	}
	for _, d := range data {
		t.Run(d.name, func(t *testing.T) {
			found := p.Match(d.pi)
			if found != d.want {
				t.Errorf("got %t, want %t", found, d.want)
			}
		})
	}
}

func TestRubyScript(t *testing.T) {
	p := RubyScript{}
	if p.Language() != Ruby {
		t.Errorf("got %s, want %s", p.Language(), Ruby)
	}
	data := []struct {
		name string
		pi   ProcessInfo
		want bool
	}{
		{
			name: "works",
			pi: ProcessInfo{
				Args: []string{"testdata/ruby/yes_script.sh"},
				Envs: []string{"PATH=/usr/bin"},
			},
			want: true,
		},
		{
			name: "fails",
			pi: ProcessInfo{
				Args: []string{"testdata/ruby/not_a_script.sh"},
				Envs: []string{"PATH=/usr/bin"},
			},
			want: false,
		},
	}
	for _, d := range data {
		t.Run(d.name, func(t *testing.T) {
			found := p.Match(d.pi)
			if found != d.want {
				t.Errorf("got %t, want %t", found, d.want)
			}
		})
	}
}

func TestDotNetBinary(t *testing.T) {
	p := DotNetBinary{}
	if p.Language() != DotNet {
		t.Errorf("got %s, want %s", p.Language(), DotNet)
	}
	data := []struct {
		name string
		pi   ProcessInfo
		want bool
	}{
		{
			name: "works",
			pi: ProcessInfo{
				Args: []string{"testdata/dotnet/linuxdotnettest"},
				Envs: []string{"PATH=/usr/bin"},
			},
			want: true,
		},
		{
			name: "fails",
			pi: ProcessInfo{
				Args: []string{"testdata/dotnet/not_a_script.sh"},
				Envs: []string{"PATH=/usr/bin"},
			},
			want: false,
		},
	}
	for _, d := range data {
		t.Run(d.name, func(t *testing.T) {
			found := p.Match(d.pi)
			if found != d.want {
				t.Errorf("got %t, want %t", found, d.want)
			}
		})
	}
}
