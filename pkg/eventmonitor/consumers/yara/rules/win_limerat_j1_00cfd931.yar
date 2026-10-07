rule win_limerat_j1_00cfd931 {

    meta:
        author                    = "Johannes Bader"
        date                      = "2021-10-01"
        description               = "detects the lime rat"
        hash                      = "2a0575b66a700edb40a07434895bf7a9"
        malpedia_family           = "win.limerat"
        tlp                       = "TLP:WHITE"
        version                   = "v1.1"
        yarahub_author_email      = "yara@bin.re"
        yarahub_author_twitter    = "@viql"
        yarahub_license           = "CC BY-SA 4.0"
        yarahub_reference_md5     = "2a0575b66a700edb40a07434895bf7a9"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_uuid              = "00cfd931-3e03-4e32-b0d7-ca8f6bbfe062"

    strings:
        $str_1 = "Y21kLmV4ZSAvYyBwaW5nIDAgLW4gMiAmIGRlbCA=" wide
        $str_2 = "schtasks /create /f /sc ONLOGON /RL HIGHEST /tn LimeRAT-Admin" wide
        $str_3 = "Minning..." wide
        $str_4 = "--donate-level=" wide

    condition:
        uint16(0) == 0x5A4D and
        3 of them
}
