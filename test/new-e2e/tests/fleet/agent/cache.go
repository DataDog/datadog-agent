// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"fmt"
	"os"
	"strings"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/pipeline"
)

// cacheMirrorPort is the TCP port on which the VM-local package mirror listens.
// Hard-coded; tests run on dedicated VMs so port collisions are not a concern.
const cacheMirrorPort = 7071

// WarmPackageCache pre-fetches every agent package the fleet suite will install
// into a VM-local directory and starts a localhost HTTP server that serves
// them. Subsequent calls to Install() will rewrite the install script to pull
// from this mirror instead of S3, removing the per-install download cost.
//
// Requirements:
//   - E2E_PIPELINE_ID must be set (testing packages). Stable/staging installs
//     do not benefit from caching and are skipped.
//   - The VM must be able to reach s3.amazonaws.com at suite setup time (one
//     sync); afterwards every install is served locally.
//
// Fails loudly. Silent fallback would mask regressions that erase the speedup.
func (a *Agent) WarmPackageCache() error {
	pipelineID := os.Getenv("E2E_PIPELINE_ID")
	if pipelineID == "" {
		return nil
	}
	switch a.host.RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		return a.warmLinuxCache(pipelineID)
	case e2eos.WindowsFamily:
		return a.warmWindowsCache(pipelineID)
	default:
		return nil
	}
}

func (a *Agent) warmLinuxCache(pipelineID string) error {
	// Package manager family drives which S3 subtree we need. The install
	// script builds URLs like https://${apt_url}/... and https://${yum_url}/...
	// — we mirror just the subtree this VM will actually read.
	aptPrefix := fmt.Sprintf("datadog-agent/pipeline-%s-a7", pipelineID)
	yumPrefix := fmt.Sprintf("testing/pipeline-%s-a7/7", pipelineID)
	suseYumPrefix := fmt.Sprintf("suse/testing/pipeline-%s-a7/7", pipelineID)

	// Ensure tools we rely on are present. The DD CI images ship most of
	// these but an explicit best-effort install keeps this working on fresh
	// VMs. `unzip` is the one that's most commonly missing; without it the
	// AWS CLI installer fails silently and every `aws s3 sync` errors with
	// "command not found".
	bootstrap := `
set +e
if ! command -v python3 >/dev/null; then
  sudo apt-get install -y python3 || sudo yum install -y python3 || sudo zypper install -y python3
fi
if ! command -v unzip >/dev/null; then
  sudo apt-get install -y unzip || sudo yum install -y unzip || sudo zypper install -y unzip
fi
if ! command -v aws >/dev/null && command -v unzip >/dev/null; then
  tmp=$(mktemp -d)
  arch=$(uname -m); [ "$arch" = "aarch64" ] && awsarch=aarch64 || awsarch=x86_64
  if curl -sSLo "$tmp/awscli.zip" "https://awscli.amazonaws.com/awscli-exe-linux-${awsarch}.zip"; then
    (cd "$tmp" && unzip -q awscli.zip && sudo ./aws/install)
  fi
fi
sudo mkdir -p /opt/dd-pkg-cache
sudo chmod 755 /opt/dd-pkg-cache
# Report final state so a failed warm-up logs something useful.
command -v aws >/dev/null && echo "aws: $(aws --version 2>&1 | head -1)" >&2 || echo "aws: MISSING" >&2
command -v python3 >/dev/null && echo "python3: $(python3 --version 2>&1)" >&2 || echo "python3: MISSING" >&2
true
`
	if _, err := a.host.RemoteHost.Execute(bootstrap); err != nil {
		return fmt.Errorf("cache bootstrap: %w", err)
	}

	sync := fmt.Sprintf(`
set -eux
# The testing S3 buckets are public-read; anonymous listing works via --no-sign-request.
sudo aws --no-sign-request s3 sync s3://apttesting.datad0g.com/%s/ /opt/dd-pkg-cache/apttesting.datad0g.com/%s/ --only-show-errors
sudo aws --no-sign-request s3 sync s3://yumtesting.datad0g.com/%s/ /opt/dd-pkg-cache/yumtesting.datad0g.com/%s/ --only-show-errors
sudo aws --no-sign-request s3 sync s3://yumtesting.datad0g.com/%s/ /opt/dd-pkg-cache/yumtesting.datad0g.com/%s/ --only-show-errors
`, aptPrefix, aptPrefix, yumPrefix, yumPrefix, suseYumPrefix, suseYumPrefix)
	if _, err := a.host.RemoteHost.Execute(sync); err != nil {
		return fmt.Errorf("cache sync: %w", err)
	}

	// Serve the cache via a transient systemd service. Cleaner than nohup+
	// disown over SSH: systemd handles stdio, restarts, and teardown, and
	// the service survives the SSH session that created it.
	start := fmt.Sprintf(`
set -eux
sudo systemctl stop dd-pkg-cache.service 2>/dev/null || true
sudo systemd-run --unit=dd-pkg-cache --working-directory=/opt/dd-pkg-cache /usr/bin/env python3 -m http.server %d --bind 127.0.0.1
for i in 1 2 3 4 5 6 7 8 9 10; do
  if curl -sSf -o /dev/null "http://127.0.0.1:%d/apttesting.datad0g.com/%s/"; then exit 0; fi
  sleep 1
done
echo "cache HTTP server did not come up" >&2
sudo journalctl -u dd-pkg-cache --no-pager -n 50 >&2 || true
exit 1
`, cacheMirrorPort, cacheMirrorPort, aptPrefix)
	if _, err := a.host.RemoteHost.Execute(start); err != nil {
		return fmt.Errorf("cache http server: %w", err)
	}
	a.cacheMirrorHost = fmt.Sprintf("127.0.0.1:%d", cacheMirrorPort)
	return nil
}

