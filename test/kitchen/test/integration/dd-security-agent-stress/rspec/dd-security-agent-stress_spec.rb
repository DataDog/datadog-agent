require 'spec_helper'

print `cat /etc/os-release`
print `uname -a`

describe 'successfully run stress test against master' do
  # uncomment following lines once merged
  #it 'displays PASS and returns 0' do
  #  output = `sudo /tmp/security-agent/stresssuite-master -test.v --report-file /tmp/report.json 1>&2`
  #  retval = $?
  #  expect(retval).to eq(0)
  #end

  it 'displays PASS and returns 0' do
    output = `sudo /tmp/security-agent/stresssuite -test.v --diff-base /tmp/report.json 1>&2`
    retval = $?
    expect(retval).to eq(0)
  end
end
