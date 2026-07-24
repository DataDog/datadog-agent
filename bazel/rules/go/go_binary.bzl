"""Datadog Agent wrapper around rules_go go_binary.

Injects standard version x_defs and run-path linker flags into every agent
binary so callers don't have to repeat them.  The x_defs at binary level
override the placeholder values set in //pkg/version:version (x_defs there
default to "0.0.0-dev").

Version string strategy (mirrors package_naming.bzl):
- In CI: PACKAGE_VERSION env var is set by `dda inv agent.version --url-safe`,
  producing the URL-safe dotted form e.g. "7.81.0-devel.git.635.e3326d4.pipeline.102267660".
  AgentVersionURLSafe uses this directly; AgentVersion converts it back to standard
  SemVer form with '+' via _url_safe_to_standard().
- Locally: fall back to release_json current_milestone + "-localbuild" for both.

Run-path strategy, selected via //:linux_and_release and @platforms//os:linux:
- Linux + release (//:linux_and_release): /opt/datadog-packages/run
- Linux + dev    (@platforms//os:linux):  dev/lib
- Non-Linux      (//conditions:default):  no "-r" flag (ELF RPATH is Linux-specific)

The run path is injected two ways:
1. As a Go variable via x_defs (pkg/config/setup.defaultRunPath).
2. As a gc_linkopts "-r" flag — embeds the ELF RPATH (Linux only).

The Commit symbol is intentionally omitted: it requires git information that
must come from a future repository rule (bazel/repo/git_info.bzl) and should
only be set when Bazel is invoked with the --stamp flag.
"""

load("@agent_volatile//:env_vars.bzl", "env_vars")
load("@dd_release_json//:release_json.bzl", "release_json")
load("@rules_go//go:def.bzl", "go_binary")
load("//tasks:agent_payload_version.bzl", "AGENT_PAYLOAD_VERSION")
load("//tasks:build_tags.bzl", "COMMON_TAGS", "DARWIN_EXCLUDED_TAGS", "FIPS_TAGS", "LINUX_ONLY_TAGS", "WINDOWS_EXCLUDED_TAGS")

_REPO = "github.com/DataDog/datadog-agent"
_VERSION_PKG = _REPO + "/pkg/version"
_SETUP_PKG = _REPO + "/pkg/config/setup"

_RUN_PATH_RELEASE = "/opt/datadog-packages/run"
_RUN_PATH_DEV = "dev/lib"

def _url_safe_to_standard(url_safe):
    """Convert a URL-safe agent version string to the standard SemVer form.

    PACKAGE_VERSION is produced by `dda inv agent.version --url-safe`, which
    replaces the SemVer '+' build-metadata separator with '.'.  AgentVersion
    expects the standard form with '+'.

    Examples:
      "7.81.0-devel.git.635.e3326d4.pipeline.1" -> "7.81.0-devel+git.635.e3326d4.pipeline.1"
      "7.81.0-rc.1.git.635.e3326d4"             -> "7.81.0-rc.1+git.635.e3326d4"
      "7.81.0"                                   -> "7.81.0"  (clean release, no change)
    """
    idx = url_safe.find(".git.")
    if idx < 0:
        return url_safe
    return url_safe[:idx] + "+git." + url_safe[idx + 5:]

def _make_agent_version_url_safe():
    """Return the URL-safe agent version string.

    Uses PACKAGE_VERSION from the environment when available (CI), otherwise
    falls back to the current milestone from release.json with a "-localbuild"
    suffix, matching the convention in packages/rules/package_naming.bzl.
    """
    if env_vars.PACKAGE_VERSION:
        return env_vars.PACKAGE_VERSION
    return release_json.get("current_milestone") + "-localbuild"

