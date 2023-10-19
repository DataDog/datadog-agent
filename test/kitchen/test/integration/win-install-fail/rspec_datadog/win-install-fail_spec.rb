require 'spec_helper'

def check_user_exists(name)
  selectstatement = "powershell -command \"get-wmiobject -query \\\"Select * from Win32_UserAccount where Name='#{name}'\\\"\""
  outp = `#{selectstatement} 2>&1`
  outp
end
shared_examples_for 'a device with no files installed' do
  it 'has no DataDog program files directory' do
    expect(File).not_to exist("#{ENV['ProgramFiles']}\\DataDog")
  end
  it 'has no DataDog program data directory' do
    expect(File).not_to exist("#{ENV['ProgramData']}\\DataDog\\conf.d")
    expect(File).not_to exist("#{ENV['ProgramData']}\\DataDog\\checks.d")
    # Do not check that the datadog.yaml file was removed because once it's created
    # it's risky to delete it.
    # expect(File).not_to exist("#{ENV['ProgramData']}\\DataDog\\datadog.yaml")
    expect(File).not_to exist("#{ENV['ProgramData']}\\DataDog\\auth_token")
  end
end

shared_examples_for 'a device with a ddagentuser' do
  is_user = check_user_exists('ddagentuser')
  it 'has a ddagentuser account' do
    expect(is_user).not_to be_empty
  end
end

shared_examples_for 'a device without a ddagentuser' do
  is_user = check_user_exists('ddagentuser')
  it 'doesn\'t have a ddagentuser account' do
    expect(is_user).to be_empty
  end
end

describe 'dd-agent-win-install-fail' do
  it_behaves_like 'a device with no files installed'
  # The installer no longer deletes the user on uninstall and is transitioning away from managing user accounts.
  # Therefore we should instead check that it did create a ddagentuser account for now, and in the future check 
  # that it did not create a ddagentuser account.
  it_behaves_like 'a device with a ddagentuser'
end
