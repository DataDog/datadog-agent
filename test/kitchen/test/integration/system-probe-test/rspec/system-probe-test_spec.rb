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

##
## The main chef recipe (test\kitchen\site-cookbooks\dd-system-probe-check\recipes\default.rb)
## copies the necessary files (including the precompiled object files), and sets the mode to
## 0755, which causes the test to fail.  The object files are not being built during the
## test, anyway, so set them to the expected value
##
Dir.glob('/tmp/system-probe-tests/pkg/ebpf/bytecode/build/*.o').each do |f|
  FileUtils.chmod 0644, f, :verbose => true
end
Dir.glob('/tmp/system-probe-tests/pkg/ebpf/bytecode/build/co-re/*.o').each do |f|
  FileUtils.chmod 0644, f, :verbose => true
end

Dir.glob('/tmp/system-probe-tests/**/testsuite').each do |f|
  pkg = f.delete_prefix('/tmp/system-probe-tests').delete_suffix('/testsuite')
  describe "prebuilt system-probe tests for #{pkg}" do
    it 'successfully runs' do
      Dir.chdir(File.dirname(f)) do
        Open3.popen2e({"DD_SYSTEM_PROBE_BPF_DIR"=>"/tmp/system-probe-tests/pkg/ebpf/bytecode/build"}, "sudo", "-E", f, "-test.v", "-test.count=1") do |_, output, wait_thr|
          test_failures = check_output(output, wait_thr)
          expect(test_failures).to be_empty, test_failures.join("\n")
        end
      end
    end
  end

  describe "runtime compiled system-probe tests for #{pkg}" do
    it 'successfully runs' do
      Dir.chdir(File.dirname(f)) do
        Open3.popen2e({"DD_TESTS_RUNTIME_COMPILED"=>"1", "DD_SYSTEM_PROBE_BPF_DIR"=>"/tmp/system-probe-tests/pkg/ebpf/bytecode/build"}, "sudo", "-E", f, "-test.v", "-test.count=1") do |_, output, wait_thr|
          test_failures = check_output(output, wait_thr)
          expect(test_failures).to be_empty, test_failures.join("\n")
        end
      end
    end
  end

  describe "CO-RE system-probe tests for #{pkg}" do
    it 'successfully runs' do
      Dir.chdir(File.dirname(f)) do
        Open3.popen2e({"DD_TESTS_CO_RE"=>"1", "DD_SYSTEM_PROBE_BPF_DIR"=>"/tmp/system-probe-tests/pkg/ebpf/bytecode/build"}, "sudo", "-E", f, "-test.v", "-test.count=1") do |_, output, wait_thr|
          test_failures = check_output(output, wait_thr)
          expect(test_failures).to be_empty, test_failures.join("\n")
        end
      end
    end
  end
end
