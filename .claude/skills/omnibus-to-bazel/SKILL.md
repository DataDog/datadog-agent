---
name: omnibus-to-bazel
description: Convert an omnibus/config/software/<name>.rb dependency to a Bazel third-party dep under deps/. Use when asked to migrate, convert, or add a dep from omnibus to Bazel.
argument-hint: "<software-name>"
allowed-tools: Read, Write, Edit, Glob, Grep, Bash
---

Convert `omnibus/config/software/$ARGUMENTS.rb` into a Bazel third-party dep.

## Steps

### 1. Read the omnibus file

Read `omnibus/config/software/$ARGUMENTS.rb` and extract:
- `version` — the default version string
- `sha256` — from the `version(...)` block
- `source url` — the download URL (substitute the version to get the concrete URL)
- `ship_source_offer` — true/false
- `license` / `license_file` — what the omnibus file claims (treat as a hint only)

### 2. Download and inspect the archive

Download the archive to `/tmp` using `curl -sL <url> -o /tmp/<name>-<version>.tar.gz`.

Extract only the subset of files you need to verify licenses. For a dep that provides headers,
extract the header directory. For example:
```
tar -xzf /tmp/<name>-<version>.tar.gz <name>-<version>/path/to/headers/
```

Check `SPDX-License-Identifier` tags in the actual source files you will be using:
```
grep -h "SPDX-License-Identifier" /tmp/<name>-<version>/path/to/headers/*.h | sort -u
```

**Do not blindly trust the omnibus license list.** The omnibus file often lists all licenses in the
project, but we only use a subset of files. Verify which SPDX identifiers apply to the files we
actually consume.

### 3. Create `deps/<name>/`

Create the directory and `deps/<name>/<name>.BUILD.bazel` following this template:

```python
load("@@//compliance/rules:purl.bzl", "purl_for_generic")
load("@@//compliance/rules:ship_source_offer.bzl", "ship_source_offer")
load("@package_metadata//rules:package_metadata.bzl", "package_metadata")
load("@rules_cc//cc:defs.bzl", "cc_library")
load("@rules_license//rules:license.bzl", "license")
load("@rules_pkg//pkg:install.bzl", "pkg_install")
load("@rules_pkg//pkg:mappings.bzl", "pkg_filegroup", "pkg_files")

package(default_package_metadata = [":package_metadata", ":ship_source_offer"])

_VERSION = "<version>"

package_metadata(
    name = "package_metadata",
    attributes = [
        ":license",
        ":ship_source_offer",
    ],
    purl = purl_for_generic(
        package = "<name>",
        version = _VERSION,
        download_url = "<canonical upstream url with {version} placeholder>",
    ),
)

license(
    name = "license",
    license_kinds = ["@rules_license//licenses/spdx:<SPDX-ID>"],
    license_text = "<license file in the archive>",
    visibility = ["//visibility:public"],
)

ship_source_offer(name = "ship_source_offer")

_HEADERS = glob(["<path/to/headers>/*.h"])

cc_library(
    name = "<name>_headers",
    hdrs = _HEADERS,
    strip_include_prefix = "<path up to but not including the include dir>",
    visibility = ["//visibility:public"],
)

pkg_files(
    name = "hdr_files",
    srcs = _HEADERS,
    prefix = "embedded/include/<name>",
)

pkg_filegroup(
    name = "all_files",
    srcs = [":hdr_files"],
    visibility = ["@@//packages:__subpackages__"],
)

pkg_install(
    name = "install",
    srcs = [":all_files"],
)
```

Omit `cc_library`, `pkg_files`, `pkg_filegroup`, and `pkg_install` if this dep builds a library
rather than just exposing headers — those targets will be added in a later step.

Only add `ship_source_offer` if the omnibus file sets `ship_source_offer true`.

### 4. Add `http_archive` to `deps/repos.MODULE.bazel`

Insert alphabetically by `name` into `deps/repos.MODULE.bazel`:

```python
http_archive(
    name = "<name>",
    files = {
        "BUILD.bazel": "//deps:<name>/<name>.BUILD.bazel",
    },
    sha256 = "<sha256>",
    strip_prefix = "<name>-<version>",
    urls = [
        "https://github.com/<org>/<repo>/archive/refs/tags/v<version>.tar.gz",
    ],
)
```

If the archive has been mirrored to the S3 bucket, add:
`"https://dd-agent-omnibus.s3.amazonaws.com/bazel/<name>-<version>.tar.gz"` as the first URL.

### 5. Verify

Run:
```
bazel query @<name>//:all
```

The output must include at minimum:
- `@<name>//:license`
- `@<name>//:package_metadata`

If ship_source_offer was set in the omnibus script, it must include
- `@<name>//:ship_source_offer`

### 6. Update omnibus scripts

Search for every `dependency "<name>"` reference across all of `omnibus/config/`:
```
grep -rn 'dependency "<name>"' omnibus/config/
```

For each file that contains it, replace the `dependency "<name>"` line with an inline `build do`
block. Preserve any surrounding conditional (e.g. `if linux_target?`):

```ruby
if linux_target?
  build do
    command_on_repo_root "bazelisk run -- @<name>//:install --destdir='#{install_dir}'"
  end
end
```

If the `dependency` line had no conditional, omit the `if` wrapper. Look at adjacent `build do`
blocks in the same file for the exact style used there (some use `--destdir=` without quotes,
some with).

Also check `omnibus/config/projects/` — project files can declare `dependency` directly and are
easy to miss.

Finally, **delete `omnibus/config/software/<name>.rb`**. The version, sha, source URL, and build
logic now all live in Bazel; the `.rb` file is no longer needed.

## Key conventions

- **Visibility for `all_files`**: always `["@@//packages:__subpackages__"]`
- **License**: use the SPDX identifier from the source files, not the omnibus `.rb` file
- **`strip_include_prefix`**: set so that `#include <name/header.h>` works for downstream consumers
- **`_HEADERS` variable**: share the glob between `cc_library` and `pkg_files` to avoid duplication
- **Alphabetical order**: entries in `deps/repos.MODULE.bazel` are sorted alphabetically by `name`
- **Reference**: `deps/attr/attr.BUILD.bazel` is a good complete example to consult
