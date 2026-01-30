# Lib Deep Inference

This Rust library is used to do machine learning inference on CPU relying on Rust / Candle.

## Dev

Build:
```sh
bazel build //pkg/deepinference/rust:libdeepinference
```

If you need to regenerate `Cargo.Bazel.lock`:
```sh
CARGO_BAZEL_REPIN=1 bazel build //pkg/deepinference/rust:libdeepinference
```
