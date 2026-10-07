rule ZeuS {
    meta:
        author = "NDA0E"
        yarahub_author_twitter = "@NDA0E"
        date = "2024-09-20"
        description = "Detects ZeuS"
        yarahub_uuid = "5f4ca030-2799-47d0-907a-942f84cff1c7"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "5a869577bc8122b96a3c8fdb26c2c10e"
        malpedia_family = "win.zeus"
    
    strings:
        $str1 = "*<input *value=\"" ascii
        $str2 = "*<option  selected" ascii
        $str3 = "*<select" ascii
        $str4 = "Ik{wvAapcgd1)%" ascii
        
    condition:
        all of them and
        uint16(0) == 0x5a4d
}