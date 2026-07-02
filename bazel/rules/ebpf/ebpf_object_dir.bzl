"""Assemble a directory of built eBPF .o programs for tools that read DD_SYSTEM_PROBE_BPF_DIR."""

def _ebpf_object_dir_impl(ctx):
    out = ctx.actions.declare_directory(ctx.label.name)
    inputs = []
    copies = []
    for target in ctx.attr.prebuilt:
        f = target.files.to_list()[0]
        inputs.append(f)
        copies.append("cp {src} {dst}/{name}.o".format(src = f.path, dst = out.path, name = target.label.name))
    for target in ctx.attr.core:
        f = target.files.to_list()[0]
        inputs.append(f)
        copies.append("cp {src} {dst}/co-re/{name}.o".format(src = f.path, dst = out.path, name = target.label.name))

    ctx.actions.run_shell(
        outputs = [out],
        inputs = inputs,
        command = "mkdir -p {root}/co-re && {copies}".format(root = out.path, copies = " && ".join(copies)),
        mnemonic = "EbpfObjectDir",
        progress_message = "Assembling eBPF object directory %{output}",
    )
    return [DefaultInfo(files = depset([out]), runfiles = ctx.runfiles(files = [out]))]

ebpf_object_dir = rule(
    implementation = _ebpf_object_dir_impl,
    doc = "Copies prebuilt/CO-RE eBPF .o targets into a single flat directory, with CO-RE variants under co-re/.",
    attrs = {
        "prebuilt": attr.label_list(
            allow_files = [".o"],
            doc = "eBPF .o targets, placed directly under the output directory as <target name>.o.",
        ),
        "core": attr.label_list(
            allow_files = [".o"],
            doc = "eBPF .o targets, placed under the output directory's co-re/ subdirectory as <target name>.o.",
        ),
    },
)
