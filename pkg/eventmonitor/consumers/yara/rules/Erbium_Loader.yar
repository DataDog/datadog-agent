rule Erbium_Loader
{
    meta:
        author                    = "@_FirehaK <yara@firehak.com>"
        date                      = "2022-09-02"
        description               = "Detects Erbium Stealer's loader"
        malpedia_family           = "win.erbium_stealer"
        modified                  = "2022-09-02"
        yarahub_author_twitter    = "@_FirehaK"
        yarahub_author_email      = "yara@firehak.com"
        yarahub_reference_link    = "https://tria.ge/220901-136gasbhdm/behavioral2"
        yarahub_reference_md5     = "7e2e4af82407b97d8f00d1ff764924d4"
        yarahub_uuid              = "1f3b58cb-cb17-45ba-aa2a-a719a4a21052"
        yarahub_license           = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"

    strings:
        $s1 = "api.php?method=getstub&bid=" wide

        $x1 = { 53 6a?? 68???????? 50 ff15???????? 8bd8 894424 }
        $x2 = { 8b35???????? 40 6a00 6a00 50 68???????? 6a00 6a00 ffd6 8bc8 33c0 660f1f440000 }
        $x3 = { 51 8d4c24?? 40 51 50 68???????? 6a00 6a00 ffd6 33c0 90 }
        $x4 = { 8b5c24?? 6a00 6a01 6a01 6a01 6a01 57 53 ff 50 e8???????? 83c4?? 85db 74 }
        $x5 = { c745??00000000 8d55?? 52 6a40 8b45?? 8b48?? 51 8b55?? 52 8b45?? 50 ff55?? 33c9 894d?? 894d?? 894d?? 894d?? 894d?? 894d?? 8b15???????? 8955?? a1???????? 8945?? 8b4d?? 894d?? 8b55?? 8955?? 8b45?? 8945?? 6a00 6800100000 8b4d?? 51 8b55?? 52 8b45?? 50 ff55?? 85c0 75  }
        $x6 = { 6800800000 6a00 8b55f8 52 8b4508 50 ff55fc 6800800000 6a00 8b4df0 51 8b5508 52 ff55fc 6800800000 6a00 8b45e8 50 8b4d08 51 ff55fc 32c0 eb0c }

    condition:
        uint16(0) == 0x5a4d
        and (
            2 of ($x*)
            or (
                $s1
                and 1 of ($x*)
            )
        )
}