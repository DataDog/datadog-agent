


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

shared_examples_for 'a device with no dd-agent-user' do
  is_user = check_user_exists('ddagentuser')
  it 'has not dd-agent-user' do
    expect(is_user).to be_empty
  end
end
describe 'dd-agent-win-install-fail' do
  it_behaves_like 'a device with no files installed'
  it_behaves_like 'a device with no dd-agent-user'
end
