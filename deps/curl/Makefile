# Hints for how to update curl from first principles
#
# This is not part of any build process. Think of it as a readme with
# executable parts.

# A tool to look at configure output and create parts to support bazel targets
C2B=$(HOME)/ws/experimental/teams/agent-supply-chain/configure2bazel

PRISTINE=curl-8.18.0

next:
	echo Do both steps on each platform.
	echo Adjust overlay/overlay.BUILD.bazel
	echo Adjust overlay/lib/curl_config.h

# Run configure and grab the generated files to the overlay tree
# Run make to create the library outputs
macos_step1:
	python3 $(C2B)/add_configuration.py \
	  --pristine_dir=$(PRISTINE) \
	  --configure_options=config_opts.txt
	echo Now do: '(cd linux_aarch64 ; make)'
	echo Now do: '(cd darwin_arm64 ; make)'


# Step 3
# Do some reasoning about the generated config.h.  For almost every project,
# the file should be identical for x86 and arm within each OS.
# Then look at config.h differences per OS.  They should almost always be
# very simliar. What might differ is the inclusion of packages which are
# os specific. Basic stuff should be in both.
# Look for HAVE_* settings which are wrong because configure can not find
# the bazel built dependency.  This happens often. Edit config.h as needed
# to match the goal you want, rather than what configure finds.

# Analyze the built libraries to map library content back to the source files
# in them. Save lists  by library name and arch back to lib_contents.bzl
macos_step3:
	python3 $(C2B)/analyze.py \
	  --pristine_dir=$(PRISTINE) \
	  --configured_dir=darwin_arm64 \
          --configured_name=darwin_arm64 \
          --overlay=overlay
	git diff overlay/lib_contents.bzl

# For linux, do the same as macos, but with different config names.

# Step 4
# Create your cc_library() target.  Use any of the other projects using
# configure2bazel as an sample starting point.
