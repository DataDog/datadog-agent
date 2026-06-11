"""whl_extract — extract a Python wheel (.whl) file into a directory TreeArtifact.

A .whl is a zip archive. This rule runs `python -m zipfile -e` to unpack it
into a declared-directory output so that rules_pkg pkg_files can place the
extracted content into the final package layout.

Usage:
    whl_extract(
        name = "my_pkg_extracted",
        whl  = ":my_pkg_wheel",
    )

    pkg_files(
        name = "my_pkg_files",
        srcs = [":my_pkg_extracted"],
        prefix = "embedded/lib/python3.13/site-packages",
        renames = {":my_pkg_extracted": REMOVE_BASE_DIRECTORY},
    )

multi_whl_extract — extract multiple Python wheels into a single merged TreeArtifact.

rules_pkg uses a dest-keyed dict (content_map) when building manifests.  Two
pkg_files entries that both use REMOVE_BASE_DIRECTORY and the same prefix map
to the *same* dest key; the second silently overwrites the first, so only the
last wheel in the list lands in the package.

multi_whl_extract avoids the collision by extracting all wheels into ONE
TreeArtifact.  A single pkg_files + REMOVE_BASE_DIRECTORY entry then covers
all of them safely.

Usage:
    multi_whl_extract(
        name = "site_packages_tree",
        whls = [
            ":wheel_a",
            ":wheel_b_renamed",
        ],
    )

    pkg_files(
        name = "site_packages_files",
        srcs = [":site_packages_tree"],
        prefix = "embedded/lib/python3.13/site-packages",
        renames = {":site_packages_tree": REMOVE_BASE_DIRECTORY},
    )
"""

def _whl_extract_impl(ctx):
    toolchain = ctx.toolchains["@rules_python//python:toolchain_type"]
    runtime = toolchain.py3_runtime

    out_dir = ctx.actions.declare_directory(ctx.attr.name)
    whl = ctx.file.whl

    if runtime.interpreter:
        python_path = runtime.interpreter.path
        inputs = depset([whl, runtime.interpreter], transitive = [runtime.files])
    else:
        # System interpreter (interpreter_path is a string, not a File).
        python_path = runtime.interpreter_path
        inputs = depset([whl])

    ctx.actions.run_shell(
        inputs = inputs,
        outputs = [out_dir],
        # Pass whl and out as positional args to avoid quoting issues with paths
        # that may contain spaces.
        command = '"$1" -m zipfile -e "$2" "$3"',
        arguments = [python_path, whl.path, out_dir.path],
        mnemonic = "WhlExtract",
        progress_message = "Extracting wheel %s" % whl.basename,
    )

    return [DefaultInfo(files = depset([out_dir]))]

whl_extract = rule(
    doc = """Extract a Python wheel (.whl) file into a directory TreeArtifact.

    The output is a Bazel TreeArtifact (declared directory) containing the
    unzipped wheel contents.  Pair with pkg_files(..., renames = {target:
    REMOVE_BASE_DIRECTORY}) to place the extracted files directly under a
    site-packages prefix.
    """,
    implementation = _whl_extract_impl,
    attrs = {
        "whl": attr.label(
            doc = "The .whl file to extract.",
            mandatory = True,
            allow_single_file = [".whl"],
        ),
    },
    toolchains = ["@rules_python//python:toolchain_type"],
)

