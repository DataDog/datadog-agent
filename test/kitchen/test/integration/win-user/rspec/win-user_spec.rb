require 'spec_helper'

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
  it 'has proper permissions on programdata\datadog' do
    # should have a sddl like so 
    # O:SYG:SYD:(A;ID;WD;;;BU)(A;ID;FA;;;BA)(A;ID;FA;;;SY)(A;ID;FA;;;<sid>)

    # on server 2016, it doesn't have the assigned system right, only the inherited.
    # allow either
    #expected_sddl = "O:SYG:SYD:(A;;FA;;;SY)(A;ID;WD;;;BU)(A;ID;FA;;;BA)(A;ID;FA;;;SY)(A;ID;FA;;;#{dd_user_sid})"
    expected_sddl = "O:SYG:SYD:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;WD;;;BU)(A;OICI;FA;;;#{dd_user_sid})"
    expected_sddl_2016 = "O:SYG:SYD:(A;ID;WD;;;BU)(A;ID;FA;;;BA)(A;ID;FA;;;SY)(A;ID;FA;;;#{dd_user_sid})"
    actual_sddl = get_sddl_for_object("#{ENV['ProgramData']}\\Datadog")

    expect(actual_sddl).to have_sddl_equal_to(expected_sddl)
                       .or have_sddl_equal_to(expected_sddl_2016)
  end
  it 'has proper permissions on datadog.yaml' do
    # should have a sddl like so 
    # O:SYG:SYD:(A;ID;WD;;;BU)(A;ID;FA;;;BA)(A;ID;FA;;;SY)(A;ID;FA;;;<sid>)

    # on server 2016, it doesn't have the assigned system right, only the inherited.
    # allow either
    #expected_sddl =   "O:SYG:SYD:(A;;FA;;;SY)(A;ID;WD;;;BU)(A;ID;FA;;;BA)(A;ID;FA;;;SY)(A;ID;FA;;;#{dd_user_sid})"
    expected_sddl = "O:SYG:SYD:PAI(A;;FA;;;SY)(A;;FA;;;BA)(A;;WD;;;BU)(A;;FA;;;#{dd_user_sid})"
    expected_sddl_2016 = "O:SYG:SYD:(A;ID;WD;;;BU)(A;ID;FA;;;BA)(A;ID;FA;;;SY)(A;ID;FA;;;#{dd_user_sid})"
    actual_sddl = get_sddl_for_object("#{ENV['ProgramData']}\\Datadog\\datadog.yaml")

    expect(actual_sddl).to have_sddl_equal_to(expected_sddl)
                       .or have_sddl_equal_to(expected_sddl_2016)
  end
  it 'has proper permissions on the conf.d directory' do
    # A,OICI;FA;;;SY = Allows Object Inheritance (OI) container inherit (CI); File All Access to LocalSystem
    # A,OICIID;WD;;;BU = Allows OI, CI, this is an inherited ACE (ID), change permissions (WD), to built-in users
    # A,OICIID;FA;;;BA = Allow OI, CI, ID, File All Access (FA) to Builtin Administrators
    # A,OICIID;FA;;;SY = Inherited right of OI, CI, (FA) to LocalSystem
    # A,OICIID;FA;;;dd_user_sid = explicit right assignment of OI, CI, FA to the dd-agent user, inherited from the parent

    expected_sddl =      "O:SYG:SYD:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;WD;;;BU)(A;OICI;FA;;;#{dd_user_sid})"
    actual_sddl = get_sddl_for_object("#{ENV['ProgramData']}\\Datadog\\conf.d")

    expect(actual_sddl).to have_sddl_equal_to(expected_sddl)
  end

  it 'has the proper permissions on the DataDog registry key' do
    # A;;KA;;;SY  = Allows KA (KeyAllAccess) to local system
    # A;;KA;;;BA  = Allows KA (KeyAllAccess) to BA builtin administrators
    # A;;KA;; <dd_user_sid> allows KEY_ALL_ACCESS to the dd agent user
    # A;OICIIO; Object Inherit AC, container inherit ace, Inherit only ace
    #  CCDCLCSWRPWPSDRCWDWOGA CC = SDDL Create Child
    #                         DC = SDDL Delete Child
    #                         LC = Listchildrent
    #                         SW = self write
    #                         RP = read property
    #                         WP = write property
    #                         SD = standard delete
    #                         RC = read control
    #                         WD = WRITE DAC
    #                         WO = Write owner
    #                         GA = Generic All
    #    for dd-agent-user
    # A;CIID;KR;;;BU  = Allow Container Inherit/inherited ace KeyRead to BU (builtin users)
    # A;CIID;KA;;;BA  =                                       KeyAllAccess  (builtin admins)
    # A;CIID;KA;;;SY  =                                       Keyallaccess  (local system)
    # A;CIIOID;KA;;;CO= container inherit, inherit only, inherited ace, keyallAccess, to creator/owner
    # A;CIID;KR;;;AC = allow container inherit/inherited ace  Key Read to AC ()
    expected_sddl =           "O:SYG:SYD:AI(A;;KA;;;SY)(A;;KA;;;BA)(A;;KA;;;#{dd_user_sid})(A;OICIIO;CCDCLCSWRPWPSDRCWDWOGA;;;#{dd_user_sid})(A;CIID;KR;;;BU)(A;CIID;KA;;;BA)(A;CIID;KA;;;SY)(A;CIIOID;KA;;;CO)(A;CIID;KR;;;AC)"
    expected_sddl_2008 =      "O:SYG:SYD:AI(A;;KA;;;SY)(A;;KA;;;BA)(A;;KA;;;#{dd_user_sid})(A;OICIIO;CCDCLCSWRPWPSDRCWDWOGA;;;#{dd_user_sid})(A;ID;KR;;;BU)(A;CIIOID;GR;;;BU)(A;ID;KA;;;BA)(A;CIIOID;GA;;;BA)(A;ID;KA;;;SY)(A;CIIOID;GA;;;SY)(A;CIIOID;GA;;;CO)"
    expected_sddl_with_edge = "O:SYG:SYD:AI(A;;KA;;;SY)(A;;KA;;;BA)(A;;KA;;;#{dd_user_sid})(A;OICIIO;CCDCLCSWRPWPSDRCWDWOGA;;;#{dd_user_sid})(A;CIID;KR;;;BU)(A;CIID;KA;;;BA)(A;CIID;KA;;;SY)(A;CIIOID;KA;;;CO)(A;CIID;KR;;;AC)(A;CIID;KR;;;S-1-15-3-1024-1065365936-1281604716-3511738428-1654721687-432734479-3232135806-4053264122-3456934681)"
    
    ## sigh.  M$ added a mystery sid some time back, that Edge/IE use for sandboxing,
    ## and it's an inherited ace.  Allow that one, too

    actual_sddl = get_sddl_for_object("HKLM:Software\\Datadog\\Datadog Agent")

    expect(actual_sddl).to have_sddl_equal_to(expected_sddl)
                       .or have_sddl_equal_to(expected_sddl_2008)
                       .or have_sddl_equal_to(expected_sddl_with_edge)
  end

  it 'has agent.exe running as ddagentuser' do
    uname = get_username_from_tasklist("agent.exe")
    expect(get_username_from_tasklist("agent.exe")).to eq("ddagentuser")
  end
  it 'has trace agent running as ddagentuser' do
    expect(get_username_from_tasklist("trace-agent.exe")).to eq("ddagentuser")
  end
  it 'has process agent running as local_system' do
    expect(get_username_from_tasklist("process-agent.exe")).to eq("SYSTEM")
  end
  secdata = get_security_settings
  it 'has proper security rights assigned' do
    expect(check_has_security_right(secdata, "SeDenyInteractiveLogonRight", "ddagentuser")).to be_truthy
    expect(check_has_security_right(secdata, "SeDenyNetworkLogonRight", "ddagentuser")).to be_truthy
    expect(check_has_security_right(secdata, "SeDenyRemoteInteractiveLogonRight", "ddagentuser")).to be_truthy
  end
  it 'is in proper groups' do
    expect(check_is_user_in_group("ddagentuser", "Performance Monitor Users")).to be_truthy
  end
end
describe 'dd-agent-win-user' do
#  it_behaves_like 'an installed Agent'
  it_behaves_like 'a running Agent with no errors'
  it_behaves_like 'an Agent with APM enabled'
  it_behaves_like 'an Agent with process enabled'
  it_behaves_like 'an Agent with valid permissions'
end
  