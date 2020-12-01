require 'spec_helper'

print `cat /etc/os-release`
print `uname -a`

describe 'successfully run stress test' do
  it 'displays PASS and returns 0' do
    # exclude stress tests; prefixed by TestStress_
    output = `sudo /tmp/security-agent/stresssuite -test.v 1>&2`
    retval = $?
    expect(retval).to eq(0)
  end
end