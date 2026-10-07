rule win_agent_tesla_ab4444e9 {

    meta:
        author                    = "Johannes Bader"
        date                      = "2020-10-01"
        description               = "detects Agent Tesla"
        hash                      = "dcd7323af2490ceccfc9da2c7f92c54a"
        malpedia_family           = "win.agent_tesla"
        tlp                       = "TLP:WHITE"
        version                   = "v1.0"
        yarahub_author_email      = "yara@bin.re"
        yarahub_author_twitter    = "@viql"
        yarahub_license           = "CC BY-SA 4.0"
        yarahub_reference_md5     = "dcd7323af2490ceccfc9da2c7f92c54a"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_uuid              = "ab4444e9-18b1-4920-b105-35377741899f"

    strings:
        $string_1  = "get_CHoo"
        $string_2  = "get_Lenght"
        $string_3  = "get_kbok"
        $string_4  = "get_sSL"
        $string_5  = "get_useSeparateFolderTree"
        $string_6  = "set_AccountCredentialsModel"
        $string_7  = "set_BindingAccountConfiguration"
        $string_8  = "set_CHoo"
        $string_9  = "set_CreateNoWindow"
        $string_10 = "set_IdnAddress"
        $string_11 = "set_IsBodyHtml"
        $string_12 = "set_Lenght"
        $string_13 = "set_MaximumAutomaticRedirections"
        $string_14 = "set_UseShellExecute"
        $string_15 = "set_disabledByRestriction"
        $string_16 = "set_kbok"
        $string_17 = "set_sSL"
        $string_18 = "set_signingEncryptionPreset"
        $string_19 = "set_useSeparateFolderTree"

    condition:
        uint16(0) == 0x5A4D and
        15 of ($string_*)
}
