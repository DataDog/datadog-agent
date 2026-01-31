C2B=$(HOME)/ws/experimental/teams/agent-supply-chain/configure2bazel
PRISTINE=curl-8.18.0

next:
	echo Do both steps on each platform.
	echo Adjust overlay/overlay.BUILD.bazel
	echo Adjust overlay/lib/curl_config.h

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
	git diff overlay/lib_contents.bzl

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
