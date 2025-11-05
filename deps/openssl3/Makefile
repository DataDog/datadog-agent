C2B=$(HOME)/ws/experimental/teams/agent-supply-chain/configure2bazel
PRISTINE=openssl-3.5.4

next:
	echo Do both steps on each platform.
	echo Adjust overlay/overlay.BUILD.bazel

macos1:
	python3 add_configuration.py \
	  --pristine_dir=$(PRISTINE) \
	  --configure_options=config_opts.txt
	echo Maybe: cd linux_aarch64 ; make
	echo Maybe: cd darwin_arm64 ; make

macos2:
	python3 $(C2B)/analyze.py \
	  --pristine_dir=$(PRISTINE) \
	  --configured_dir=darwin_arm64 \
          --configured_name=darwin_arm64 \
          --overlay=overlay
	# git diff overlay/lib_contents.bzl

linux1:
	python3 add_configuration.py \
	  --pristine_dir=$(PRISTINE) \
	  --configured_name=linux_arm64 \
	  --configure_options=config_opts.txt

linux2:
	python3 /tmp/analyze.py \
	  --pristine_dir=$(PRISTINE) \
	  --configured_dir=linux_arm64 \
          --configured_name=linux_arm64 \
          --overlay=overlay
	git diff overlay/lib_contents.bzl


gen_mod:
	bazel build :openssl3.MODULE.bazel.new
	diff ../../bazel-bin/deps/openssl3/openssl3.MODULE.bazel.new openssl3.MODULE.bazel
