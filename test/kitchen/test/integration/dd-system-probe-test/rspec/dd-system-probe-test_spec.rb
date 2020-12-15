require 'spec_helper'
require 'open3'

print `cat /etc/os-release`
print `uname -a`

GOLANG_TEST_FAILURE = /FAIL:/

Dir.glob('/tmp/system-probe-tests/**/testsuite').each do |f|
  describe "system-probe tests for #{f}" do
    it 'succesfully runs' do
      Dir.chdir(File.dirname(f)) do
        Open3.popen2e("sudo", f, "-test.v") do |_, output, _|
          test_failures = []
          output.each_line do |line|
            puts "#{line}"
            test_failures << line.strip if line =~ GOLANG_TEST_FAILURE
          end

          expect(test_failures).to be_empty, test_failures.join("\n")
        end
      end
    end
  end
end
