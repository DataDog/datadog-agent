require 'spec_helper'
require 'open3'

GOLANG_TEST_FAILURE = /FAIL:/

def check_output(output, wait_thr)
  test_failures = []

  output.each_line do |line|
    puts line
    test_failures << line.strip if line =~ GOLANG_TEST_FAILURE
  end

  if test_failures.empty? && !wait_thr.value.success?
    test_failures << "Test command exited with status (#{wait_thr.value.exitstatus}) but no failures were captured."
  end

  test_failures
end

print `Powershell -C "Get-WmiObject Win32_OperatingSystem | Select Caption, OSArchitecture, Version, BuildNumber | FL"`

root_dir = "#{ENV['USERPROFILE']}\\AppData\\Local\\Temp\\kitchen\\cache\\system-probe\\tests".gsub("\\", File::SEPARATOR)
print root_dir
print Dir.entries(root_dir)

Dir.glob("#{root_dir}/**/testsuite.exe").each do |f|
  pkg = f.delete_prefix(root_dir).delete_suffix('/testsuite')
  describe "system probe tests for #{pkg}" do
    it 'successfully runs' do
      Dir.chdir(File.dirname(f)) do
        Open3.popen2e(f, "-test.v", "-test.timeout=10m", "-test.count=1") do |_, output, wait_thr|
          test_failures = check_output(output, wait_thr)
          expect(test_failures).to be_empty, test_failures.join("\n")
        end
      end
    end
  end
end
