require 'spec_helper'
require 'open3'

print `cat /etc/os-release`
print `uname -a`

GOLANG_TEST_FAILURE = /FAIL:/

Dir.glob('/tmp/system-probe-tests/**/testsuite').each do |f|
  describe "system-probe tests for #{f}" do
    it 'succesfully runs' do
      Dir.chdir(File.dirname(f)) do
        Open3.popen2e("sudo", f, "-test.v") do |_, output, wait_thr|
          test_failures = []

          output.each_line do |line|
            puts line
            test_failures << line.strip if line =~ GOLANG_TEST_FAILURE
          end

          if test_failures.empty? && !wait_thr.value.success?
            test_failures << "Test command exited with status (#{wait_thr.value.exitstatus}) but no failures were captured."
          end

          expect(test_failures).to be_empty, test_failures.join("\n")
        end
      end
    end
  end
end
