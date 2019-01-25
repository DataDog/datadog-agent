require 'spec_helper'

def read_conf_file
    conf_path = ""
    if os == :windows
      conf_path = "#{ENV['ProgramData']}\\Datadog\\datadog.yaml"
    else
      conf_path = '/etc/datadog-agent/datadog.yaml'
    end
    f = File.read(conf_path)
    confYaml = YAML.load(f)
    confYaml
end

def get_user_sid(uname)
  output = `powershell -command "(New-Object System.Security.Principal.NTAccount('#{uname}')).Translate([System.Security.Principal.SecurityIdentifier]).value"`.strip
  output
end 

def get_sddl_for_object(name) 
  outp = `powershell -command "get-acl -Path \"#{name}\" | format-list -Property sddl"`.gsub("\n", "").gsub(" ", "")
  sddl = outp.gsub("/\s+/", "").split(":").drop(1).join(":").strip
  sddl
end

def equal_sddl?(left, right)
  # First, split the sddl into the ownership (user and group), and the dacl
  left_array = left.split("D:")
  right_array = right.split("D:")

  # compare the ownership & group.  Must be the same
  if left_array[0] != right_array[0]
    return false
  end
  left_dacl = left_array[1].scan(/(\([^)]*\))/)
  right_dacl = right_array[1].scan(/(\([^)]*\))/)

  
  # if they're different lengths, they're different
  if left_dacl.length != right_dacl.length
    return false
  end

  ## now need to break up the DACL list, because they may be listed in different
  ## orders... the order doesn't matter but the components should be the same.  So..

  left_dacl.each do |left_entry|
    found = false
    right_dacl.each do |right_entry|
      if left_entry == right_entry
        found = true
        right_dacl.delete(right_entry)
        break
      end
    end
    if !found
      return false
    end
  end
  return false if right_dacl.length != 0
  return true
end

def get_username_from_tasklist(exename)
  # output of tasklist command is
  # Image Name  PID  Session Name  Session#  Mem Usage Status  User Name  CPU Time  Window Title
  output = `tasklist /v /fi "imagename eq #{exename}" /nh`.gsub("\n", "").gsub("NT AUTHORITY", "NT_AUTHORITY")

  # for the above, the system user comes out as "NT AUTHORITY\System", which confuses the split 
  # below.  So special case it, and get rid of the space

  #username is fully qualified <domain>\username
  uname = output.split(' ')[7].partition('\\').last
  uname
end

=begin
Each SDDL is defined as follows (split across multiple lines here for readability, but they're
all concatenated into one)
O:owner_sid
G:group_sid
D:dacl_flags(string_ace1)(string_ace2)... (string_acen)
S:sacl_flags(string_ace1)(string_ace2)... (string_acen)

Well known SID strings we're interested in 
SY = LOCAL_SYSTEM
BU = Builtin Users
BA = Builtin Administrators 

So, the string O:SYG:SY indicates owner sid is LOCAL_SYSTEM group sid is SYSTEM
Then, D: indicates what comes after is the DACL, which is a list of ACE strings

Ace strings are defined as
ace_type;ace_flags;rights;object_guid;inherit_object_guid;account_sid;(resource_attribute)

Ace types include
A = Allowed
D = Denied

Ace flags 
ID = this ace inherited from parent

rights
GA = Generic All
FA = File All access
FR = File Read
FW = File Write
WD = Write DAC (change permissions)

Putting it all together, the sddl that we expect for Datadog.yaml is
O:SYG:SYD:(A;;FA;;;SY)(A;ID;WD;;;BU)(A;ID;FA;;;BA)(A;ID;FA;;;SY)(A;ID;FA;;;#{dd_user_sid})

Owner: Local System
Group: Local System

A;;FA;;;SY grants File All Access to Local System
A;ID;WD;;;BU grants members of the builtin users group Change Permissions; this ACE is inherited
A;ID;FA;;;BA grants Fila All Access to Builtin Administrators; this ACE was inherited from the parent
A;ID;FA;;;SY grants LocalSystem file AllAccess
A;ID;FA;;;#{dd_user_id} grants the ddagentuser FileAllAccess, this ACE is inherited from the parent
=end

shared_examples_for 'an Agent with valid permissions' do
  dd_user_sid = get_user_sid('ddagentuser')
  #datadog_yaml_sddl = get_sddl_for_object("c:\\programdata\\datadog\\datadog.yaml")
  it 'has proper permissions on datadog.yaml' do
    # should have a sddl like so 
    # O:SYG:SYD:(A;ID;WD;;;BU)(A;ID;FA;;;BA)(A;ID;FA;;;SY)(A;ID;FA;;;<sid>)

    # on server 2016, it doesn't have the assigned system right, only the inherited.
    # allow either
    expected_sddl = "O:SYG:SYD:(A;;FA;;;SY)(A;ID;WD;;;BU)(A;ID;FA;;;BA)(A;ID;FA;;;SY)(A;ID;FA;;;#{dd_user_sid})"
    expected_sddl_2016 = "O:SYG:SYD:(A;ID;WD;;;BU)(A;ID;FA;;;BA)(A;ID;FA;;;SY)(A;ID;FA;;;#{dd_user_sid})"
    actual_sddl = get_sddl_for_object("#{ENV['ProgramData']}\\Datadog\\datadog.yaml")
    equal_base = equal_sddl?(expected_sddl, actual_sddl)
    equal_2016 = equal_sddl?(expected_sddl_2016, actual_sddl)
    expect(equal_base | equal_2016).to be_truthy
  end
  it 'has proper permissions on the conf.d directory' do
    # A,OICI;FA;;;SY = Allows Object Inheritance (OI) container inherit (CI); File All Access to LocalSystem
    # A,OICIID;WD;;;BU = Allows OI, CI, this is an inherited ACE (ID), change permissions (WD), to built-in users
    # A,OICIID;FA;;;BA = Allow OI, CI, ID, File All Access (FA) to Builtin Administrators
    # A,OICIID;FA;;;SY = Inherited right of OI, CI, (FA) to LocalSystem
    # A,OICIID;FA;;;dd_user_sid = explicit right assignment of OI, CI, FA to the dd-agent user, inherited from the parent
    expected_sddl = "O:SYG:SYD:(A;OICI;FA;;;SY)(A;OICIID;WD;;;BU)(A;OICIID;FA;;;BA)(A;OICIID;FA;;;SY)(A;OICIID;FA;;;#{dd_user_sid})"
    actual_siddl = get_sddl_for_object("#{ENV['ProgramData']}\\Datadog\\conf.d")
  end
  it 'has agent.exe running as ddagentuser' do
    uname = get_username_from_tasklist("agent.exe")
    puts "uftl #{uname}"
    expect(get_username_from_tasklist("agent.exe")).to eq("ddagentuser")
  end
  it 'has trace agent running as ddagentuser' do
    expect(get_username_from_tasklist("trace-agent.exe")).to eq("ddagentuser")
  end
  it 'has process agent running as local_system' do
    expect(get_username_from_tasklist("process-agent.exe")).to eq("SYSTEM")
  end

end
describe 'dd-agent-user-win' do
#  it_behaves_like 'an installed Agent'
  it_behaves_like 'a running Agent with no errors'
  it_behaves_like 'an Agent with APM enabled'
  it_behaves_like 'an Agent with process enabled'
  it_behaves_like 'an Agent with valid permissions'
end
  