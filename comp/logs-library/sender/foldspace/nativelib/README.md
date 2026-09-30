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

| Platform | sha256 |
|---|---|
| `linux_amd64/libfoldspace_go.so` | `f00ec97ccefd01c62975eb3e83cc3479b8aac0798168998e04fa9f3cd491d1c8` |
| `linux_arm64/libfoldspace_go.so` | `865842f53a28f670c807713e8fc6d842a0b51b64d8fec1f91b8075bff23c89be` |

Platforms without a binary here cannot build the `foldspace` tag, which is why
it appears in the excluded tag sets for darwin, Windows and AIX in
`tasks/build_tags.bzl`. Building on those platforms takes a locally produced
library and `CGO_LDFLAGS`.

## Regenerating

The header and the library are one artifact: the header declares the ABI a
particular revision exports, and a mismatch is caught at construction rather
than at link time. Take both from the same commit.

Both arches build in an `arm64` container. `amd64` is cross-compiled rather
than emulated, because QEMU crashes gcc while building zstd's C sources.

```bash
git -C <foldspace> worktree add /tmp/fs-pin 926d529e59a3c8c3612ef7ba50911377c2294b22

docker run --rm --platform linux/arm64 \
  -v /tmp/fs-pin:/src:ro -v /tmp/fs-out:/out -w /src \
  -e CARGO_TARGET_DIR=/out/target \
  rust:1.94.0 bash -c '
    apt-get update -qq && apt-get install -y -qq protobuf-compiler gcc-x86-64-linux-gnu
    rustup target add x86_64-unknown-linux-gnu aarch64-unknown-linux-gnu
    export CARGO_TARGET_X86_64_UNKNOWN_LINUX_GNU_LINKER=x86_64-linux-gnu-gcc
    export CC_x86_64_unknown_linux_gnu=x86_64-linux-gnu-gcc
    # A cdylib carries no SONAME by default, which leaves the linker recording
    # whatever path it resolved at build time.
    export RUSTFLAGS="-C link-arg=-Wl,-soname,libfoldspace_go.so"
    for target in x86_64-unknown-linux-gnu aarch64-unknown-linux-gnu; do
      cargo build --release --target "$target" -p foldspace-go-ffi
    done'
```

Copy `libfoldspace_go.so` from each target's `release/` directory into the
matching directory here, and update the header, the commit and the checksums
above.
