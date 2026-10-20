#!/opt/datadog-agent/embedded/bin/python3
# Populates a copy of datadog.yaml.example from DD_* environment variables.
# Invoked by the "config" installp script; see that script for why this is a
# separate file (AIX sed has no -i, and interpolating env values into an
# eval'd script is a code-injection risk) and why edits are staged rather
# than applied to the real datadog.yaml directly (permissions).
import os
import sys

import yaml


def env(*names, allow_empty=False):
    # allow_empty=True matches os.LookupEnv semantics (used by LoadProxyFromEnv
    # in pkg/config/setup/config.go): an explicit DD_PROXY_HTTP="" must
    # suppress a lower-priority HTTP_PROXY rather than fall through to it.
    for name in names:
        if allow_empty:
            if name in os.environ:
                return os.environ[name]
        elif os.environ.get(name):
            return os.environ[name]
    return None


def split_list(value):
    # DD_TAGS and DD_PROXY_NO_PROXY both accept space or comma as separators.
    return [v for v in value.replace(",", " ").split() if v]


path = sys.argv[1]
with open(path) as f:
    lines = f.readlines()

# api_key is the only field with a real, uncommented line in the template;
# the rest below ship commented out. Duplicate YAML keys resolve last-wins,
# so it must be edited in place -- appending it instead would get
# overridden by the template's own empty "api_key:" line.
api_key = env("DD_API_KEY")
for i, line in enumerate(lines):
    if line.startswith("api_key:"):
        value_yaml = yaml.safe_dump(api_key, default_flow_style=True).splitlines()[0]
        lines[i] = "api_key: " + value_yaml + "\n"
        break

extra = {}
if site := env("DD_SITE"):
    extra["site"] = site
if hostname := env("DD_HOSTNAME"):
    extra["hostname"] = hostname
if environment := env("DD_ENV"):
    extra["env"] = environment
if infrastructure_mode := env("DD_INFRASTRUCTURE_MODE"):
    extra["infrastructure_mode"] = infrastructure_mode
if tags := env("DD_TAGS"):
    extra["tags"] = split_list(tags)

# DD_PROXY_* takes precedence over the generic HTTP_PROXY/HTTPS_PROXY/NO_PROXY
# vars, matching the Agent's own env var resolution (LoadProxyFromEnv).
proxy = {}
if http_proxy := env("DD_PROXY_HTTP", "HTTP_PROXY", "http_proxy", allow_empty=True):
    proxy["http"] = http_proxy
if https_proxy := env("DD_PROXY_HTTPS", "HTTPS_PROXY", "https_proxy", allow_empty=True):
    proxy["https"] = https_proxy
if no_proxy := env("DD_PROXY_NO_PROXY", "NO_PROXY", "no_proxy", allow_empty=True):
    # DD_PROXY_NO_PROXY accepts space or comma; generic NO_PROXY is comma-only.
    if "DD_PROXY_NO_PROXY" in os.environ:
        proxy["no_proxy"] = split_list(no_proxy)
    else:
        proxy["no_proxy"] = [p for p in no_proxy.split(",") if p]
if proxy:
    extra["proxy"] = proxy

with open(path, "w") as f:
    if extra:
        f.write(yaml.safe_dump(extra, default_flow_style=False) + "\n")
    f.writelines(lines)
