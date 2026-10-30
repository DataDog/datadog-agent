"""Multi-threaded xz compression."""

_XZ_THREADS = 8

_XZ_DEFAULT_LEVEL = 6

# Each thread needs 3*3*DictSize plus CompMem, see the "Presets" section of
# https://tukaani.org/xz/man/xz.1.html. Book for the worst case, -9.
_XZ_MEMORY_MB = _XZ_THREADS * (3 * 3 * 64 + 674)

def _xz_resource_set(_os, _inputs_size):
    return {"cpu": _XZ_THREADS, "memory": _XZ_MEMORY_MB}

def _xz_compress_impl(ctx):
    src = ctx.file.src
    out = ctx.outputs.out
    if out.dirname != src.dirname or not out.basename.startswith(src.basename) or out.basename == src.basename:
        fail("out (%s) must be next to src (%s) and named after it, with a suffix" % (out.path, src.path))

    level = ctx.attr.level if ctx.attr.level >= 0 else _XZ_DEFAULT_LEVEL
    args = ctx.actions.args()
    args.add("-%d" % level)
    args.add("-T%d" % _XZ_THREADS)
    args.add("--keep")
    args.add("--force")
    args.add("--suffix", out.basename[len(src.basename):])
    args.add("--")
    args.add(src)
    ctx.actions.run(
        mnemonic = "XzCompress",
        progress_message = "Compressing %{output}",
        executable = ctx.executable._xz,
        toolchain = None,
        inputs = [src],
        outputs = [out],
        arguments = [args],
        resource_set = _xz_resource_set,
    )
    return [DefaultInfo(files = depset([out]))]

xz_compress = rule(
    doc = "Compresses a file with xz using %d threads." % _XZ_THREADS,
    implementation = _xz_compress_impl,
    attrs = {
        "src": attr.label(allow_single_file = True, mandatory = True),
        "out": attr.output(mandatory = True),
        "level": attr.int(default = -1, values = list(range(-1, 10))),
        "_xz": attr.label(
            default = "@xz//:xz_cli",
            executable = True,
            cfg = "exec",
        ),
    },
)
