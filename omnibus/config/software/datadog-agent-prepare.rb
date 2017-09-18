# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https:#www.datadoghq.com/).
# Copyright 2017 Datadog, Inc.

name "datadog-agent-prepare"
description "steps required to preprare the build"
default_version "1.0.0"

build do
  block do
    %w{embedded/lib embedded/bin bin}.each do |dir|
      dir_fullpath = File.expand_path(File.join(install_dir, dir))
      FileUtils.mkdir_p(dir_fullpath)
      FileUtils.touch(File.join(dir_fullpath, ".gitkeep"))
    end
  end
end