def dd_agent_go_binary(name, gc_linkopts = None, gotags = None, exact_gotags = None, **kwargs):
    """Wrapper around go_binary that injects Datadog Agent version x_defs.

    Accepts all go_binary attributes.  x_defs and gc_linkopts are merged with
    the version/run-path/strip definitions; caller-supplied values take
    precedence over the defaults provided here.

    Defaults applied automatically (override by passing the attribute explicitly):
      cgo: True on Windows (required to link .syso resource files), False elsewhere.

    Args:
      name: target name
      gc_linkopts: Base set of link opts. rpath and stripping options are
                   automatically added to these.
                   On linux: add RPATH
                   On release builds: add -s -w (strip symbol table and DWARF)
      gotags: Base set of gotags for this binary. COMMON tags are added, and
              per-platform adjustments are made.
      exact_gotags: Like gotags, but if this is specified, no other tag sets are added.
      **kwargs: arguments to be forwarded to go_binary
    """
    # TODO: When --stamp support is in place, also inject:
    #   _VERSION_PKG + ".Commit": "{STABLE_GIT_COMMIT}",
    # The value must come from a stamp file produced by a git_info repository
    # rule (planned: bazel/repo/git_info.bzl).

    # Build two complete x_defs dicts — one per //:is_release branch.
    # string_dict attributes do not support per-value select(); the select()
    # must wrap the whole dict.
    agent_version_url_safe = _make_agent_version_url_safe()
    release_x_defs = {
        _VERSION_PKG + ".AgentPayloadVersion": AGENT_PAYLOAD_VERSION,
        _VERSION_PKG + ".AgentVersion": _url_safe_to_standard(agent_version_url_safe),
        _VERSION_PKG + ".AgentVersionURLSafe": agent_version_url_safe,
        _SETUP_PKG + ".defaultRunPath": _RUN_PATH_RELEASE,
    }
    dev_x_defs = {
        _VERSION_PKG + ".AgentPayloadVersion": AGENT_PAYLOAD_VERSION,
        _VERSION_PKG + ".AgentVersion": _url_safe_to_standard(agent_version_url_safe),
        _VERSION_PKG + ".AgentVersionURLSafe": agent_version_url_safe,
        _SETUP_PKG + ".defaultRunPath": _RUN_PATH_DEV,
    }
    existing_x_defs = kwargs.pop("x_defs", {})
    release_x_defs.update(existing_x_defs)
    dev_x_defs.update(existing_x_defs)

    # cgo must be enabled on Windows to link the .syso resource file produced
    # by win_resource().  Callers that need additional conditions (e.g. FIPS)
    # should pass an explicit cgo = select({...}) which replaces this default.
    if "cgo" not in kwargs:
        kwargs["cgo"] = select({
            "@platforms//os:windows": True,
            "//conditions:default": False,
        })

    # "-r <path>" embeds the ELF RPATH so shared libraries under the run path
    # are found at runtime.  This flag is Linux-specific; non-Linux targets get
    # an empty list.
    # //:linux_and_release (Linux + release=True) is more specific than the plain
    # @platforms//os:linux constraint, so Bazel's ambiguity resolution picks it
    # first when both conditions hold.
    run_path_linkopts = select({
        "//:linux_and_release": ["-r", _RUN_PATH_RELEASE],
        "@platforms//os:linux": ["-r", _RUN_PATH_DEV],
        "//conditions:default": [],
    })

    # Strip the symbol table and DWARF debug info in release builds to reduce
    # binary size.  Dev builds keep symbols for debugger and profiler use.
    strip_linkopts = select({
        "//:is_release": ["-s", "-w"],
        "//conditions:default": [],
    })

    if exact_gotags:
        kwargs["gotags"] = exact_gotags
    else:
        gotags = gotags or set()
        kwargs["gotags"] = select({
            "//packages/agent:linux_fips": sorted(COMMON_TAGS | gotags | FIPS_TAGS),
            "//packages/agent:macos_default": sorted((COMMON_TAGS | gotags) - LINUX_ONLY_TAGS - DARWIN_EXCLUDED_TAGS),
            "//packages/agent:macos_fips": sorted((COMMON_TAGS | gotags | FIPS_TAGS) - LINUX_ONLY_TAGS - DARWIN_EXCLUDED_TAGS),
            "//packages/agent:windows_default": sorted((COMMON_TAGS | gotags) - LINUX_ONLY_TAGS - WINDOWS_EXCLUDED_TAGS),
            "//packages/agent:windows_fips": sorted((COMMON_TAGS | gotags | FIPS_TAGS) - LINUX_ONLY_TAGS - WINDOWS_EXCLUDED_TAGS),
            "//conditions:default": sorted(COMMON_TAGS | gotags),
        })

    go_binary(
        name = name,
        gc_linkopts = (gc_linkopts or []) + run_path_linkopts + strip_linkopts,
        x_defs = select({
            "//:is_release": release_x_defs,
            "//conditions:default": dev_x_defs,
        }),
        **kwargs
    )
