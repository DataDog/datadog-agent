# pkg/util/sds

This package wraps the [Datadog Sensitive Data Scanner (SDS)](https://github.com/DataDog/dd-sensitive-data-scanner) shared library and exposes a small scanner API: create a scanner, configure it with rules and scan events (e.g. to redact sensitive data).


## Build and Test
## Prepare
```bash
dda inv rtloader.clean
dda inv -- rtloader.make 
dda inv -- rtloader.install
dda inv -- sds.build-library
```

## Test
```bash
dda inv -- test --targets ./pkg/util/sds --include-sds
```

## Build agent binary
```bash
dda inv -- agent.build --include-sds
```

## Test in dda env

```bash
dda env dev start 
dda env dev shell # open shell in your dev env
dda inv rtloader.clean
dda inv -- rtloader.make 
dda inv -- rtloader.install
dda inv -- sds.build-library
dda inv -- test --targets ./pkg/util/sds --include-sds
```



## Test with bzl

```bash
bazel test //pkg/util/sds:all
```

Please use env to format rtloader
```bash
dda inv -- -e rtloader.format
```

Remember also that we need to generate 3rd party license for rust in dda dnv dev 