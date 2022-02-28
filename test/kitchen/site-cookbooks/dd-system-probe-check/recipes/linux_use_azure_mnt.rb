if azure? && !platform?('windows')
  directory '/mnt/system-probe-tests' do
    owner 'root'
    group 'root'
    mode '0777'
    action :create
  end
  link '/tmp/system-probe-tests' do
    to '/mnt/system-probe-tests'
  end
end
