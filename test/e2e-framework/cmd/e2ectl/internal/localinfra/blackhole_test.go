// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package localinfra

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The managed sink must be a read-only, unprivileged, host-detached container:
// the recorded operator loop proved this exact shape, and the boundaries are
// asserted so drift is caught offline.
func TestBlackholeRunArgsBoundaries(t *testing.T) {
	args := blackholeRunArgs("dev-blackhole", "dev-net", "registry.datadoghq.com/agent:7.83.0", "/store/dev/sink/e2ectl")
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--name dev-blackhole", "--network dev-net", "--read-only",
		"--cap-drop=ALL", "--security-opt no-new-privileges", "--no-healthcheck",
		"-v /store/dev/sink/e2ectl:/tool:ro", "--entrypoint /tool",
		"receiver serve --type blackhole --listen 0.0.0.0:8080",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("run args missing %q: %s", want, joined)
		}
	}
	for _, banned := range []string{"--privileged", "-p ", "--publish", "--net=host", "/var/run/docker.sock"} {
		if strings.Contains(joined, banned) {
			t.Fatalf("run args must not contain %q: %s", banned, joined)
		}
	}
	if BlackholeContainer("dev") != "dev-blackhole" || BlackholeAgentURL("dev") != "http://dev-blackhole:8080" {
		t.Fatal("managed sink names/URL must stay deterministic")
	}
}

// Staging copies the running binary into the environment directory so the
// sink mount does not depend on the original binary's lifetime or path.
func TestStageBlackholeBinaryCopiesRunningExecutable(t *testing.T) {
	envDir := t.TempDir()
	staged, err := StageBlackholeBinary(envDir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(staged) != filepath.Join(envDir, "sink") {
		t.Fatalf("staged binary must live in the environment: %s", staged)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(staged)
	if err != nil {
		t.Fatal(err)
	}
	if len(want) == 0 || string(got) != string(want) {
		t.Fatal("staged sink binary differs from the running executable")
	}
	info, err := os.Stat(staged)
	if err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatal("staged sink binary must be executable")
	}
}
