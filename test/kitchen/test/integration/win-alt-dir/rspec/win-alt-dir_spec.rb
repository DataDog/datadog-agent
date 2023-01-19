require 'spec_helper'
  

def check_user_exists(name)
  selectstatement = "powershell -command \"get-wmiobject -query \\\"Select * from Win32_UserAccount where Name='#{name}'\\\"\""
  outp = `#{selectstatement} 2>&1`
  outp
end

shared_examples_for 'a correctly created configuration root' do
  # We retrieve the value defined in kitchen.yml because there is no simple way
  # to set env variables on the target machine or via parameters in Kitchen/Busser
  # See https://github.com/test-kitchen/test-kitchen/issues/662 for reference
  let(:configuration_path) {
    dna_json_path = "#{ENV['USERPROFILE']}\\AppData\\Local\\Temp\\kitchen\\dna.json"
    JSON.parse(IO.read(dna_json_path)).fetch('dd-agent-rspec').fetch('APPLICATIONDATADIRECTORY')
  }
  it 'has the proper configuration root' do
    expect(File).not_to exist("#{ENV['ProgramData']}\\DataDog")
    expect(File).to exist("#{configuration_path}")
  end
end

shared_examples_for 'a correctly created binary root' do
  # We retrieve the value defined in kitchen.yml because there is no simple way
  # to set env variables on the target machine or via parameters in Kitchen/Busser
  # See https://github.com/test-kitchen/test-kitchen/issues/662 for reference
  let(:binary_path) {
    dna_json_path = "#{ENV['USERPROFILE']}\\AppData\\Local\\Temp\\kitchen\\dna.json"
    JSON.parse(IO.read(dna_json_path)).fetch('dd-agent-rspec').fetch('PROJECTLOCATION')
  }
  it 'has the proper binary root' do
    expect(File).not_to exist("#{ENV['ProgramFiles']}\\DataDog")
    expect(File).to exist("#{binary_path}")
  end
end

shared_examples_for 'an Agent with valid permissions' do
  let(:configuration_path) {
    dna_json_path = "#{ENV['USERPROFILE']}\\AppData\\Local\\Temp\\kitchen\\dna.json"
    JSON.parse(IO.read(dna_json_path)).fetch('dd-agent-rspec').fetch('APPLICATIONDATADIRECTORY')
  }
  let(:binary_path) {
    dna_json_path = "#{ENV['USERPROFILE']}\\AppData\\Local\\Temp\\kitchen\\dna.json"
    JSON.parse(IO.read(dna_json_path)).fetch('dd-agent-rspec').fetch('PROJECTLOCATION')
  }
  dd_user_sid = get_user_sid('ddagentuser')
  #datadog_yaml_sddl = get_sddl_for_object("c:\\programdata\\datadog\\datadog.yaml")
  it 'has proper permissions on programdata\datadog' do
    expected_sddl = "O:SYG:SYD:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;WD;;;BU)(A;OICI;FA;;;#{dd_user_sid})"
    expected_sddl_2016 = "O:SYG:SYD:(A;ID;WD;;;BU)(A;ID;FA;;;BA)(A;ID;FA;;;SY)(A;ID;FA;;;#{dd_user_sid})"
    actual_sddl = get_sddl_for_object(configuration_path)
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
    actual_sddl = get_sddl_for_object("#{configuration_path}\\datadog.yaml")
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
    actual_sddl = get_sddl_for_object("#{configuration_path}\\conf.d")

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

describe 'dd-agent-install-alternate-dir' do
  it_behaves_like 'a correctly created configuration root'
  it_behaves_like 'a correctly created binary root'
  it_behaves_like 'an Agent with valid permissions'
end
  
