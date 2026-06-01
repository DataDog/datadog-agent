"""Repository rule for downloading tools from DotSlash manifests."""

_SHEBANG = "#!/usr/bin/env dotslash"

_ARCH_NAMES = {
    "amd64": "x86_64",
    "arm64": "aarch64",
    "aarch64": "aarch64",
    "x86_64": "x86_64",
}

_ARCHIVE_FORMATS = {
    "tar.gz": True,
    "tar.xz": True,
    "tar.zst": True,
    "zip": True,
}

def _fail(message):
    fail("invalid DotSlash tool manifest: " + message)

def _parse_manifest(contents):
    if not contents.startswith(_SHEBANG + "\n") and not contents.startswith(_SHEBANG + "\r\n"):
        _fail("manifest must start with '{}'".format(_SHEBANG))

    parts = contents.split("\n", 1)
    if len(parts) != 2:
        _fail("manifest is missing its JSON payload")

    manifest = json.decode(parts[1])
    if type(manifest) != "dict":
        _fail("JSON payload must be an object")
    if type(manifest.get("name")) != "string":
        _fail("'name' must be a string")
    if type(manifest.get("platforms")) != "dict":
        _fail("'platforms' must be an object")

    return manifest

def _dotslash_platform(rctx):
    os_name = None
    if "linux" in rctx.os.name:
        os_name = "linux"
    elif "mac os x" in rctx.os.name:
        os_name = "macos"
    elif "windows" in rctx.os.name:
        os_name = "windows"
    if os_name == None:
        _fail("unsupported host OS '{}'".format(rctx.os.name))

    arch_name = _ARCH_NAMES.get(rctx.os.arch)
    if arch_name == None:
        _fail("unsupported host architecture '{}'".format(rctx.os.arch))

    return "{}-{}".format(os_name, arch_name)

def _validate_path(path):
    if type(path) != "string" or not path:
        _fail("'path' must be a non-empty string")
    if path.startswith("/") or path.endswith("/"):
        _fail("'path' must be a normalized relative UNIX path")
    if "\\" in path or ":" in path:
        _fail("'path' must use normalized relative UNIX path syntax")

    components = path.split("/")
    for component in components:
        if component in (".", "..", ""):
            _fail("'path' must not contain '.', '..', or empty path components")

def _validate_entry(manifest, platform):
    entry = manifest["platforms"].get(platform)
    if entry == None:
        _fail("'{}' does not define platform '{}'".format(manifest["name"], platform))
    if type(entry) != "dict":
        _fail("platform '{}' must be an object".format(platform))

    if type(entry.get("size")) != "int" or entry["size"] < 0:
        _fail("'size' for platform '{}' must be a non-negative integer".format(platform))
    if entry.get("hash") != "sha256":
        _fail("platform '{}' must use sha256".format(platform))
    if type(entry.get("digest")) != "string" or not entry["digest"]:
        _fail("'digest' for platform '{}' must be a non-empty string".format(platform))
    entry_format = entry.get("format")
    if entry_format != None and not _ARCHIVE_FORMATS.get(entry_format):
        _fail("platform '{}' uses unsupported format '{}'".format(platform, entry_format))
    if entry.get("providers_order") != None:
        _fail("phase 1 requires deterministic ordered providers")

    _validate_path(entry.get("path"))

    providers = entry.get("providers")
    if type(providers) != "list" or len(providers) == 0:
        _fail("'providers' for platform '{}' must be a non-empty list".format(platform))

    urls = []
    for provider in providers:
        if type(provider) != "dict":
            _fail("provider entries for platform '{}' must be objects".format(platform))
        provider_type = provider.get("type", "http")
        if provider_type != "http":
            _fail("phase 1 supports only HTTP providers")
        url = provider.get("url")
        if type(url) != "string" or not url:
            _fail("HTTP providers must define a non-empty 'url'")
        urls.append(url)

    return struct(
        digest = entry["digest"],
        format = entry_format,
        path = entry["path"],
        urls = urls,
    )

