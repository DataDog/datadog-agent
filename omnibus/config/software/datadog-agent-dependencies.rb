name 'datadog-agent-dependencies'

description "Enforce building dependencies as soon as possible so they can be cached"

if heroku_target?
  flavor_flag = "--//packages/agent:flavor=heroku"
else
  flavor_flag = fips_mode? ? "--//packages/agent:flavor=fips" : ""
end

# Linux-specific dependencies
if linux_target?
  build do
    command_on_repo_root "bazelisk run #{flavor_flag} -- @nghttp2//:install --destdir='#{install_dir}'"
    command_on_repo_root "bazelisk run #{flavor_flag} -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
      " #{install_dir}/embedded/lib/libnghttp2.so"

    command_on_repo_root "bazelisk run #{flavor_flag} -- @curl//:install --destdir='#{install_dir}'"
    command_on_repo_root "bazelisk run #{flavor_flag} -- //bazel/rules:replace_prefix --prefix '#{install_dir}/embedded'" \
      " #{install_dir}/embedded/lib/libcurl.so" \
      " #{install_dir}/embedded/bin/curl"
  end
end

dependency 'datadog-agent-data-plane' if linux_target? && !heroku_target?

# Bundled cacerts file (is this a good idea?)
dependency 'cacerts'

# Used for memory profiling with the `status py` agent subcommand
dependency 'pympler'

dependency 'datadog-agent-integrations-py3'

build do
    command_on_repo_root "bazelisk run #{flavor_flag} -- //packages/agent/dependencies:install --destdir=#{install_dir}"
end

build do
    # Delete empty folders that can still be present when building
    # without the omnibus cache.
    # When the cache gets used, git will transparently remove empty dirs for us
    # We do this here since we are done building our dependencies, but haven't
    # started creating the agent directories, which might be empty but that we
    # still want to keep
    command "find #{install_dir} -type d -empty -delete"
end
