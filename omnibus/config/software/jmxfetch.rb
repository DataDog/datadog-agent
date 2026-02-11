# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name "jmxfetch"

jmxfetch_version = ENV['JMXFETCH_VERSION']
jmxfetch_hash = ENV['JMXFETCH_HASH']

if jmxfetch_version.nil? || jmxfetch_version.empty? || jmxfetch_hash.nil? || jmxfetch_hash.empty?
  raise "Please specify JMXFETCH_VERSION and JMXFETCH_HASH env vars to build."
end

default_version jmxfetch_version

skip_transitive_dependency_licensing true

jar_dir = "#{install_dir}/bin/agent/dist/jmx"

relative_path "jmxfetch"

build do
  mkdir jar_dir
  command_on_repo_root "bazelisk run -- //deps/jmxfetch:install --verbose --destdir=#{install_dir}", :live_stream => Omnibus.logger.live_stream(:info)

  # TOOD(https://datadoghq.atlassian.net/browse/ABLD-386): figure this out and fold into /deps/jmxfetch
  if osx_target? && code_signing_identity
    # Also sign binaries and libraries inside the .jar, because they're detected by the Apple notarization service.
    command "unzip #{jar_dir}/jmxfetch.jar -d ."
    delete "#{jar_dir}/jmxfetch.jar"

    if ENV['HARDENED_RUNTIME_MAC'] == 'true'
      hardened_runtime = "-o runtime --entitlements #{entitlements_file} "
    else
      hardened_runtime = ""
    end

    command "find . -type f | grep -E '(\\.so|\\.dylib|\\.jnilib)' | xargs -I{} codesign #{hardened_runtime}--force --timestamp --deep -s '#{code_signing_identity}' '{}'"
    command "zip jmxfetch.jar -r ."
    copy "jmxfetch.jar", "#{jar_dir}/jmxfetch.jar"
    block { File.chmod(0644, "#{jar_dir}/jmxfetch.jar") }
  end
end
