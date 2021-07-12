require 'spec_helper'

print `cat /etc/os-release`
print `uname -a`

describe 'successfully run functional test' do
  it 'displays PASS and returns 0' do
    output = `sudo /tmp/security-agent/testsuite -test.v 1>&2`
    retval = $?
    expect(retval).to eq(0)
  end
end

# describe 'successfully run functional test in 32 bits' do
#   it 'displays PASS and returns 0' do
#     output = `sudo /tmp/security-agent/testsuite32 -test.v 2>&1`
#     print output
#     retval = $?
#     expect(retval).to eq(0)
#     expect(output).not_to include("FAIL")
#   end
# end

if File.readlines("/etc/os-release").grep(/SUSE/).size == 0 and ! File.exists?('/etc/rhsm')
  describe 'successfully run functional test inside a container' do
    it 'displays PASS and returns 0' do
      output = `sudo docker exec -ti docker-testsuite /tmp/security-agent/testsuite -test.v --env docker 1>&2`
      retval = $?
      expect(retval).to eq(0)
    end
  end
end
