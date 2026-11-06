# Vendored libfoldspace_go

Prebuilt shared objects for the native foldspace library, linked by
`core_native.go` under the `foldspace` build tag.

Only compiled artifacts and the C header (`../foldspace_go.h`) live here. The
library's own sources stay in its repository.

## Provenance

| | |
|---|---|
| Source | `DataDog/foldspace`, crate `bindings/go/native` (`foldspace-go-ffi`) |
| Commit | `271aae809fdefab7f8d0e2b4d2a83d955e2219f5` |
| Rust | 1.94.0, per the library's `rust-toolchain.toml` |
| ABI | 1, as asserted against `FOLDSPACE_ABI_VERSION` at construction |

| Platform | sha256 |
|---|---|
| `linux_amd64/libfoldspace_go.so` | `61afd558b96832c401981cd5b8d0eba40771eda39af1c6990accfab806131fb6` |
| `linux_arm64/libfoldspace_go.so` | `7a0aba0a7abe1b851db1c85fd3d387bfddeee3ccfe95e6a93f35b0bcf5f86f04` |

Platforms without a binary here cannot build the `foldspace` tag from this
directory. Windows and AIX exclude the tag in `tasks/build_tags.bzl`. Darwin
does not: it links a locally produced library through `CGO_LDFLAGS`.

## Regenerating

The header and the library are one artifact: the header declares the ABI a
particular revision exports, and a mismatch is caught at construction rather
than at link time. Take both from the same commit.

The agent links this library with the cross toolchains in the linux build
image (`CI_IMAGE_LINUX` in `.gitlab-ci.yml`), whose sysroots carry glibc 2.17
for `amd64` and 2.23 for `arm64`. A library linked against a newer glibc
references symbol versions those sysroots lack, and the agent link fails with
`undefined reference to ...@GLIBC_2.xx`. Build it with the same toolchains, in
the same image, so it inherits that floor.

Both arches build in an `arm64` container, using the image's cross compiler for
each target. `amd64` is cross-compiled rather than emulated, because QEMU
crashes gcc while building zstd's C sources. The paths are under `$HOME`
because that is the one directory every macOS Docker VM shares by default;
Colima shares nothing else.

```bash
git -C <foldspace> worktree add ~/fs-pin 271aae809fdefab7f8d0e2b4d2a83d955e2219f5

IMAGE="registry.ddbuild.io/ci/datadog-agent-buildimages/linux:$(awk '/^  CI_IMAGE_LINUX:/ {print $2}' .gitlab-ci.yml)"
docker run --rm --platform linux/arm64 \
  -v ~/fs-pin:/src:ro -v ~/fs-out:/out -w /src \
  -e CARGO_TARGET_DIR=/out/target \
  "$IMAGE" bash -lc '
    set -euo pipefail
    rustup target add x86_64-unknown-linux-gnu aarch64-unknown-linux-gnu
    for arch in x86_64 aarch64; do
      target_env=$(echo "${arch}_unknown_linux_gnu" | tr a-z A-Z)
      export "CARGO_TARGET_${target_env}_LINKER=${arch}-linux-gnu-gcc"
      export "CC_${arch}_unknown_linux_gnu=${arch}-linux-gnu-gcc"
      export "CXX_${arch}_unknown_linux_gnu=${arch}-linux-gnu-g++"
      export "AR_${arch}_unknown_linux_gnu=${arch}-linux-gnu-ar"
    done
    # A cdylib carries no SONAME by default, which leaves the linker recording
    # whatever path it resolved at build time.
    export RUSTFLAGS="-C link-arg=-Wl,-soname,libfoldspace_go.so"
    for target in x86_64-unknown-linux-gnu aarch64-unknown-linux-gnu; do
      cargo build --locked --release --target "$target" -p foldspace-go-ffi
    done'
```

The image's `rustup` installs the toolchain the library's `rust-toolchain.toml`
pins. Check the result before vendoring it: the highest `GLIBC_` version under
"Version References" in `objdump -p libfoldspace_go.so` is at most 2.17 for
`amd64` and 2.23 for `arm64`.

Copy `libfoldspace_go.so` from each target's `release/` directory into the
matching directory here, and update the header, the commit and the checksums
above.
