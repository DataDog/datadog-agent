// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentbuild

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BinaryMountArgs mounts only verified staged outputs, not a source checkout.
func BinaryMountArgs(b BinaryBundle) []string {
	return []string{"-v", b.Executable.Path + ":/opt/datadog-agent/bin/agent/agent:ro", "-v", b.Runtime.Root + ":" + b.RuntimePrefix + ":ro", "-v", b.Assets.Root + ":/opt/e2ectl/assets:ro", "-e", "LD_LIBRARY_PATH=" + b.RuntimePrefix + "/lib", "-e", "PYTHONPATH=" + b.PythonPath}
}

type runtimeMetadata struct {
	ABI     string `json:"abi"`
	Path    string `json:"path"`
	OS      string `json:"os"`
	Version string `json:"version"`
}

func (a Adapter) inspectRuntimeImage(ctx context.Context, reference string, target Target) (Image, runtimeMetadata, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	image, err := a.InspectImage(ctx, reference, target)
	if err != nil {
		return Image{}, runtimeMetadata{}, err
	}
	out, err := a.runContainer(ctx, "", []string{"--rm", "--pull=never", "--network=none", "--entrypoint", "/opt/datadog-agent/embedded/bin/python3", image.ID, "-c", `import sys,sysconfig,json; o=dict(line.strip().split('=',1) for line in open('/etc/os-release') if '=' in line); print(json.dumps(dict(abi='python%d.%d'%sys.version_info[:2],path=sysconfig.get_path('purelib'),os=o['ID'].strip('"'),version=o['VERSION_ID'].strip('"'))))`})
	if err != nil {
		return Image{}, runtimeMetadata{}, err
	}
	var metadata runtimeMetadata

	if err = json.Unmarshal(out, &metadata); err != nil {
		return Image{}, runtimeMetadata{}, err
	}
	if !pythonABIPattern.MatchString(metadata.ABI) || metadata.OS != "ubuntu" || metadata.Path != "/opt/datadog-agent/embedded/lib/"+metadata.ABI+"/site-packages" {
		return Image{}, runtimeMetadata{}, fmt.Errorf("runtime requires Ubuntu with known embedded Python layout")
	}
	return image, metadata, nil
}

// PreflightRuntimeImage rejects incompatible runtime images before invoking a builder.
func (a Adapter) PreflightRuntimeImage(ctx context.Context, reference string, target Target) (string, error) {
	image, _, err := a.inspectRuntimeImage(ctx, reference, target)
	return image.ID, err
}

// VerifyRuntime runs a Go check and a custom Python check with networking off.
// The full staged embedded tree overlays the baked-in absolute runtime prefix;
// no original checkout/runtime files are exposed to the container.
func (a Adapter) VerifyRuntime(ctx context.Context, r Result, reference string) (Result, error) {
	if err := r.Validate(r.Target); err != nil {
		return r, err
	}
	if r.Binary == nil {
		return r, fmt.Errorf("binary bundle required")
	}
	image, metadata, err := a.inspectRuntimeImage(ctx, reference, r.Target)
	if err != nil {
		return r, err
	}
	if _, err = os.Stat(filepath.Join(r.Binary.Runtime.Root, "lib", metadata.ABI)); err != nil {
		return r, fmt.Errorf("staged/runtime-image Python ABI mismatch: %w", err)
	}
	b := *r.Binary
	r.Binary = &b
	b.RuntimeImageID = image.ID
	b.PythonPath = metadata.Path
	b.PythonABI = metadata.ABI
	b.OSVersion = metadata.Version
	dir, err := os.MkdirTemp("", "e2ectl-runtime-")
	if err != nil {
		return r, err
	}
	defer os.RemoveAll(dir)
	for path, content := range map[string]string{
		"datadog.yaml":                      "api_key: 00000000000000000000000000000000\nhostname: e2ectl-runtime-probe\npython_version: 3\nconfd_path: /probe/conf.d\nadditional_checksd: /probe/checks.d\nremote_configuration:\n  enabled: false\n",
		"conf.d/e2ectl_runtime.d/conf.yaml": "init_config: {}\ninstances:\n  - {}\n",
		"checks.d/e2ectl_runtime.py":        fmt.Sprintf("from datadog_checks.base import AgentCheck\nimport ssl, sqlite3, sys\nif 'python' + '.'.join(map(str, sys.version_info[:2])) != %q:\n    raise RuntimeError('runtime-image Python ABI mismatch')\nclass E2ectlRuntime(AgentCheck):\n    def check(self, instance):\n        self.gauge('e2ectl.runtime.loaded', 1)\n", b.PythonABI),
		"conf.d/cpu.d/conf.yaml":            "init_config: {}\ninstances:\n  - {}\n",
	} {
		full := filepath.Join(dir, path)
		if err = os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			return r, err
		}
		if err = os.WriteFile(full, []byte(content), 0644); err != nil {
			return r, err
		}
	}
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	args := []string{"--rm", "--pull=never", "--network=none"}
	args = append(args, BinaryMountArgs(*r.Binary)...)
	// The check subcommand fetches Agent IPC artifacts. Let the real staged
	// Agent create them in this disposable private directory (no fabricated certs
	// and no user credentials). All senders are network-isolated throughout.
	script := `agent=/opt/datadog-agent/bin/agent/agent
 "$agent" run -c /probe/datadog.yaml >/probe/agent.log 2>&1 & pid=$!
 trap 'kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true' EXIT
 for i in $(seq 1 45); do
   test -s /probe/auth_token && test -s /probe/ipc_cert.pem && break
   kill -0 "$pid" 2>/dev/null || { cat /probe/agent.log; exit 1; }
   sleep 1
 done
 "$agent" check cpu -c /probe/datadog.yaml
 "$agent" check e2ectl_runtime -c /probe/datadog.yaml
 `
	args = append(args, "-v", dir+":/probe", "--entrypoint", "sh", image.ID, "-ec", script)
	out, err := a.runContainer(probeCtx, "", args)
	if err != nil {
		return r, fmt.Errorf("staged Agent runtime checks failed: %w", err)
	}
	if !strings.Contains(string(out), "e2ectl.runtime.loaded") || !strings.Contains(string(out), "system.cpu.") {
		return r, fmt.Errorf("Go/Python runtime checks did not emit verification metrics")
	}

	return r, nil
}
