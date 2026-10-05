rule win_danabot_cdf38827 {

    meta:
        author                    = "Johannes Bader"
        date                      = "2022-04-19"
        description               = "detects DanaBot"
        hash1                     = "b7f891f4ed079420e16c4509680cfad824b061feb94a0d801c96b82e1f7d52ad"
        hash1b                    = "62174157b42e5c8c86b05baf56dfd24b"
        hash2                     = "c8f27c0e0d4e91b1a6f62f165d45d8616fc24d9c798eb8ab4269a60e29a2de5e"
        hash3                     = "5cb70c87f0b98279420dde0592770394bf8d5b57df50bce4106d868154fd74cb"
        malpedia_family           = "win.danabot"
        tlp                       = "TLP:WHITE"
        version                   = "v1.1"
        yarahub_author_email      = "yara@bin.re"
        yarahub_author_twitter    = "@viql"
        yarahub_license           = "CC BY-SA 4.0"
        yarahub_reference_md5     = "62174157b42e5c8c86b05baf56dfd24b"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_uuid              = "cdf38827-649c-4194-85b0-881c98f1c562"

    strings:
        $keyboard = { C6 05 [4] 71 C6 05 [4] 77 C6 05 [4] 65 C6 05 [4] 72 C6 05 [4] 74 C6 05 [4] 79 C6 05 [4] 75 C6 05 [4] 69 C6 05 [4] 6F  }
        $move_y   = { 8B 45 F8 C6 80 [4] 79 } // mov     eax, [ebp-8], mov     byte ptr <addr>[eax], 79h
        $id_str   = /[A-F0-9]{32}zz/

    condition:
        uint16(0) == 0x5A4D and
        (
            all of them
        )
}
