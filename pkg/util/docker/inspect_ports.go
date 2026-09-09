// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path"
	"sort"
	"strings"

	dcontainer "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// isInvalidPortKeyError reports whether err looks like moby rejecting a port map
// key: network.Port.UnmarshalText returns "invalid port '<key>': ...", which
// aborts the whole ContainerInspect decode. It matches any unparseable key, not
// just ranges, since the sanitizer drops those too and still recovers the rest
// of the container. The single quote avoids matching unrelated errors such as
// net/url's `invalid port "x"`.
//
// This is only a cheap pre-filter to avoid refetching on unrelated failures;
// correctness does not depend on it. recoverInspect independently verifies the
// payload actually contained a fixable key, so a future rewording of moby's
// message would merely stop the recovery attempt (the pre-fix behaviour) rather
// than produce a wrong result. See CONS-8441.
func isInvalidPortKeyError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "invalid port '")
}

// recoverInspect attempts to recover a container inspect that moby failed to
// decode, by refetching the raw payload, expanding any port-range keys the
// strict decoder rejects (CONS-8441), and re-decoding. It does not trust the
// error text for correctness: it reports ok=false — so the caller surfaces the
// original error — when the payload held no range to expand, the re-decode still
// failed, or the response is not the container that was requested.
func (d *DockerUtil) recoverInspect(ctx context.Context, id string, withSize bool) (dcontainer.InspectResponse, bool) {
	var c dcontainer.InspectResponse

	// A done context cannot produce a successful refetch, so don't add daemon
	// load for it: the caller surfaces the original (timeout/cancel) error.
	if ctx.Err() != nil {
		return c, false
	}

	id = strings.TrimSpace(id) // moby's client trims too; keep the paths identical
	raw, err := d.rawContainerInspect(ctx, id, withSize)
	if err != nil {
		return c, false
	}
	sanitized, changed := sanitizeInspectPortRanges(raw)
	if !changed {
		return c, false // no port range present; not something we can fix
	}
	if err := json.Unmarshal(sanitized, &c); err != nil {
		return c, false
	}
	// Callers may pass a name or a short ID, and the container could have been
	// replaced between the failed inspect and this refetch, so make sure we are
	// about to return the container that was actually asked for.
	if !matchesContainer(c, id) {
		return c, false
	}
	return c, true
}

// matchesContainer reports whether the inspect response identifies the container
// referred to by ref, which may be a full ID, an ID prefix, or a name.
func matchesContainer(c dcontainer.InspectResponse, ref string) bool {
	if c.ID == "" || ref == "" {
		return false
	}
	if strings.HasPrefix(c.ID, ref) {
		return true
	}
	// Inspect reports names with a leading slash ("/my-container").
	return strings.EqualFold(strings.TrimPrefix(c.Name, "/"), strings.TrimPrefix(ref, "/"))
}

// rawContainerInspect issues GET /containers/<id>/json to the daemon through the
// moby client's dialer and returns the raw, undecoded response body. moby's
// ContainerInspect discards the raw bytes when its strict decode fails, so the
// fallback must refetch them here to sanitize and re-decode. This mirrors the
// raw-fetch in safe_info.go's tolerantInfo (the analogous /info strict-decode
// workaround), kept separate to avoid coupling the two.
func (d *DockerUtil) rawContainerInspect(ctx context.Context, id string, withSize bool) ([]byte, error) {
	httpClient := &http.Client{
		Transport: &http.Transport{
			// One-shot client: no benefit from keep-alive, and DisableKeepAlives
			// ensures the dialed connection is closed when the response body
			// is, so the unreferenced transport does not retain FDs.
			DisableKeepAlives: true,
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return d.cli.Dialer()(ctx)
			},
		},
	}

	// The dialer takes care of reaching the daemon (unix, npipe, tcp, tcp+tls).
	// For TCP daemons, preserve the configured host and base path so reverse
	// proxies relying on Host-header or path routing reach the same backend
	// as the SDK does. For unix/npipe, the SDK uses DummyHost — match it.
	// Use the "http" scheme even for tls-fronted daemons: the dialer returns
	// an already-TLS-encrypted connection, and the http transport writes plain
	// HTTP bytes over it.
	reqHost := client.DummyHost
	basePath := ""
	if hostURL, err := client.ParseHostURL(d.cli.DaemonHost()); err == nil {
		if hostURL.Scheme == "tcp" {
			reqHost = hostURL.Host
		}
		basePath = hostURL.Path
	}
	// Match the SDK's path construction so reverse proxies routing on
	// /vX.Y/containers/... don't reject the fallback (see moby client.getAPIPath).
	apiPath := "/containers/" + id + "/json"
	if v := d.cli.ClientVersion(); v != "" {
		apiPath = "/v" + strings.TrimPrefix(v, "v") + apiPath
	}
	url := "http://" + reqHost + path.Join("/", basePath, apiPath)
	if withSize {
		// Mirror the SDK's Size option, or the recovered container would come
		// back with its size fields unset.
		url += "?size=1"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, apiPath)
	}

	return io.ReadAll(resp.Body)
}