func (a *Agent) warmWindowsCache(pipelineID string) error {
	artifactURL, err := pipeline.GetPipelineArtifact(pipelineID, pipeline.AgentS3BucketTesting, pipeline.DefaultMajorVersion, func(artifact string) bool {
		return strings.Contains(artifact, "datadog-installer") && strings.HasSuffix(artifact, ".exe")
	})
	if err != nil {
		return fmt.Errorf("pipeline artifact lookup: %w", err)
	}
	// Derive the filename the install script expects.
	parts := strings.Split(artifactURL, "/")
	filename := parts[len(parts)-1]

	download := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
New-Item -ItemType Directory -Force -Path C:\dd-pkg-cache | Out-Null
[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
(New-Object System.Net.WebClient).DownloadFile('%s', 'C:\dd-pkg-cache\%s')
`, artifactURL, filename)
	if _, err := a.host.RemoteHost.Execute(download); err != nil {
		return fmt.Errorf("windows installer download: %w", err)
	}

	// Serve the cache directory. PowerShell's HttpListener is native, no
	// Python dependency required.
	startServer := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
# Kill any previous cache server (harmless if none).
Get-CimInstance Win32_Process -Filter "Name='powershell.exe'" | Where-Object { $_.CommandLine -match 'dd-pkg-cache-server' } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
$script = @'
# dd-pkg-cache-server
$listener = New-Object System.Net.HttpListener
$listener.Prefixes.Add('http://127.0.0.1:%d/')
$listener.Start()
while ($listener.IsListening) {
  $ctx = $listener.GetContext()
  $path = 'C:\dd-pkg-cache' + $ctx.Request.Url.LocalPath.Replace('/', '\')
  if (Test-Path $path -PathType Leaf) {
    $bytes = [System.IO.File]::ReadAllBytes($path)
    $ctx.Response.ContentLength64 = $bytes.Length
    $ctx.Response.OutputStream.Write($bytes, 0, $bytes.Length)
  } else {
    $ctx.Response.StatusCode = 404
  }
  $ctx.Response.Close()
}
'@
$encoded = [Convert]::ToBase64String([System.Text.Encoding]::Unicode.GetBytes($script))
Start-Process -WindowStyle Hidden powershell -ArgumentList '-NoProfile','-EncodedCommand',$encoded
# Wait until up.
for ($i = 0; $i -lt 10; $i++) {
  try { Invoke-WebRequest -UseBasicParsing "http://127.0.0.1:%d/%s" -Method Head | Out-Null; exit 0 } catch { Start-Sleep -Seconds 1 }
}
Write-Error "cache HTTP server did not come up"
exit 1
`, cacheMirrorPort, cacheMirrorPort, filename)
	if _, err := a.host.RemoteHost.Execute(startServer); err != nil {
		return fmt.Errorf("windows cache http server: %w", err)
	}
	a.cacheMirrorHost = fmt.Sprintf("127.0.0.1:%d", cacheMirrorPort)
	a.cacheWindowsInstaller = filename
	return nil
}
