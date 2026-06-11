name 'datadog-agent-dependencies'

description "Enforce building dependencies as soon as possible so they can be cached"

if heroku_target?
  flavor_flag = "--//packages/agent:flavor=heroku"
else
  flavor_flag = fips_mode? ? "--//packages/agent:flavor=fips" : ""
end

adp_artifact_hash_available = arm_target? ? !ENV['AGENT_DATA_PLANE_HASH_DARWIN_ARM64'].to_s.empty? : !ENV['AGENT_DATA_PLANE_HASH_DARWIN_AMD64'].to_s.empty?
include_adp = linux_target? || (osx_target? && adp_artifact_hash_available)

dependency 'datadog-agent-data-plane' if include_adp && !heroku_target?

dependency 'datadog-agent-integrations-py3'

build do
    command_on_repo_root "bazelisk run --//:install_dir=#{install_dir} #{flavor_flag} -- //packages/agent/dependencies:install --destdir=#{install_dir}",
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
