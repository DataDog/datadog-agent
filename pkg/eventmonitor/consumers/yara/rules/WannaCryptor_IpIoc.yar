rule WannaCryptor_IpIoc
{
    meta:
        author = "blade391off"
        description = "Detects WannaCryptor malware"
        date = "2026-09-26"
        yarahub_uuid = "f702abed-3b3a-4ffa-b753-1f99645a901e"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "84C82835A5D21BBCF75A61706D8AB549"
        
    strings:
        $be_ip1 = { CC 0D CA 47 }
        $be_ip2 = { 08 F8 73 FE }
        $be_ip3 = { AC D9 10 CE }
        $be_ip4 = { 5B C7 D4 34 }
        $be_ip5 = { 0D 23 FE 52 }
        $be_ip6 = { 54 0F 40 0D }
        $be_ip7 = { 5D B8 DC 1D }
        $be_ip8 = { D9 4F B3 B1 }
        $be_ip9 = { 80 1F 00 27 }
        $be_ip10 = { 9E AE FF EB }
        $be_ip11 = { 3E D2 7B 18 }
        $be_ip12 = { 5D BA C8 D5 }
        $be_ip13 = { D8 3A CF 43 }
        $be_ip14 = { D8 3A D0 2D }
        $be_ip15 = { D8 3A CD EE }
        $be_ip16 = { AC D9 12 63 }

        $le_ip1 = { 47 CA 0D CC }
        $le_ip2 = { FE 73 F8 08 }
        $le_ip3 = { CE 10 D9 AC }
        $le_ip4 = { 34 D4 C7 5B }
        $le_ip5 = { 52 FE 23 0D }
        $le_ip6 = { 0D 40 0F 54 }
        $le_ip7 = { 1D DC B8 5D }
        $le_ip8 = { B1 B3 4F D9 }
        $le_ip9 = { 27 00 1F 80 }
        $le_ip10 = { EB FF AE 9E }
        $le_ip11 = { 18 7B D2 3E }
        $le_ip12 = { D5 C8 BA 5D }
        $le_ip13 = { 43 CF 3A D8 }
        $le_ip14 = { 2D D0 3A D8 }
        $le_ip15 = { EE CD 3A D8 }
        $le_ip16 = { 63 12 D9 AC }

    condition:
        uint16(0) == 0x5A4D and (5 of ($be_ip*) or 5 of ($le_ip*))
}
