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
    if os == :windows
      dna_json_path = "#{ENV['USERPROFILE']}\\AppData\\Local\\Temp\\kitchen\\dna.json"
    else
      dna_json_path = "/tmp/kitchen/dna.json"
    end
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
    if os == :windows
      dna_json_path = "#{ENV['USERPROFILE']}\\AppData\\Local\\Temp\\kitchen\\dna.json"
    else
      dna_json_path = "/tmp/kitchen/dna.json"
    end
    JSON.parse(IO.read(dna_json_path)).fetch('dd-agent-rspec').fetch('PROJECTLOCATION')
  }
  it 'has the proper binary root' do
    expect(File).not_to exist("#{ENV['ProgramFiles']}\\DataDog")
    expect(File).to exist("#{binary_path}")
  end
end

shared_examples_for 'an Agent with valid permissions' do
  let(:configuration_path) {
    if os == :windows
      dna_json_path = "#{ENV['USERPROFILE']}\\AppData\\Local\\Temp\\kitchen\\dna.json"
    else
      dna_json_path = "/tmp/kitchen/dna.json"
    end
    JSON.parse(IO.read(dna_json_path)).fetch('dd-agent-rspec').fetch('APPLICATIONDATADIRECTORY')
  }
  let(:binary_path) {
    if os == :windows
      dna_json_path = "#{ENV['USERPROFILE']}\\AppData\\Local\\Temp\\kitchen\\dna.json"
    else
      dna_json_path = "/tmp/kitchen/dna.json"
    end
    JSON.parse(IO.read(dna_json_path)).fetch('dd-agent-rspec').fetch('PROJECTLOCATION')
  }
  datadogagent_sid = get_service_sid('datadogagent')
  traceagent_sid = get_service_sid('datadog-trace-agent')
  #datadog_yaml_sddl = get_sddl_for_object("c:\\programdata\\datadog\\datadog.yaml")
  it 'has proper permissions on programdata\datadog' do
    expected_sddl = "O:SYG:SYD:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;WD;;;BU)(A;OICI;FA;;;#{datadogagent_sid})(A;OICI;FA;;;#{traceagent_sid})"
    expected_sddl_2016 = "O:SYG:SYD:(A;ID;WD;;;BU)(A;ID;FA;;;BA)(A;ID;FA;;;SY)(A;ID;FA;;;#{datadogagent_sid})(A;ID;FA;;;#{traceagent_sid})"

    actual_sddl = get_sddl_for_object(configuration_path)
    equal_base = equal_sddl?(expected_sddl, actual_sddl)
    equal_2016 = equal_sddl?(expected_sddl_2016, actual_sddl)
    expect(equal_base | equal_2016).to be_truthy
  end
  it 'has proper permissions on datadog.yaml' do
    # should have a sddl like so 
    # O:SYG:SYD:(A;ID;WD;;;BU)(A;ID;FA;;;BA)(A;ID;FA;;;SY)(A;ID;FA;;;<sid>)

    # on server 2016, it doesn't have the assigned system right, only the inherited.
    # allow either
    #expected_sddl =   "O:SYG:SYD:(A;;FA;;;SY)(A;ID;WD;;;BU)(A;ID;FA;;;BA)(A;ID;FA;;;SY)(A;ID;FA;;;#{dd_user_sid})"
    expected_sddl = "O:SYG:SYD:PAI(A;;FA;;;SY)(A;;FA;;;BA)(A;;WD;;;BU)(A;;FA;;;#{datadogagent_sid})(A;;FA;;;#{traceagent_sid})"
    expected_sddl_2016 = "O:SYG:SYD:(A;ID;WD;;;BU)(A;ID;FA;;;BA)(A;ID;FA;;;SY)(A;ID;FA;;;#{datadogagent_sid})(A;ID;FA;;;#{traceagent_sid})"
    
    actual_sddl = get_sddl_for_object("#{configuration_path}\\datadog.yaml")
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

    expected_sddl =      "O:SYG:SYD:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;WD;;;BU)(A;OICI;FA;;;#{datadogagent_sid})(A;OICI;FA;;;#{traceagent_sid})"
    actual_sddl = get_sddl_for_object("#{configuration_path}\\conf.d")

    sddl_result = equal_sddl?(expected_sddl, actual_sddl)
    expect(sddl_result).to be_truthy
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
    ddagent_sdl = "(A;;KA;;;#{datadogagent_sid})(A;OICIIO;CCDCLCSWRPWPSDRCWDWOGA;;;#{datadogagent_sid})"
    trace_sdl = "(A;;KA;;;#{traceagent_sid})(A;OICIIO;CCDCLCSWRPWPSDRCWDWOGA;;;#{traceagent_sid})"
    expected_sddl =           "O:SYG:SYD:AI(A;;KA;;;SY)(A;;KA;;;BA)#{ddagent_sdl}#{trace_sdl}(A;CIID;KR;;;BU)(A;CIID;KA;;;BA)(A;CIID;KA;;;SY)(A;CIIOID;KA;;;CO)(A;CIID;KR;;;AC)"
    expected_sddl_2008 =      "O:SYG:SYD:AI(A;;KA;;;SY)(A;;KA;;;BA)#{ddagent_sdl}#{trace_sdl}(A;ID;KR;;;BU)(A;CIIOID;GR;;;BU)(A;ID;KA;;;BA)(A;CIIOID;GA;;;BA)(A;ID;KA;;;SY)(A;CIIOID;GA;;;SY)(A;CIIOID;GA;;;CO)"
    expected_sddl_with_edge = "O:SYG:SYD:AI(A;;KA;;;SY)(A;;KA;;;BA)#{ddagent_sdl}#{trace_sdl}(A;CIID;KR;;;BU)(A;CIID;KA;;;BA)(A;CIID;KA;;;SY)(A;CIIOID;KA;;;CO)(A;CIID;KR;;;AC)(A;CIID;KR;;;S-1-15-3-1024-1065365936-1281604716-3511738428-1654721687-432734479-3232135806-4053264122-3456934681)"

    ## sigh.  M$ added a mystery sid some time back, that Edge/IE use for sandboxing,
    ## and it's an inherited ace.  Allow that one, too

    actual_sddl = get_sddl_for_object("HKLM:Software\\Datadog\\Datadog Agent")

    sddl_result = equal_sddl?(expected_sddl, actual_sddl)
    equal_2008 = equal_sddl?(expected_sddl_2008, actual_sddl)
    edge_result = equal_sddl?(expected_sddl_with_edge, actual_sddl)
    expect(sddl_result | equal_2008 | edge_result).to be_truthy
  end

  it 'has agent.exe running as local service' do
    uname = get_username_from_tasklist("agent.exe")
    expect(get_username_from_tasklist("agent.exe")).to eq("LOCAL_SERVICE")
  end

end

describe 'dd-agent-install-alternate-dir' do
  it_behaves_like 'a correctly created configuration root'
  it_behaves_like 'a correctly created binary root'
  it_behaves_like 'an Agent with valid permissions'
end
  
