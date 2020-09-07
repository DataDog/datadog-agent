require 'spec_helper'

describe 'successfully run functional test' do
  it 'displays PASS and returns 0' do
    output = `sudo /tmp/security-agent/testsuite -test.v`
    retval = $?
    print output
    expect(retval).to eq(0)
    expect(output).not_to include("FAIL")
  end
end