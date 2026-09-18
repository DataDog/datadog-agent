// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package localinfra

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type volumeFixture struct {
	t                *testing.T
	volume           RuntimeVolume
	exists           bool
	owner            string
	commands         []string
	state            string
	containerFailure bool
}

func (f *volumeFixture) run(args ...string) (string, error) {
	f.t.Helper()
	f.commands = append(f.commands, strings.Join(args, " "))
	if args[0] == "rm" {
		if f.containerFailure {
			return "", errors.New("container removal failed")
		}
		return "", nil
	}
	if args[0] != "volume" {
		f.t.Fatal(args)
	}
	switch args[1] {
	case "ls":
		if f.exists {
			return f.volume.Name + "\n" + f.volume.Name + "-unrelated", nil
		}
		return f.volume.Name + "-unrelated", nil
	case "inspect":
		if !f.exists {
			f.t.Fatal("inspected missing volume")
		}
		data, _ := json.Marshal(map[string]string{runtimeOwnerLabel: f.owner})
		return string(data), nil
	case "create":
		if f.exists {
			f.t.Fatal("recreated existing runtime state")
		}
		if args[4] != f.volume.Name {
			f.t.Fatal(args)
		}
		f.exists = true
		f.owner = strings.TrimPrefix(args[3], runtimeOwnerLabel+"=")
		return f.volume.Name, nil
	case "rm":
		if args[2] != f.volume.Name || len(args) != 3 {
			f.t.Fatalf("broad/unowned volume removal: %v", args)
		}
		f.exists = false
		f.state = ""
		return f.volume.Name, nil
	default:
		f.t.Fatalf("unexpected command: %v", args)
	}
	return "", nil
}
func TestRuntimeVolumeLifecycleReusesStateAndRemovesOnlyOwnedVolume(t *testing.T) {
	v := AgentRuntimeVolume("/private/envs/dev", time.Unix(10, 0))
	f := &volumeFixture{t: t, volume: v}
	if err := EnsureRuntimeVolume(v, f.run); err != nil {
		t.Fatal(err)
	}
	f.state = "private root-owned nested RC state"
	if err := EnsureRuntimeVolume(v, f.run); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRuntimeVolume(v, f.run); err != nil {
		t.Fatal(err)
	}
	if f.state == "" {
		t.Fatal("reuse discarded runtime state")
	}
	commands := strings.Join(f.commands, "\n")
	if strings.Count(commands, "volume create") != 1 {
		t.Fatal(commands)
	}
	f.commands = nil
	if err := RemoveAgentAndRuntime("dev-agent", v, f.run); err != nil {
		t.Fatal(err)
	}
	if f.exists || f.state != "" {
		t.Fatal("runtime volume leaked")
	}
	if f.commands[0] != "rm -f dev-agent" || f.commands[len(f.commands)-1] != "volume rm "+v.Name {
		t.Fatal(f.commands)
	}
	if strings.Contains(strings.Join(f.commands, " "), "prune") {
		t.Fatal("broad cleanup")
	}
}
func TestRuntimeVolumeRejectsForeignOwnershipAndMissingReuse(t *testing.T) {
	v := AgentRuntimeVolume("/private/envs/dev", time.Unix(10, 0))
	f := &volumeFixture{t: t, volume: v, exists: true, owner: "someone-else", state: "do not touch"}
	for _, operation := range []func(RuntimeVolume, DockerCommand) error{EnsureRuntimeVolume, VerifyRuntimeVolume, RemoveRuntimeVolume} {
		if operation(v, f.run) == nil {
			t.Fatal("foreign state accepted")
		}
	}
	if !f.exists || f.state != "do not touch" {
		t.Fatal("foreign state modified")
	}
	f.exists = false
	f.commands = nil
	if VerifyRuntimeVolume(v, f.run) == nil {
		t.Fatal("missing state accepted")
	}
	if strings.Contains(strings.Join(f.commands, " "), "create") {
		t.Fatal("missing reuse silently recreated volume")
	}
}
func TestRuntimeVolumeStopFailureRetainsStateAndOwnerIdentity(t *testing.T) {
	v := AgentRuntimeVolume("/private/envs/dev", time.Unix(10, 0))
	f := &volumeFixture{t: t, volume: v, exists: true, owner: v.Owner, state: "private", containerFailure: true}
	if RemoveAgentAndRuntime("dev-agent", v, f.run) == nil {
		t.Fatal("stop failure ignored")
	}
	if !f.exists || len(f.commands) != 1 {
		t.Fatal("state removed after failed container stop")
	}
	if AgentRuntimeVolume("/another/store/dev", time.Unix(10, 0)).Name == v.Name || AgentRuntimeVolume("/private/envs/dev", time.Unix(11, 0)).Name == v.Name {
		t.Fatal("environment generations share runtime ownership")
	}
	if RemoveRuntimeVolume(RuntimeVolume{Name: "unrelated"}, f.run) == nil {
		t.Fatal("unowned arbitrary name accepted")
	}
}
