load("@package_metadata//rules:package_metadata.bzl", "package_metadata")
load("@rules_rs//rs/private:rust_deps.bzl", "rust_deps")
load("@rules_rust//cargo/private:cargo_build_script_wrapper.bzl", "cargo_build_script")
load("@rules_rust//rust:defs.bzl", "rust_binary", "rust_library", "rust_proc_macro")

# A small fork of @rules_rs//rs:rust_crate.bzl with one extra knob:
# build_script_rundir. rules_rs' rust_crate macro does not expose the
# underlying rules_rust cargo_build_script.rundir attribute, but Windows needs
# it for cryptography's nested source-overlay packages. Without it, rules_rust
# runs build.rs from a synthetic CARGO_MANIFEST_DIR runfiles tree; on Windows
# that has proven fragile for these external, nested packages. Setting
# build_script_rundir = "." keeps build.rs execution in the execroot while
# preserving CARGO_MANIFEST_DIR.

def cryptography_rust_crate(
        name,
        crate_name,
        version,
        aliases,
        deps,
        data,
        crate_features,
        crate_root,
        edition,
        rustc_flags,
        target_compatible_with,
        links,
        build_script,
        build_script_data,
        build_deps,
        build_script_env,
        build_script_toolchains,
        build_script_tools,
        is_proc_macro,
        binaries,
        build_script_rundir = None):
    package_metadata(
        name = name + "_package_metadata",
        purl = "pkg:cargo/%s/%s" % (crate_name, version),
        visibility = ["//visibility:public"],
    )

    compile_data = native.glob(
        include = ["**"],
        exclude = [
            "**/* *",
            ".tmp_git_root/**/*",
            "BUILD",
            "BUILD.bazel",
            "REPO.bazel",
            "Cargo.toml.orig",
            "WORKSPACE",
            "WORKSPACE.bazel",
        ],
        allow_empty = True,
    )

    srcs = native.glob(
        include = ["**/*.rs"],
        allow_empty = True,
    )

    tags = [
        "crate-name=" + name,
        "manual",
        "noclippy",
        "norustfmt",
    ]

    if build_script:
        rust_deps(
            name = name + "_build_script_deps",
            deps = build_deps,
        )

        rust_deps(
            name = name + "_build_script_proc_macro_deps",
            deps = build_deps,
            proc_macros = True,
        )

        cargo_build_script(
            name = name + "_build_script",
            aliases = aliases,
            compile_data = compile_data,
            crate_features = crate_features,
            crate_name = "build_script_build",
            crate_root = build_script,
            links = links,
            data = compile_data + build_script_data,
            deps = [name + "_build_script_deps"],
            link_deps = deps,
            build_script_env = build_script_env,
            build_script_env_files = ["cargo_toml_env_vars.env"],
            toolchains = build_script_toolchains,
            tools = build_script_tools,
            proc_macro_deps = [name + "_build_script_proc_macro_deps"],
            edition = edition,
            pkg_name = crate_name,
            rundir = build_script_rundir,
            rustc_env_files = ["cargo_toml_env_vars.env"],
            rustc_flags = ["--cap-lints=allow"],
            srcs = srcs,
            target_compatible_with = target_compatible_with,
            tags = tags,
            version = version,
        )

        maybe_build_script = [name + "_build_script"]
    else:
        maybe_build_script = []

    rust_deps(
        name = name + "_deps",
        deps = deps,
    )

    rust_deps(
        name = name + "_proc_macro_deps",
        deps = deps,
        proc_macros = True,
    )

    deps = [name + "_deps"] + maybe_build_script

    kwargs = dict(
        name = name,
        crate_name = crate_name,
        version = version,
        srcs = srcs,
        compile_data = compile_data,
        aliases = aliases,
        deps = deps,
        data = data,
        proc_macro_deps = [name + "_proc_macro_deps"],
        crate_features = crate_features,
        crate_root = crate_root,
        edition = edition,
        rustc_env_files = ["cargo_toml_env_vars.env"],
        rustc_flags = rustc_flags + ["--cap-lints=allow"],
        tags = tags,
        target_compatible_with = target_compatible_with,
        package_metadata = [name + "_package_metadata"],
        visibility = ["//visibility:public"],
    )

    if is_proc_macro:
        rust_proc_macro(**kwargs)
    else:
        rust_library(**kwargs)

    for bin_name, bin_path in binaries.items():
        rust_binary(
            name = bin_name + "__bin",
            srcs = [bin_path],
            aliases = aliases,
            deps = deps,
            edition = edition,
            crate_features = crate_features,
            tags = tags,
            visibility = ["//visibility:public"],
        )
