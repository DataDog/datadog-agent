# Vendored libfoldspace_go

Prebuilt shared objects for the native foldspace library, linked by
`core_native.go` under the `foldspace` build tag.

Only compiled artifacts and the C header (`../foldspace_go.h`) live here. The
library's own sources stay in its repository.

## Provenance

| | |
|---|---|
| Source | `DataDog/foldspace`, crate `bindings/go/native` (`foldspace-go-ffi`) |
| Commit | `926d529e59a3c8c3612ef7ba50911377c2294b22` |
| Rust | 1.94.0, per the library's `rust-toolchain.toml` |
| ABI | 4, as asserted against `FOLDSPACE_ABI_VERSION` at construction |
| glibc | Built for glibc 2.23, the Agent's build sysroot (cargo-zigbuild `.2.23` targets); needs at most `GLIBC_2.18` |

| Platform | sha256 |
|---|---|
| `linux_amd64/libfoldspace_go.so` | `31c34c3bfbe14d4e48a9b67ab095421b820d814820f2e817cb971aa779aa7889` |
| `linux_arm64/libfoldspace_go.so` | `94db4b598dd31108165fc22523b3e01486a1e8817b16c0c51137e685f2015764` |

Platforms without a binary here cannot build the `foldspace` tag, which is why
it appears in the excluded tag sets for darwin, Windows and AIX in
`tasks/build_tags.bzl`. Building on those platforms takes a locally produced
library and `CGO_LDFLAGS`.

## Regenerating

The header and the library are one artifact: the header declares the ABI a
particular revision exports, and a mismatch is caught at construction rather
than at link time. Take both from the same commit.

Both arches build in an `arm64` container with
[cargo-zigbuild](https://github.com/rust-cross/cargo-zigbuild), which
cross-compiles `amd64` (QEMU crashes gcc while building zstd's C sources) and,
through the `.2.23` target suffix, links against glibc 2.23 symbol versions.
The Agent's release packages link against a glibc 2.23 sysroot, so a library
built against the container's own glibc (2.34+ symbols such as
`pthread_key_create@GLIBC_2.34`) fails to link there. Check with
`objdump -T libfoldspace_go.so | grep -o 'GLIBC_[0-9.]*' | sort -uV | tail -1`.

```bash
git -C <foldspace> worktree add /tmp/fs-pin 926d529e59a3c8c3612ef7ba50911377c2294b22

docker run --rm --platform linux/arm64 \
  -v /tmp/fs-pin:/src:ro -v /tmp/fs-out:/out -w /src \
  -e CARGO_TARGET_DIR=/out/target -e CARGO_HOME=/out/cargo-home \
  rust:1.94.0 bash -c '
    set -euo pipefail
    apt-get update -qq && apt-get install -y -qq protobuf-compiler xz-utils
    curl -fsSLo /tmp/zig.tar.xz https://ziglang.org/download/0.14.1/zig-aarch64-linux-0.14.1.tar.xz
    mkdir -p /opt/zig && tar -xf /tmp/zig.tar.xz -C /opt/zig --strip-components=1
    export PATH="/opt/zig:$CARGO_HOME/bin:$PATH"
    cargo install --locked cargo-zigbuild
    rustup target add x86_64-unknown-linux-gnu aarch64-unknown-linux-gnu
    # A cdylib carries no SONAME by default, which leaves the linker recording
    # whatever path it resolved at build time.
    export RUSTFLAGS="-C link-arg=-Wl,-soname,libfoldspace_go.so"
    for target in x86_64-unknown-linux-gnu aarch64-unknown-linux-gnu; do
      cargo zigbuild --release --locked --target "$target.2.23" -p foldspace-go-ffi
    done'
```

Copy `libfoldspace_go.so` from each target's `release/` directory into the
matching directory here, and update the header, the commit and the checksums
above.
