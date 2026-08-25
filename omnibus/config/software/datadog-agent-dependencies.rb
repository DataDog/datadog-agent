name 'datadog-agent-dependencies'

description "Enforce building dependencies as soon as possible so they can be cached"

dependency 'datadog-agent-integrations-py3'

build do
    command "bazel run #{omnibazel_flags} -- //packages/agent/dependencies:install --destdir=#{install_dir}",
        :live_stream => Omnibus.logger.live_stream(:info)

    if linux_target? && !fips_mode? && !heroku_target?
        python = "#{install_dir}/embedded/bin/python3"
        site_packages_path = "#{install_dir}/embedded/lib/python3.13/site-packages"
        command_on_repo_root "#{python} -B tasks/libs/package/auditwheel.py #{site_packages_path} #{install_dir}/embedded/lib"
    end
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
