return unless Chef::SystemProbeHelpers::azure?(node) && !platform?('windows')

mnt_path = ::File.exist?('/mnt/resource') ? '/mnt/resource' : '/mnt'

script 'use mnt' do
  interpreter "bash"
  code <<-EOH
    mkdir -p #{mnt_path}/system-probe-tests
    chmod 0777 #{mnt_path}/system-probe-tests
    ln -s #{mnt_path}/system-probe-tests /tmp/system-probe-tests
  EOH
end
