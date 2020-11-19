require 'spec_helper'

print `cat /etc/os-release`
print `uname -a`

Dir.glob('/tmp/system-probe-tests/**/testsuite').each do |f|
  describe "system-probe tests for #{f}" do
    it 'succesfully runs' do
      Dir.chdir(File.dirname(f)) do
        `sudo #{f} -test.v 1>&2`
        retval = $?
        expect(retval).to eq(0)
      end
    end
  end
end
