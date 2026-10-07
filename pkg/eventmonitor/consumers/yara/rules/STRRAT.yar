rule STRRAT {
    meta:
        author = "NDA0E"
        yarahub_author_twitter = "@NDA0E"
        date = "2024-04-28"
        description = "Detects STRRAT config filename"
        yarahub_uuid = "a8d86b9e-fd57-422c-9124-88bbfc9b75c7"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "5d16505a5abfcfc99095a676f1f0bd64"
        malpedia_family = "jar.strrat"
    
    strings:
        $config = "config.txt" ascii
        $str01 = "carLambo" ascii
        $str02 = "kingDavid" ascii
    
    condition:
        uint32(0) == 0x04034b50 and
        (($config) and
        any of ($str*))
}    