def _build_file_content(tool_name, artifact_path):
    alias = ""
    if tool_name != "tool":
        alias = """
alias(
    name = "{tool_name}",
    actual = ":tool",
)
""".format(tool_name = tool_name)

    return """\
load("@bazel_skylib//rules:native_binary.bzl", "native_binary")

package(default_visibility = ["//visibility:public"])

exports_files(["{artifact_path}"])

native_binary(
    name = "tool",
    out = "{tool_name}.exe",
    src = "{artifact_path}",
)
{alias}""".format(
        alias = alias,
        artifact_path = artifact_path,
        tool_name = tool_name,
    )

def _dotslash_tool_impl(rctx):
    manifest = _parse_manifest(rctx.read(rctx.attr.manifest))
    platform = _dotslash_platform(rctx)
    entry = _validate_entry(manifest, platform)

    artifact_path = "download/" + entry.path
    tool_name = manifest["name"]
    canonical_id = "dotslash:{}:{}:{}".format(manifest["name"], platform, entry.digest)
    if entry.format:
        rctx.download_and_extract(
            url = entry.urls,
            output = "download",
            sha256 = entry.digest,
            type = entry.format,
            canonical_id = canonical_id,
        )
    else:
        rctx.download(
            url = entry.urls,
            output = artifact_path,
            executable = True,
            sha256 = entry.digest,
            canonical_id = canonical_id,
        )

    rctx.file("MODULE.bazel", 'module(name = "{}")\n'.format(rctx.original_name))
    rctx.file("BUILD.bazel", _build_file_content(tool_name, artifact_path))

    return rctx.repo_metadata(reproducible = True)

dotslash_tool = repository_rule(
    implementation = _dotslash_tool_impl,
    attrs = {
        "manifest": attr.label(
            allow_single_file = True,
            doc = "DotSlash manifest file to read.",
            mandatory = True,
        ),
    },
    doc = "Download a single-file executable from a strict DotSlash manifest.",
)

def _hub_build_file_content(tools):
    aliases = []
    for name, actual in sorted(tools.items()):
        aliases.append("""\
alias(
    name = "{name}",
    actual = "{actual}",
)
""".format(
            actual = actual,
            name = name,
        ))

    return """\
package(default_visibility = ["//visibility:public"])

{aliases}""".format(aliases = "\n".join(aliases))

def _dotslash_hub_impl(rctx):
    rctx.file("MODULE.bazel", 'module(name = "{}")\n'.format(rctx.original_name))
    rctx.file("BUILD.bazel", _hub_build_file_content(rctx.attr.tools))

    return rctx.repo_metadata(reproducible = True)

dotslash_hub = repository_rule(
    implementation = _dotslash_hub_impl,
    attrs = {
        "tools": attr.string_dict(
            doc = "Mapping from hub target name to the generated DotSlash tool label.",
            mandatory = True,
        ),
    },
    doc = "Expose DotSlash-managed tools through a single repository.",
)

_tools = tag_class(
    attrs = {
        "manifests": attr.label_list(
            allow_files = True,
            doc = "DotSlash manifests to expose through @dotslash.",
            mandatory = True,
        ),
    },
)

def _repo_name(tool_name):
    return "dotslash_" + tool_name.replace("-", "_")

def _dotslash_extension_impl(mctx):
    tools = {}
    for mod in mctx.modules:
        for tag in mod.tags.tools:
            for manifest_label in tag.manifests:
                manifest = _parse_manifest(mctx.read(manifest_label))
                tool_name = manifest["name"]
                if tool_name in tools:
                    fail("duplicate DotSlash tool manifest name: " + tool_name)

                repo_name = _repo_name(tool_name)
                dotslash_tool(
                    name = repo_name,
                    manifest = manifest_label,
                )
                tools[tool_name] = "@{}//:tool".format(repo_name)

    dotslash_hub(
        name = "dotslash",
        tools = tools,
    )

dotslash = module_extension(
    implementation = _dotslash_extension_impl,
    tag_classes = {"tools": _tools},
)
