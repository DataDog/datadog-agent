require 'fileutils'
require 'kernel_out_spec_helper'
require 'open3'

GOLANG_TEST_FAILURE = /FAIL:/

def check_output(output, wait_thr)
  test_failures = []

  output.each_line do |line|
    puts KernelOut.format(line.strip)
    test_failures << KernelOut.format(line.strip) if line =~ GOLANG_TEST_FAILURE
  end

  if test_failures.empty? && !wait_thr.value.success?
    test_failures << KernelOut.format("Test command exited with status (#{wait_thr.value.exitstatus}) but no failures were captured.")
  end

  test_failures
end

print KernelOut.format(`cat /etc/os-release`)
print KernelOut.format(`uname -a`)

Dir.glob('/tmp/security-agent/pkg/ebpf/bytecode/build/*.o').each do |f|
  FileUtils.chmod 0644, f, :verbose => true
end

describe 'functional test running directly on host' do
  it 'successfully runs' do
    Open3.popen2e({"DD_TESTS_RUNTIME_COMPILED"=>"1", "DD_SYSTEM_PROBE_BPF_DIR"=>"/tmp/security-agent/pkg/ebpf/bytecode/build"}, "sudo", "-E", "/tmp/security-agent/testsuite", "-test.v", "-status-metrics") do |_, output, wait_thr|
      test_failures = check_output(output, wait_thr)
      expect(test_failures).to be_empty, test_failures.join("\n")
    end
  end
end

if File.readlines("/etc/os-release").grep(/SUSE/).size == 0 and !File.exists?('/etc/rhsm')
  describe 'functional test running inside a container' do
    it 'successfully runs' do
      Open3.popen2e("sudo", "docker", "exec", "-e", "DD_SYSTEM_PROBE_BPF_DIR=/tmp/security-agent/pkg/ebpf/bytecode/build", "docker-testsuite", "/tmp/security-agent/testsuite", "-test.v", "-status-metrics", "--env", "docker") do |_, output, wait_thr|
        test_failures = check_output(output, wait_thr)
        expect(test_failures).to be_empty, test_failures.join("\n")
      end
    end
  end
end
