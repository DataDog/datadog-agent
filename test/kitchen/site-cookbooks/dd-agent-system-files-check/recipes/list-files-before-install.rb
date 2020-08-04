#
# Cookbook Name:: dd-agent-system-files-check
# Recipe:: default
#
# Copyright (C) 2020 Datadog
#
# All rights reserved - Do Not Redistribute
#
require 'find'

if node['platform_family'] != 'windows'
    puts "dd-agent-system-files-check: Not implemented on non-windows"
else
    ruby_block "list-before-files" do
        block do
            # Windows update is likely to change lots of files, disable it.
            # It's okay to do this because this should run on an ephemereal VM.
            system("sc.exe config wuauserv start=disabled")
            system("sc.exe stop wuauserv")

            File.open("c:/before-files.txt", "w") do |out|
                Find.find('c:/windows/').each { |f| out.puts(f) }
            end
        end
        action :run
    end
end
