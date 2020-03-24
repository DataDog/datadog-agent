#
# Cookbook Name:: dd-agent-system-files-check
# Recipe:: default
#
# Copyright (C) 2020 Datadog
#
# All rights reserved - Do Not Redistribute
#

if node['platform_family'] != 'windows'
    puts "dd-agent-system-files-check: Not implemented on non-windows"
else
    ruby_block "list-after-files" do
        block do
            File.open("c:/after-files.txt", "w") do |out|
                list_files().each { |f| out.puts(f) }
            end
        end
        action :run
    end
end
