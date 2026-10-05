rule win_strelastealer : stealer
{
    meta:
        author                    = "@_FirehaK <yara@firehak.com>"
        date                      = "2022-11-18"
        description               = "Detects Strela Stealer"
        malpedia_family           = "win.strelastealer"
        modified                  = "2022-11-18"
        reference                 = "https://medium.com/@DCSO_CyTec/shortandmalicious-strelastealer-aims-for-mail-credentials-a4c3e78c8abc"
        yarahub_author_twitter    = "@_FirehaK"
        yarahub_author_email      = "yara@firehak.com"
        yarahub_reference_link    = "https://medium.com/@DCSO_CyTec/shortandmalicious-strelastealer-aims-for-mail-credentials-a4c3e78c8abc"
        yarahub_reference_md5     = "57ec0f7cf124d1ae3b73e643a6ac1dad"
        yarahub_uuid              = "685f9c70-2e4f-42ba-9e9e-77d022de6d0e"
        yarahub_license           = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"

    strings:
        $s1 = "StrelaDLLCompile" ascii

        $x1 = { 8d4c24?? 51 6a00 6a00 ff15????0010 ff15????0010 3db7000000 74?? e8????ffff e8????ffff 8b8c24????0000 5f 5e 5b 33cc 33c0 e8???????? 8be5 5d c3 }
        $xor_string = { (33d2 8bc?|8bc? 33d2) f7f? 4? 8a(82|92)???????? 30(44|54)??ff (3b|83??)?? 72 }

    condition:
        uint16(0) == 0x5a4d
        and any of them
}