def _multi_whl_extract_impl(ctx):
    """Extract multiple wheels into a single merged TreeArtifact.

    Each wheel is unzipped in order into the same output directory using
    `python -m zipfile -e`.  If two wheels provide the same file (e.g. a
    shared namespace __init__.py with identical content) the later wheel's
    copy wins, which is harmless for pure-Python namespace packages.

    After extraction, if `python` is provided (or falls back to the toolchain
    interpreter):
      - Runs `python -m compileall` to generate __pycache__/*.pyc files,
        matching the bytecode cache that omnibus produces via `pip install`.
        SOURCE_DATE_EPOCH is fixed for reproducible .pyc timestamps.
      - Synthesizes INSTALLER and REQUESTED metadata files in every
        .dist-info directory, matching the files pip generates on
        `pip install`.
    """
    toolchain = ctx.toolchains["@rules_python//python:toolchain_type"]
    runtime = toolchain.py3_runtime

    out_dir = ctx.actions.declare_directory(ctx.attr.name)
    whls = ctx.files.whls

    # Determine the Python interpreter to use.  If the caller provides an
    # explicit `python` executable (e.g. @python_3_13//:python3 to match the
    # embedded CPython ABI), use it directly.  Otherwise fall back to the
    # resolved toolchain interpreter.
    if ctx.attr.python:
        python_exec = ctx.executable.python
        python_path = python_exec.path
        python_runfiles = ctx.attr.python[DefaultInfo].default_runfiles
        inputs = depset(whls + [python_exec], transitive = [python_runfiles.files])
    elif runtime.interpreter:
        python_path = runtime.interpreter.path
        inputs = depset(whls + [runtime.interpreter], transitive = [runtime.files])
    else:
        # System interpreter (interpreter_path is a string, not a File).
        python_path = runtime.interpreter_path
        inputs = depset(whls)

    # Extract each wheel directly into out_dir.  `python -m zipfile -e` places
    # the zip contents directly under the given directory (no sub-directory is
    # created), so successive extractions naturally merge into one flat tree.
    extract_cmds = [
        '"{python}" -m zipfile -e "{whl}" "$OUT"'.format(
            python = python_path,
            whl = whl.path,
        )
        for whl in whls
    ]

    # Compile all extracted .py files to produce __pycache__/*.pyc files,
    # matching the bytecode cache that omnibus generates via `pip install`.
    # SOURCE_DATE_EPOCH is fixed to a stable value so .pyc mtime fields are
    # reproducible across rebuilds (required by Bazel hermeticity: two builds
    # with the same inputs must produce bit-identical outputs).
    # --invalidation-mode unchecked-hash skips mtime-based invalidation so the
    # .pyc files remain valid regardless of when the source files are accessed.
    # Errors are suppressed (|| true) because some C-extension stubs or
    # syntax-error test fixtures may not compile; that is harmless.
    extract_cmds.append(
        'SOURCE_DATE_EPOCH=315532800 "{python}" -m compileall -q --invalidation-mode unchecked-hash "$OUT" 2>/dev/null || true'.format(
            python = python_path,
        ),
    )

    # Synthesize pip install metadata in every .dist-info directory.
    # Omnibus runs `pip install` which writes INSTALLER ("pip\n") and
    # REQUESTED (empty marker) into each dist-info.  Wheels extracted with
    # `python -m zipfile -e` do not contain these files; generate them here
    # so the installed package tree matches the omnibus layout.
    extract_cmds.append(
        r"""
find "$OUT" -maxdepth 2 -type d -name '*.dist-info' | while IFS= read -r d; do
  printf 'pip\n' > "$d/INSTALLER"
  touch "$d/REQUESTED"
done
""",
    )

    ctx.actions.run_shell(
        inputs = inputs,
        outputs = [out_dir],
        env = {"OUT": out_dir.path},
        command = "\n".join(extract_cmds),
        mnemonic = "MultiWhlExtract",
        progress_message = "Extracting %d wheels into %s" % (len(whls), out_dir.basename),
    )

    return [DefaultInfo(files = depset([out_dir]))]

multi_whl_extract = rule(
    doc = """Extract multiple Python wheels into a single merged TreeArtifact.

    Avoids the rules_pkg content_map dest-key collision that drops all but the
    last pkg_files entry when multiple per-wheel pkg_files rules use
    REMOVE_BASE_DIRECTORY and the same prefix.

    All wheels are extracted into one declared directory, which is then covered
    by a single pkg_files + REMOVE_BASE_DIRECTORY entry.

    After extraction, __pycache__/*.pyc files are compiled via compileall
    and pip install metadata (INSTALLER, REQUESTED) is synthesized in every
    .dist-info directory to match the omnibus site-packages layout.

    Optionally pass `python` pointing to a specific interpreter (e.g.
    @python_3_13//:python3) when the embedded CPython ABI differs from the
    default rules_python toolchain.
    """,
    implementation = _multi_whl_extract_impl,
    attrs = {
        "whls": attr.label_list(
            doc = "The .whl files to extract, in order.",
            mandatory = True,
            allow_files = [".whl"],
        ),
        "python": attr.label(
            doc = """Optional explicit Python interpreter executable.

            When provided, this interpreter is used instead of the
            toolchain-resolved one to run compileall.  Use this when the
            integration wheels are built for a specific CPython version that
            differs from the default rules_python toolchain (e.g.
            @python_3_13//:python3 for CPython 3.13 ABI wheels).
            """,
            mandatory = False,
            allow_single_file = True,
            executable = True,
            cfg = "exec",
        ),
    },
    toolchains = ["@rules_python//python:toolchain_type"],
)
