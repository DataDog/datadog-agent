name 'datadog-agent-dependencies'

description "Enforce building dependencies as soon as possible so they can be cached"

dependency 'datadog-agent-data-plane' if (linux_target? || osx_target? || windows_target?) && !heroku_target?

dependency 'datadog-agent-integrations-py3'

build do
    command "bazel run #{omnibazel_flags} -- //packages/agent/dependencies:install --destdir=#{install_dir}",
        :live_stream => Omnibus.logger.live_stream(:info)
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
