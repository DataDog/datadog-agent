"""Datadog Agent wrapper around rules_go go_binary.

Injects standard version x_defs and run-path linker flags into every agent
binary so callers don't have to repeat them.  The x_defs at binary level
override the placeholder values set in //pkg/version:version (x_defs there
default to "0.0.0-dev").

Version string strategy: build_version/agent_version/agent_version_url_safe/base_branch/
milestone/agent_payload_version are all computed once by
//bazel/rules/variables:variables.bzl's compute_version_variables() (the same function
package_naming.bzl uses), so this file, package_naming.bzl, and the other packaging
rules never disagree on these values.
- The agent_version parameter, when passed, overrides both AgentVersion and AgentVersionURLSafe
  so the two stay in sync. Used by callers that compute their own version outside of
  PACKAGE_VERSION, e.g. host-profiler's nightly/dev-branch build.

x_defs values may reference any of the common variables via Python-style format
placeholders, e.g. {"some/pkg.appVersion": "{agent_version}"}; they are expanded with
.format(**subs) against the same substitution dict compute_version_variables() (plus the
resolved agent_version/agent_version_url_safe) produces. Available placeholders:
{build_version}, {agent_version}, {agent_version_url_safe}, {base_branch}, {milestone},
{agent_payload_version}.

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

load("@rules_go//go:def.bzl", "go_binary")
load("//bazel/rules/variables:variables.bzl", "compute_version_variables", "standard_to_url_safe")
load(
    "//tasks:build_tags.bzl",
    "COMMON_TAGS",
    "DARWIN_EXCLUDED_TAGS",
    "FIPS_TAGS",
    "LINUX_ONLY_TAGS",
    "WINDOWS_EXCLUDED_TAGS",
    "WINDOWS_INCLUDED_TAGS",
)

_REPO = "github.com/DataDog/datadog-agent"
_VERSION_PKG = _REPO + "/pkg/version"
_SETUP_PKG = _REPO + "/pkg/config/setup"

_RUN_PATH_RELEASE = "/opt/datadog-packages/run"
_RUN_PATH_DEV = "dev/lib"

def dd_agent_go_binary(
        name,
        gc_linkopts = None,
        gotags = None,
        exact_gotags = None,
        agent_version = None,
        **kwargs):
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
      agent_version: overrides pkg/version.AgentVersion and AgentVersionURLSafe (URL-safe
                     encoded) instead of deriving them from PACKAGE_VERSION/release.json.
      **kwargs: arguments to be forwarded to go_binary. x_defs values are expanded with
                .format() against the common substitution dict before being applied — see
                the module docstring for available placeholders.
    """
    # TODO: When --stamp support is in place, also inject:
    #   _VERSION_PKG + ".Commit": "{STABLE_GIT_COMMIT}",
    # The value must come from a stamp file produced by a git_info repository
    # rule (planned: bazel/repo/git_info.bzl).

    # Build two complete x_defs dicts — one per //:is_release branch.
    # string_dict attributes do not support per-value select(); the select()
    # must wrap the whole dict.
    common = compute_version_variables()
    if agent_version:
        agent_version_url_safe = standard_to_url_safe(agent_version)
    else:
        agent_version_url_safe = common["agent_version_url_safe"]
        agent_version = common["agent_version"]

    subs = dict(common)
    subs["agent_version"] = agent_version
    subs["agent_version_url_safe"] = agent_version_url_safe

    release_x_defs = {
        _VERSION_PKG + ".AgentPayloadVersion": common["agent_payload_version"],
        _VERSION_PKG + ".AgentVersion": agent_version,
        _VERSION_PKG + ".AgentVersionURLSafe": agent_version_url_safe,
        _SETUP_PKG + ".defaultRunPath": _RUN_PATH_RELEASE,
    }
    dev_x_defs = {
        _VERSION_PKG + ".AgentPayloadVersion": common["agent_payload_version"],
        _VERSION_PKG + ".AgentVersion": agent_version,
        _VERSION_PKG + ".AgentVersionURLSafe": agent_version_url_safe,
        _SETUP_PKG + ".defaultRunPath": _RUN_PATH_DEV,
    }

    existing_x_defs = kwargs.pop("x_defs", {})
    expanded_x_defs = {k: v.format(**subs) for k, v in existing_x_defs.items()}
    release_x_defs.update(expanded_x_defs)
    dev_x_defs.update(expanded_x_defs)

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
        # this might be select()'ed by platform. It is up to the user to sort it.
        kwargs["gotags"] = exact_gotags
    else:
        gotags = gotags or set()
        kwargs["gotags"] = select({
            "@platforms//os:macos": sorted((COMMON_TAGS | gotags) - LINUX_ONLY_TAGS - DARWIN_EXCLUDED_TAGS),
            "//packages/agent:linux_fips": sorted(COMMON_TAGS | gotags | FIPS_TAGS),
            "//packages/agent:windows_x86_64_fips": sorted((COMMON_TAGS | gotags | FIPS_TAGS | WINDOWS_INCLUDED_TAGS) - LINUX_ONLY_TAGS - WINDOWS_EXCLUDED_TAGS),
            "//:windows_x86_64": sorted((COMMON_TAGS | gotags | WINDOWS_INCLUDED_TAGS) - LINUX_ONLY_TAGS - WINDOWS_EXCLUDED_TAGS),
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
