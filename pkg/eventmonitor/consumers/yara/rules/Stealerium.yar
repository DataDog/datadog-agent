rule Stealerium {
    meta:
        author = "NDA0E"
        yarahub_author_twitter = "@NDA0E"
        date = "2024-09-01"
        description = "Detects Stealerium Stealer"
        yarahub_uuid = "bbf5262c-8a7d-434d-a800-1254f1063921"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "96c62ca985ed966d7c6d274caa5bb41a"
        malpedia_family = "win.stealerium"
    
    strings:
        $GitHub = " https://github.com/kgnfth" ascii
        $StealeriumReport = " *Stealerium - Report:*" wide ascii

    condition:
        all of them and
        uint16(0) == 0x5a4d
}