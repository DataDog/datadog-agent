# How we build openssl with bazel.

## Overall

- run configure/make in a work directory
- use tools to
  - grab the generated files and save them in overlay/<config>/...
  - update overlay/lib_contents.bzl to account for all the files needed to build the library.
- hand tune BUILD.bazel to match the copts used in the configured make.

## Regenerating saved configured files

1. You will need to check out DataDog/experimental. In my example I keep that at $HOME/ws/experimental.
2. You will need to have grabbed an openssl tarball and untarred it somewhere, typically here.

```

### Step 1: Create the configured dir

```
C2B=$HOME/ws/experimental/teams/agent-supply-chain/configure2bazel
PRISTINE=openssl-3.5.4

python3 $C2B/add_configuration.py \
    --pristine_dir=$PRISTINE \
    --configured_name=linux_arm64 \
    --configure_tool=Configure \
    --configure_options=config_opts.txt
```

### Step 2: build it with make

```
cd linux_arm64
make >make_out.txt   # we save the output to use the examine_build tool later.

```

### Step 3: grab the generated files

```
python3 $C2B/analyze.py \
    --pristine_dir=$PRISTINE \
    --configured_dir=linux_arm64 \
    --configured_name=linux_arm64 \
    --overlay=overlay
```

analyze.py is still a bit rough. It may not find files it knows were generated.
You may have to hand copy them from linux_arm64 to overlay/linux_arm64.

### Step 4: upate overlay/overlay.BUILD.bazel

Generally we only do this once. Find the `select` clauses and add the
new case, following the existing pattern.

### Step 5: Regenrate the MODULE file

You need to do this when you change the content of overlay.

```
bazel build //deps/openssl3:openssl3.MODULE.bazel.new
cp bazel-bin/deps/openssl3/openssl3.MODULE.bazel.new deps/openssl3/openssl3.MODULE.bazel
```
