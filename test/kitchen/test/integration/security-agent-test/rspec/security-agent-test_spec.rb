require 'spec_helper'

print `cat /etc/os-release`
print `uname -a`

Dir.glob('/tmp/security-agent/pkg/ebpf/bytecode/build/*.o').each do |f|
  FileUtils.chmod 0644, f, :verbose => true
end

describe 'successfully run functional test' do
  it 'displays PASS and returns 0' do
    output = `DD_TESTS_RUNTIME_COMPILED=1 DD_SYSTEM_PROBE_BPF_DIR=/tmp/security-agent/pkg/ebpf/bytecode/build sudo -E /tmp/security-agent/testsuite -test.v -status-metrics 1>&2`
    retval = $?
    expect(retval).to eq(0)
  end
end

if File.readlines("/etc/os-release").grep(/SUSE/).size == 0 and ! File.exists?('/etc/rhsm')
  describe 'successfully run functional test inside a container' do
    it 'displays PASS and returns 0' do
      output = `sudo docker exec -e DD_SYSTEM_PROBE_BPF_DIR=/tmp/security-agent/pkg/ebpf/bytecode/build -ti docker-testsuite /tmp/security-agent/testsuite -test.v -status-metrics -env docker 1>&2`
      retval = $?
      expect(retval).to eq(0)
    end
  end
end
