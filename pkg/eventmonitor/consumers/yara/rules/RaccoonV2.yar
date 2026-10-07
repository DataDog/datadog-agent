rule RaccoonV2 : loader stealer
{
    meta:
        author                    = "@_FirehaK <yara@firehak.com>"
        date                      = "2022-06-04"
        description               = "Detects Raccoon Stealer version 2.0 (called Recordbreaker before attribution)."
        malpedia_family           = "win.recordbreaker"
        modified                  = "2022-10-23"
        reference                 = "https://www.zerofox.com/blog/brief-raccoon-stealer-version-2-0/"
        yarahub_author_twitter    = "@_FirehaK"
        yarahub_author_email      = "yara@firehak.com"
        yarahub_reference_link    = "https://www.zerofox.com/blog/brief-raccoon-stealer-version-2-0/"
        yarahub_reference_md5     = "b35cde0ed02bf71f1a87721d09746f7b"
        yarahub_uuid              = "817722f6-fe01-4772-b432-adb7b0c3a5ec"
        yarahub_license           = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"

    strings:
        $get_username = { 6802020000 6a40 c745fc01010000 (ff15??????00|ffd0) 8bf0 8d45fc 50 56 ff15??????00 8bc6 5e c9 c3 }
        $to_wide_char = { 8d145d10000000 52 6a40 (ff15??????00|ffd6) 53 8bf0 56 6aff 57 6a00 68e9fd0000 ff15 }
        $x1 = { 6878ff0000 6a40 8bf1 (ff15??????00|ffd0) 8b16 8bc8 e8???????? ba???????? 8bc8 e8???????? ba???????? 8bc8 5e e9 }
        $x2 = { ff15??????00 85ff 75?? 57 ff15??????00 8b45?? 40 8945?? 83f805 7c?? eb }
        $x3 = { 6808020000 6a40 (ff15??????00|ffd0) 8b55e4 8bc8 e8???????? 8b15???????? 8bc8 e8???????? 8b7df4 8bc8 8bd7 e8???????? ba??????00 8bc8 e8???????? 8b0d???????? 8b }
        $x4 = { 6808020000 6a40 (ff15??????00|ffd1) 6a00 6a1a 50 6a00 8945?? ff15??????00 8bce e8???????? 85c0 74 }
        $x5 = { 85c9 74?? 0fb73c30 6685ff 74?? 66893e 83c602 49 83ea01 75?? 5f 33c9 b87a000780 }
        $xor_c2 = { 8bc8 33d2 8b45fc f7f1 8a0e 8b45fc 328a???????? 40 880c33 46 8945fc 83f840 72 }
        $xor_str = { 8bc8 33d2 8bc3 f7f1 8b45f8 8a0c02 8d1433 8b45fc 8a0410 32c1 43 8802 3bdf 72 }

    condition:
        uint16(0) == 0x5a4d
        and 3 of them
}