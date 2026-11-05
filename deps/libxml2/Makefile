TOOLS=/Users/tony.aiuto/ws/experimental/teams/agent-supply-chain/configure2bazel
PRISTINE=libxml2-2.14.5

macos_conf:
	python3 $(TOOLS)/add_configuration.py --pristine_dir=$(PRISTINE) \
	  --configure_options=config_opts.txt \
	  --configured_name=macos
	cd macos ; make >make.log

macos_pull:
	python3 $(TOOLS)/analyze.py --pristine_dir=$(PRISTINE) --overlay_dir=overlay \
	  --configured_dir=macos --configured_name=macos

linux_conf:
	python3 ../../add_configuration.py \
	  --pristine_dir=$(PRISTINE) \
	  --configure_options=config_opts.txt \
	  --configured_name=linux
	cd linux ; make >make.log


fake_libs:
	bazel run -- @xz//:install --destdir='/tmp/opt/datadog/embedded'
	bazel run -- @zlib//:install --destdir='/tmp/opt/datadog/embedded'
