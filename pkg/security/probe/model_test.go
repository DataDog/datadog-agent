// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"net"
	"reflect"
	"sort"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func TestSetFieldValue(t *testing.T) {
	event := &Event{}

	for _, field := range event.GetFields() {
		kind, err := event.GetFieldType(field)
		if err != nil {
			t.Fatal(err)
		}

		switch kind {
		case reflect.String:
			if err = event.SetFieldValue(field, "aaa"); err != nil {
				t.Error(err)
			}
		case reflect.Int:
			if err = event.SetFieldValue(field, 123); err != nil {
				t.Error(err)
			}
		case reflect.Bool:
			if err = event.SetFieldValue(field, true); err != nil {
				t.Error(err)
			}
		case reflect.Struct:
			switch field {
			case "network.destination.ip", "network.source.ip":
				_, ipnet, err := net.ParseCIDR("127.0.0.1/24")
				if err != nil {
					t.Error(err)
				}

				if err = event.SetFieldValue(field, *ipnet); err != nil {
					t.Error(err)
				}
			}
		default:
			t.Errorf("type of field %s unknown: %v", field, kind)
		}
	}
}

func TestProcessArgsFlags(t *testing.T) {
	e := Event{
		Event: model.Event{
			Exec: model.ExecEvent{
				Process: &model.Process{
					ArgsEntry: &model.ArgsEntry{
						Values: []string{
							"cmd", "-abc", "--verbose", "test",
							"-v=1", "--host=myhost",
							"-9", "-", "--",
						},
					},
				},
			},
		},
	}

	resolver, _ := NewProcessResolver(&Probe{}, nil, NewProcessResolverOpts(10000))
	e.resolvers = &Resolvers{
		ProcessResolver: resolver,
	}

	flags := e.ResolveProcessArgsFlags(e.Exec.Process)
	sort.Strings(flags)

	hasFlag := func(flags []string, flag string) bool {
		i := sort.SearchStrings(flags, flag)
		return i < len(flags) && flags[i] == flag
	}

	if !hasFlag(flags, "a") {
		t.Error("flags 'a' not found")
	}

	if !hasFlag(flags, "b") {
		t.Error("flags 'b' not found")
	}

	if hasFlag(flags, "v") {
		t.Error("flags 'v' found")
	}

	if !hasFlag(flags, "9") {
		t.Error("flags '9' not found found")
	}

	if !hasFlag(flags, "abc") {
		t.Error("flags 'abc' not found")
	}

	if !hasFlag(flags, "verbose") {
		t.Error("flags 'verbose' not found")
	}

	if len(flags) != 6 {
		t.Errorf("expected 6 flags, got %d", len(flags))
	}
}

func TestProcessArgsOptions(t *testing.T) {
	e := Event{
		Event: model.Event{
			Exec: model.ExecEvent{
				Process: &model.Process{
					ArgsEntry: &model.ArgsEntry{
						Values: []string{
							"cmd", "--config", "/etc/myfile", "--host=myhost", "--verbose",
							"-c", "/etc/myfile", "-e", "", "-h=myhost", "-v",
							"--", "---", "-9",
						},
					},
				},
			},
		},
	}

	resolver, _ := NewProcessResolver(&Probe{}, nil, NewProcessResolverOpts(10000))
	e.resolvers = &Resolvers{
		ProcessResolver: resolver,
	}

	options := e.ResolveProcessArgsOptions(e.Exec.Process)
	sort.Strings(options)

	hasOption := func(options []string, option string) bool {
		i := sort.SearchStrings(options, option)
		return i < len(options) && options[i] == option
	}

	if !hasOption(options, "config=/etc/myfile") {
		t.Error("option 'config=/etc/myfile' not found")
	}

	if !hasOption(options, "c=/etc/myfile") {
		t.Error("option 'c=/etc/myfile' not found")
	}

	if !hasOption(options, "e=") {
		t.Error("option 'e=' not found")
	}

	if !hasOption(options, "host=myhost") {
		t.Error("option 'host=myhost' not found")
	}

	if !hasOption(options, "h=myhost") {
		t.Error("option 'h=myhost' not found")
	}

	if hasOption(options, "verbose=") {
		t.Error("option 'verbose=' found")
	}

	if len(options) != 5 {
		t.Errorf("expected 5 options, got %d", len(options))
	}
}
