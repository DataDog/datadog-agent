rule generic_mal_netad_collector {
  meta:
      author = "0x0d4y"
      description = "This rule detects generic samples that implement network/active directory info collection."
      date = "2024-06-05"
      score = 75
      yarahub_reference_md5 = "aeb08b0651bc8a13dcf5e5f6c0d482f8"
      yarahub_uuid = "c51f3446-9737-4707-9f7c-04c29d873d40"
      yarahub_license = "CC BY 4.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
    $adcollect_string1 = "ipconfig /all" wide ascii
    $adcollect_string2 = "net config workstation" wide ascii
    $adcollect_string3 = "net view /all" wide ascii
    $adcollect_string4 = "net view /all /domain" wide ascii
    $adcollect_string5 = "nltest /domain_trusts" wide ascii
    $adcollect_string6 = "nltest /domain_trusts /all_trusts" wide ascii
    $adcollect_string7 = "ROOT\\CIMV2" wide ascii
    $adcollect_string8 = "SELECT * FROM Win32_OperatingSystem" wide ascii
    $adcollect_string9 = "(&(objectCategory=computer)(userAccountControl:1.2.840.113556.1.4.803:=8192))" wide ascii
    $adcollect_string10 = "(&(objectcategory=person)(samaccountname=*))" wide ascii
    $adcollect_string11 = "{001677D0-FD16-11CE-ABC4-02608C9E7553}" wide ascii
    $adcollect_string12 = "{00020404-0000-0000-C000-000000000046}" wide ascii
    $adcollect_string13 = "{109BA8EC-92F0-11D0-A790-00C04FD8D5A8}" wide ascii
    $adcollect_string14 = "POST" wide ascii
    $adcollect_string15 = "ACTIVEDS.dll" wide ascii

    condition:
        uint16(0) == 0x5a4d and
        7 of ($adcollect_string*)
}