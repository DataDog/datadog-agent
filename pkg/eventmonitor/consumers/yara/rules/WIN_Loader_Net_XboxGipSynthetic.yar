rule WIN_Loader_Net_XboxGipSynthetic
{
    meta:
        description = "PE natif chargeant du .NET obfusque (GetDelegateForFunctionPointer + clrjit), deguise en processus Xbox"
        author = "Marjoriefort"
        yarahub_reference_md5 = "c7a019ce3e8cf5c9ba381169136e9a2f"
        date = "2026-09-19"

        yarahub_uuid = "89d82f39-e0a2-45e4-b00d-5c331fe43f13"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $a = "xboxgipsynthetic.exe" wide ascii
        $b = "GetDelegateForFunctionPointer" wide ascii
        $c = "clrjit.dll" wide ascii

    condition:
        uint16(0) == 0x5A4D and $a and ($b or $c)
}
