# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

name "datadog-agent-prepare"
description "steps required to preprare the build"
default_version "1.0.0"

skip_transitive_dependency_licensing true

build do
  license :project_license

  block do
    %w{embedded/lib embedded/bin embedded/etc bin}.each do |dir|
      dir_fullpath = File.expand_path(File.join(install_dir, dir))
      FileUtils.mkdir_p(dir_fullpath)
      FileUtils.touch(File.join(dir_fullpath, ".gitkeep"))
    end

    # Add a README for the embedded environment's configuration folder
    File.open(File.expand_path(File.join(install_dir, "/embedded/etc/README.md")), "w") do |f|
      f.puts <<~EOF
          # Embedded environment configuration folder

          This folder contains configuration files for the Agent embedded environment.

      EOF
    end
  end
end

if windows_target?
  build do
    block do
      FileUtils.mkdir_p(File.expand_path(File.join(Omnibus::Config.source_dir(), "datadog-agent", "src", "github.com", "DataDog", "datadog-agent", "bin", "agent")))
      FileUtils.mkdir_p(File.expand_path(File.join(install_dir, "bin", "agent")))

    end
  end
end