// sanitizeInspectPortRanges rewrites port-range keys ("1061-1070" -> "1061/tcp"
// … "1070/tcp") in Config.ExposedPorts of a raw inspect payload so the strict
// moby decoder can parse it, returning the payload and whether it changed.
// ExposedPorts is the only inspect field observed to carry a baked-in range.
func sanitizeInspectPortRanges(raw []byte) ([]byte, bool) {
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) != nil {
		return raw, false
	}
	var config map[string]json.RawMessage
	if json.Unmarshal(top["Config"], &config) != nil {
		return raw, false // Config absent, null, or not an object
	}
	ports, changed := expandPortKeys(config["ExposedPorts"])
	if !changed {
		return raw, false
	}
	config["ExposedPorts"] = ports

	cfg, err := json.Marshal(config)
	if err != nil {
		return raw, false
	}
	top["Config"] = cfg
	out, err := json.Marshal(top)
	if err != nil {
		return raw, false
	}
	return out, true
}

// maxExpandedPorts bounds how many ports the range keys of one object may expand
// into in total. Pathological input ("1-65535", or many wide ranges) would
// otherwise inflate the payload and push a huge number of ports into
// workloadmeta for a single container. The budget is shared across the whole
// object, not per key, so a payload full of ranges cannot multiply past it.
// Ranges that do not fit are dropped, which still recovers the container.
const maxExpandedPorts = 1024

// expandPortKeys rewrites one port-keyed object. Valid single ports are kept
// (normalized so a range never overwrites an equivalent explicit key); ranges
// are expanded within the maxExpandedPorts budget; keys that are neither are
// dropped.
func expandPortKeys(raw json.RawMessage) (json.RawMessage, bool) {
	var ports map[string]json.RawMessage
	if json.Unmarshal(raw, &ports) != nil {
		return nil, false
	}

	out := make(map[string]json.RawMessage, len(ports))
	ranges := make([]string, 0, len(ports))
	changed := false
	for k, v := range ports {
		if p, err := network.ParsePort(k); err == nil {
			out[p.String()] = v // valid single port, kept normalized
			continue
		}
		if _, err := network.ParsePortRange(k); err != nil {
			log.Debugf("dropping unparseable docker port key %q: %s", k, err)
			changed = true
			continue
		}
		ranges = append(ranges, k)
	}

	// Expand in sorted order: map iteration is randomised, so without this the
	// set of ranges that fits the budget could differ between two inspects of
	// the same container, making its reported ports flap.
	sort.Strings(ranges)
	budget := maxExpandedPorts
	for _, k := range ranges {
		changed = true
		pr, err := network.ParsePortRange(k)
		if err != nil {
			continue // already validated above
		}
		width := int(pr.End()) - int(pr.Start()) + 1
		if width > budget {
			// Debug, not Warn: this runs per container event on an uncached path,
			// so a single offending container would otherwise spam warnings.
			log.Debugf("dropping docker port range %q: %d ports exceeds the remaining budget of %d",
				k, width, budget)
			continue
		}
		budget -= width
		v := ports[k]
		for p := range pr.All() {
			if _, exists := out[p.String()]; !exists {
				out[p.String()] = v
			}
		}
	}
	if !changed {
		return nil, false
	}
	if b, err := json.Marshal(out); err == nil {
		return b, true
	}
	return nil, false
}
