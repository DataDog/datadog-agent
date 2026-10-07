rule HellsUchecker_MSI_Dropper
{
    meta:
        id = "7yYUKBsAG7h9MzsQlZbxze"
        fingerprint = "2ca7e864d2cdfad4bb2dfb2c98d6da02e6634502a40912ec8a03f2970f1d70cd"
        version = "1.0"
        date = "2026-03-11"
        modified = "2026-03-11"
        status = "RELEASED"
        sharing = "TLP:CLEAR"
        source = "HTTPS://GITHUB.COM/KIRKDERP/YARA"
        author = "kirkderp"
        description = "HellsUchecker MSI dropper - WiX-built installer masquerading as Microsoft Update. Drops BAT/MSBuild polyglot + VBS launcher to SvcUpdate directory."
        category = "MALWARE"
        malware = "HELLSUCHECKER"
        mitre_att = "T1218.007"
        reference = "https://derp.ca/blog/hellsuchecker-clickfix-etherhiding"
        triage_score = 10
        triage_description = "HellsUchecker MSI dropper detected. Fake Microsoft Update installer dropping BAT/MSBuild polyglot."
        yarahub_uuid = "c5b7d204-da91-4d64-beae-330a975c1cb7"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "465d5847030acbbe1b6aaad39088d2f1"

    strings:
        $install_dir = "SvcUpdate_" ascii wide
        $bat_name = "wscript_ce49" ascii wide
        $vbs_name = "wscript_29ab" ascii wide
        $cache_name = "runtime_75ef" ascii wide
        $fake_author = "Microsoft Update" ascii wide
        $product = "standalone_standard_configure" ascii wide

    condition:
        uint32(0) == 0xE011CFD0
        and filesize > 500KB
        and filesize < 5MB
        and $install_dir
        and ($bat_name or $vbs_name or $cache_name)
        and ($fake_author or $product)
}
