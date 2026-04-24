// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"fmt"
	"os"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
)

// cacheMirrorPort is the TCP port on which the VM-local package mirror listens.
// Hard-coded; tests run on dedicated VMs so port collisions are not a concern.
const cacheMirrorPort = 7071

// WarmPackageCache pre-fetches every agent package the fleet suite will install
// into a VM-local directory and starts a localhost HTTP server that serves
// them. Subsequent Install() calls rewrite the install script to pull from
// this mirror instead of S3, removing the per-install download cost.
//
// Requirements:
//   - E2E_PIPELINE_ID must be set (testing packages). Stable/staging installs
//     do not benefit and are skipped.
//   - python3 must be present on the VM (true on every DD CI Linux image).
//   - The VM reaches s3.amazonaws.com at suite setup time (one sync);
//     afterwards every install is served locally.
//
// Only Linux is supported. Windows tests use the uncached S3 path until a
// suitable Windows HTTP server implementation is available.
func (a *Agent) WarmPackageCache() error {
	pipelineID := os.Getenv("E2E_PIPELINE_ID")
	if pipelineID == "" {
		return nil
	}
	if a.host.RemoteHost.OSFamily != e2eos.LinuxFamily {
		return nil
	}
	return a.warmLinuxCache(pipelineID)
}

func (a *Agent) warmLinuxCache(pipelineID string) error {
	aptPrefix := fmt.Sprintf("datadog-agent/pipeline-%s-a7", pipelineID)
	yumPrefix := fmt.Sprintf("testing/pipeline-%s-a7/7", pipelineID)
	suseYumPrefix := fmt.Sprintf("suse/testing/pipeline-%s-a7/7", pipelineID)

	// Python3 is the only dependency, and it's present on every CI image we
	// run on. Doing the sync with Python's urllib (no AWS CLI, no unzip)
	// sidesteps the fragile "install awscli via curl+unzip" dance and keeps
	// the warm-up dependency surface tiny.
	//
	// The script is written to /tmp via a quoted heredoc so the Python body
	// isn't subject to shell expansion.
	setup := fmt.Sprintf(`
set +e
if ! command -v python3 >/dev/null; then
  sudo apt-get install -y python3 || sudo yum install -y python3 || sudo zypper install -y python3
fi
if ! command -v python3 >/dev/null; then
  echo "python3 missing, cannot warm cache" >&2
  exit 1
fi
sudo mkdir -p /opt/dd-pkg-cache
sudo chmod 1777 /opt/dd-pkg-cache
cat > /tmp/dd-pkg-cache-sync.py <<'PYEOF'
import sys, os, urllib.request, urllib.parse, xml.etree.ElementTree as ET
NS = "{http://s3.amazonaws.com/doc/2006-03-01/}"
bucket, prefix, dest = sys.argv[1], sys.argv[2], sys.argv[3]
cont = None
while True:
    qs = "list-type=2&prefix=" + urllib.parse.quote(prefix)
    if cont:
        qs += "&continuation-token=" + urllib.parse.quote(cont)
    with urllib.request.urlopen("https://s3.amazonaws.com/" + bucket + "/?" + qs) as r:
        root = ET.parse(r).getroot()
    for c in root.findall(NS + "Contents"):
        key = c.find(NS + "Key").text
        if key.endswith("/"):
            continue
        local = os.path.join(dest, key)
        os.makedirs(os.path.dirname(local), exist_ok=True)
        if os.path.exists(local) and os.path.getsize(local) == int(c.find(NS + "Size").text):
            continue
        urllib.request.urlretrieve("https://s3.amazonaws.com/" + bucket + "/" + urllib.parse.quote(key), local)
    if (root.find(NS + "IsTruncated").text or "false") != "true":
        break
    cont = root.find(NS + "NextContinuationToken").text
PYEOF
set -e
python3 /tmp/dd-pkg-cache-sync.py apttesting.datad0g.com %q /opt/dd-pkg-cache/apttesting.datad0g.com
python3 /tmp/dd-pkg-cache-sync.py yumtesting.datad0g.com %q /opt/dd-pkg-cache/yumtesting.datad0g.com
python3 /tmp/dd-pkg-cache-sync.py yumtesting.datad0g.com %q /opt/dd-pkg-cache/yumtesting.datad0g.com
`, aptPrefix, yumPrefix, suseYumPrefix)
	if _, err := a.host.RemoteHost.Execute(setup); err != nil {
		return fmt.Errorf("cache sync: %w", err)
	}

	// Serve via a transient systemd service. Cleaner than nohup+disown over
	// SSH: systemd handles stdio/teardown and the service survives the
	// session that created it.
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
