rule detect_qbot_v5
{
    meta:
        description = "just a rule for Qakbot v5"
        author = "Mohamed Ezzat (@ZW01f)"
        date ="2024-05-24"
        yarahub_reference_md5  = "362978ed1c1eec5ff19b744601e082a2"
        yarahub_uuid = "ca1a4dbd-0b7a-4eb1-bc46-77e711ed471f"
        yarahub_license = "CC BY-NC 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        malpedia_family = "win.qakbot"
        hash2  = "59559e97962e40a15adb2237c4d01cfead03623aff1725616caeaa5a8d273a35"
    strings:
        $s1 = "\\u%04X\\u%04X" ascii wide
        $s2 = "%u;%u;%u" ascii wide 
        $s3 = "CfGetPlatformInfo" ascii wide
        $p1 = {45 33 C0 E8 ?? ?? ?? ?? 35 91 CB 35 A2 41 3B C7}
        $p2 = { 0F B6 01 48 FF C1 44 33 C0 41 8B C0 41 C1 E8 04 83 E0 0F 44 33 04 82 41 8B C0 41 C1 E8 04 83 E0 0F 44 33 04 82 49 83 E9 01 75 ?? 41 F7 D041 8B C0 C3}
    condition:
        uint16(0) == 0x5A4D and all of ($p*) and (2 of ($s*)) and filesize < 500KB
} 
