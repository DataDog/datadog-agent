"""Toolchains providing the Windows message and resource compilers of an exec platform."""

def _winlibs_toolchain_impl(ctx):
    windmc = ctx.attr._windmc[DefaultInfo].files_to_run

    def compile_messagetable(actions, src, basename, h_out):
        rc_out = actions.declare_file("{}.rc".format(basename))
        bin_out = actions.declare_file("MSG00409.bin")
        windmc_args = actions.args()
        windmc_args.add("--target", "pe-x86-64")
        windmc_args.add("-r", rc_out.dirname)
        windmc_args.add("-h", rc_out.dirname)
        windmc_args.add(src)
        actions.run(
            executable = windmc,
            arguments = [windmc_args],
            inputs = [src],
            outputs = [rc_out, h_out, bin_out],
            mnemonic = "WindMC",
            progress_message = "Compiling message table %{input}",
        )
        return rc_out, [bin_out]

    return [platform_common.ToolchainInfo(
        compile_messagetable = compile_messagetable,
        windres = ctx.attr._windres[DefaultInfo].files_to_run,
        windres_args = ["--use-temp-file"],
    )]

winlibs_toolchain = rule(
    implementation = _winlibs_toolchain_impl,
    attrs = {
        "_windmc": attr.label(default = "@winlibs_mingw64//:windmc", cfg = "exec", executable = True, allow_files = True),
        "_windres": attr.label(default = "@winlibs_mingw64//:windres", cfg = "exec", executable = True, allow_files = True),
    },
)

def _wine_llvm_toolchain_impl(ctx):
    wmc = ctx.attr._wmc[DefaultInfo].files_to_run
    nls = ctx.files._nls

    def compile_messagetable(actions, src, basename, h_out):
        res_out = actions.declare_file("{}.res".format(basename))
        nls_dir = "--nls-dir={}".format(nls[0].dirname)
        actions.run(
            executable = wmc,
            arguments = [nls_dir, "-O", "res", "-o", res_out.path, src.path],
            inputs = [src] + nls,
            outputs = [res_out],
            mnemonic = "Wmc",
            progress_message = "Compiling message table %{input} into %{output}",
        )
        actions.run(
            executable = wmc,
            arguments = [nls_dir, "-i", "-H", h_out.path, "-o", "/dev/null", src.path],  # discard .rc file
            inputs = [src] + nls,
            outputs = [h_out],
            mnemonic = "Wmc",
            progress_message = "Compiling message table %{input} into %{output}",
        )
        return res_out, []

    return [platform_common.ToolchainInfo(
        compile_messagetable = compile_messagetable,
        windres = ctx.attr._llvm[DefaultInfo].files_to_run,
        windres_args = [
            "windres",
            "--preprocessor={}".format(ctx.executable._llvm.path),
            "--preprocessor-arg=clang",
            "--preprocessor-arg=-target",
            "--preprocessor-arg=x86_64-w64-mingw32",
            "--preprocessor-arg=-E",
            "--preprocessor-arg=-xc",
            "--preprocessor-arg=-DRC_INVOKED",
        ],
    )]

wine_llvm_toolchain = rule(
    implementation = _wine_llvm_toolchain_impl,
    attrs = {
        "_llvm": attr.label(default = "//:llvm", cfg = "exec", executable = True, allow_files = True),
        "_nls": attr.label(default = "@wine_linux_x86_64//:nls", cfg = "exec", allow_files = True),
        "_wmc": attr.label(default = "@wine_linux_x86_64//:wmc", cfg = "exec", executable = True, allow_files = True),
    },
)
