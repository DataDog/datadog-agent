BZL=bazelisk
HERE=$(shell /bin/pwd)


go:
	# touch the file to keep the aspect running
	echo '' >>gather_metadata/core.bzl
	bazel build //packages/agent/linux:debian_sbom 2>&1 | sed -e 's@^DEBUG:[^:]*/gather_metadata@DEBUG: gather_metadata@' >sbom.out
	grep === sbom.out
	bazel build //packages/agent/linux:debian_spdx
	cp bazel-macos/packages/agent/linux/debian_spdx.yml latest
	diff before/debian_spdx.yml latest

sbom1:
	@echo "Show that we do point to zlib and the license"
	bazel cquery 'somepath(//packages/agent/linux:debian_sbom, @zlib//:license)'
	bazel cquery 'somepath(//packages/agent/linux:debian_sbom, @rules_license//licenses/spdx:Zlib)'

	@echo ==========================
	@echo "Now examine the zlib license target"
	bazel cquery --output=build @zlib//:license
	@echo Note 'depset([<generated file external/+http_archive+zlib/license.package-attribute.json>]),'
	@echo in the output to show we have a package attribute rule
	bazel cquery --output=starlark --starlark:file=$(HERE)/show_rule.bzl @zlib//:license
