"""Bazel rule to generate a minimized CO-RE BTF for a single kernel.

Replaces the ninja `minimize_btf` rule in `tasks/system_probe.py`
(`bpftool gen min_core_btf <full-btf> <out-btf> <co-re .o files...>`), one Bazel
action per kernel BTF.

Why per-kernel actions: the cache key becomes hash(full BTF + CO-RE objects +
bpftool + command line), content-addressed by Bazel and shared through the
Buildbarn remote cache across CI pipelines and local dev. A btfhub update then
re-minimizes only the kernels whose full BTF actually changed. (Note: because
`min_core_btf` takes the union of all programs, any CO-RE `.o` change still
re-minimizes every kernel — same blast radius as the current S3-cache approach on
that axis.)
"""

_TOOLCHAIN_TYPE = "@@//bazel/toolchains/bpftool:bpftool_toolchain_type"

def _min_core_btf_impl(ctx):
    tc = ctx.toolchains[_TOOLCHAIN_TYPE].bpftool
    if not tc.valid:
        fail("bpftool toolchain is not available")

    out = ctx.actions.declare_file(ctx.label.name + ".btf")
    programs = ctx.files.programs

    args = ctx.actions.args()
    args.add("gen")
    args.add("min_core_btf")
    args.add(ctx.file.btf)
    args.add(out)
    args.add_all(programs)

    ctx.actions.run(
        inputs = depset([ctx.file.btf] + programs),
        outputs = [out],
        executable = tc.bpftool,
        arguments = [args],
        mnemonic = "MinCoreBtf",
        toolchain = _TOOLCHAIN_TYPE,
        progress_message = "Minimizing BTF %{label}",
    )

    return [DefaultInfo(files = depset([out]))]

min_core_btf = rule(
    implementation = _min_core_btf_impl,
    doc = "Generate a minimized CO-RE BTF from one full kernel BTF and a set of " +
          "CO-RE eBPF object files, using `bpftool gen min_core_btf`. Linux-only.",
    attrs = {
        "btf": attr.label(
            mandatory = True,
            allow_single_file = [".btf"],
            doc = "The full (decompressed) kernel BTF to minimize.",
        ),
        "programs": attr.label_list(
            allow_files = [".o"],
            doc = "CO-RE eBPF object files whose referenced types the minimized " +
                  "BTF must retain. Pass the ebpf_prog targets directly.",
        ),
    },
    toolchains = [_TOOLCHAIN_TYPE],
)
