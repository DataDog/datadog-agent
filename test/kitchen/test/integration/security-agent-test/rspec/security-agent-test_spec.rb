require 'spec_helper'

print `cat /etc/os-release`
print `ls /etc/zypp/repos.d/`
print `cat /etc/zypp/repos.d/*`

print `ls /etc/yum/vars`
print `cat /etc/yum/vars/*`

print `ls /etc/dnf/vars`
print `cat /etc/dnf/vars/*`

print `ls /etc/zypp/vars.d`
print `cat /etc/zypp/vars.d/*`

print `uname -a`
print `uname -r`

describe 'successfully run functional test' do
  it 'displays PASS and returns 0' do
    output = `sudo /tmp/security-agent/testsuite -test.v -status-metrics 1>&2`
    retval = $?
    expect(retval).to eq(0)
  end
end

if File.readlines("/etc/os-release").grep(/SUSE/).size == 0 and ! File.exists?('/etc/rhsm')
  describe 'successfully run functional test inside a container' do
    it 'displays PASS and returns 0' do
      output = `sudo docker exec -ti docker-testsuite /tmp/security-agent/testsuite -test.v -status-metrics --env docker 1>&2`
      retval = $?
      expect(retval).to eq(0)
    end
  end
end
