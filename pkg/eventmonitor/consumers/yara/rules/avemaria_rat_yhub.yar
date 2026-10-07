rule avemaria_rat_yhub {
    meta:
        date = "2022-10-18"
        yarahub_uuid = "bb7a4c5e-c2dc-46a1-8a82-028e4e1c5570"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "7897feb76a3beab6fe8aa9851a894437"
        yarahub_author_twitter = "@billyaustintx"
        author = "Billy Austin"
        description = "Detects AveMaria RAT a.k.a. WarZone"
        malpedia_family = "AVE_MARIA"

    strings:
        $h1 = "find.db" ascii //packed
        $h2 = "encryptedPassword" ascii
        $h3 = "encryptedUsername" ascii
        $h4 = {63 6d 64 2e 65 78 65 20 2f 43 20 70 69 6e 67} // cmd.exe /C ping
        
        $u1 = "logins.json" wide
        $u2 = "usebackq tokens" wide
        $u3 = "\\rdpwrap.ini" wide //persistence
        $u4 = "MidgetPorn" wide
        $u5 = "wmic process call create" wide
        $u6 = "sqlmap.dll" wide
        
    condition:
        uint16(0) == 0x5a4d and filesize < 1125KB and 3 of ($h*) and 4 of ($u*)
}