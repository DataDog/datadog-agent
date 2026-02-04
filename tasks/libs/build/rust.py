# Utils to build Rust libraries

import glob
import os
import shutil
import sys

from invoke import Context

from tasks.libs.common.utils import get_embedded_path, gitlab_section


def build_rust_lib(
    ctx: Context, libpath: str, libname: str, env: dict, embedded_path=None, target_arch=None, profile="release"
):
    if embedded_path is None:
        embedded_path = get_embedded_path(ctx)

    # TODO: Windows support
    target_os = os.getenv("GOOS") or sys.platform
    if target_os not in ("windows", "win32"):
        with gitlab_section(f"Build {libname} rust library", collapsed=True):
            rustenv = env.copy()
            if glob.glob(os.path.join(embedded_path, "lib", "libssl*.so")):
                rustenv["OPENSSL_DIR"] = embedded_path
                rustenv["OPENSSL_LIB_DIR"] = os.path.join(embedded_path, "lib")
                rustenv["OPENSSL_INCLUDE_DIR"] = os.path.join(embedded_path, "include")
            if os.path.exists(os.path.join(embedded_path, "lib", "pkgconfig")):
                rustenv["PKG_CONFIG_PATH"] = os.path.join(embedded_path, "lib", "pkgconfig")
            if sys.platform.startswith("linux"):
                rustenv["RUSTFLAGS"] = (
                    f"-C link-arg=-Wl,-rpath={os.path.join(embedded_path, 'lib')} "
                    f"-C link-arg=-L{os.path.join(embedded_path, 'lib')}"
                )
            elif sys.platform == "darwin":
                rustenv["RUSTFLAGS"] = (
                    f"-C link-arg=-Wl,-rpath,{os.path.join(embedded_path, 'lib')} "
                    f"-C link-arg=-L{os.path.join(embedded_path, 'lib')}"
                )
            else:
                rustenv["RUSTFLAGS"] = f"-C link-arg=-L{os.path.join(embedded_path, 'lib')}"

            if os.uname().machine == "arm64":
                # TODO: Verify this, we shouldn't use fp16 in theory
                rustenv["RUSTFLAGS"] += " -C target-feature=+fp16"

            print("CC rustenv:")
            for k, v in rustenv.items():
                print(f"{k}: {v}")

            with ctx.cd(libpath):
                target_arg = f"--target {target_arch}" if target_arch else ""
                profile_arg = '--release' if profile == 'release' else ''
                ctx.run(
                    f"cargo build {profile_arg} {target_arg}",
                    env=rustenv,
                )

        if embedded_path is not None:
            ext = "so" if sys.platform.startswith("linux") else "dylib"
            target_dir = f"{target_arch}/" if target_arch else ""
            final_lib_path = os.path.join(embedded_path, "lib", f"lib{libname}.{ext}")
            shutil.move(
                f"{libpath}/target/{target_dir}{profile}/lib{libname}.{ext}",
                final_lib_path,
            )

            # On Linux, use patchelf to set rpath so the library can find OpenSSL at runtime
            if sys.platform.startswith("linux"):
                openssl_lib_dir = os.path.join(embedded_path, "lib")
                ctx.run(f"patchelf --add-rpath {openssl_lib_dir} {final_lib_path}")

    # TODO: Do it only once
    # Add OpenSSL library directory to linker search path for Go build
    if target_os not in ("windows", "win32"):
        openssl_lib_dir = os.path.join(embedded_path, "lib")
        # Add to CGO_LDFLAGS so the linker can find OpenSSL libraries
        if sys.platform.startswith("linux"):
            if 'CGO_LDFLAGS' in env:
                env['CGO_LDFLAGS'] += f" -L{openssl_lib_dir} -Wl,-rpath-link={openssl_lib_dir}"
            else:
                env['CGO_LDFLAGS'] = f"-L{openssl_lib_dir} -Wl,-rpath-link={openssl_lib_dir}"
            # Add to LD_LIBRARY_PATH for runtime library resolution
            if 'LD_LIBRARY_PATH' in env:
                env['LD_LIBRARY_PATH'] = f"{openssl_lib_dir}:{env['LD_LIBRARY_PATH']}"
            else:
                env['LD_LIBRARY_PATH'] = openssl_lib_dir
        elif sys.platform == "darwin":
            # On macOS, rpath-link is not supported; use -rpath instead and DYLD_LIBRARY_PATH at runtime
            if 'CGO_LDFLAGS' in env:
                env['CGO_LDFLAGS'] += f" -L{openssl_lib_dir} -Wl,-rpath,{openssl_lib_dir}"
            else:
                env['CGO_LDFLAGS'] = f"-L{openssl_lib_dir} -Wl,-rpath,{openssl_lib_dir}"
            # Add to DYLD_LIBRARY_PATH for runtime library resolution
            if 'DYLD_LIBRARY_PATH' in env:
                env['DYLD_LIBRARY_PATH'] = f"{openssl_lib_dir}:{env['DYLD_LIBRARY_PATH']}"
            else:
                env['DYLD_LIBRARY_PATH'] = openssl_lib_dir
