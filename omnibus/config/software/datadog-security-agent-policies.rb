# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

require './lib/ostools.rb'
require 'json'

name 'datadog-security-agent-policies'

dependency 'datadog-agent'

relative_path 'security-agent-policies'

source git: 'https://github.com/DataDog/security-agent-policies.git'

policies_version = ENV['SECURITY_AGENT_POLICIES_VERSION']
if policies_version.nil? || policies_version.empty?
  policies_version = 'master'
end
default_version policies_version

always_build true

build do
  license "Apache-2.0"
  license_file "./LICENSE"

  compliance_dir = "#{install_dir}/etc/datadog-agent/compliance.d"
  mkdir compliance_dir

  # Copy config files for compliance
  block do

    Dir.glob("#{project_dir}/compliance/containers/*").each do |file|

      next if !File.file?(file)

      copy file, "#{compliance_dir}/"

    end

  end

  runtime_dir = "#{install_dir}/etc/datadog-agent/runtime-security.d"
  mkdir runtime_dir

  # Copy config files for runtime
  block do

    Dir.glob("#{project_dir}/runtime/*").each do |file|

      next if !File.file?(file)

      copy file, "#{runtime_dir}/"

    end

  end

end
