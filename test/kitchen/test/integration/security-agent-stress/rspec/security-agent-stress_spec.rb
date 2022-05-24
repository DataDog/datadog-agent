print `cat /etc/os-release`
print `uname -a`

print `ls /sys/devices/system/cpu`
print `cat /sys/devices/system/cpu/nohz_full`

describe 'successfully run stress test against master' do
  it 'displays PASS and returns 0' do
    output = `sudo /tmp/security-agent/stresssuite -test.v -duration 120 1>&2`
    retval = $?
    expect(retval).to eq(0)
  end
end
