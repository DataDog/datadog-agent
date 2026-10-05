rule Java_GitHub_Resolver_Dropper {
    meta:
        description = "Dropper Java deguise en projet GitHub (classes obfusquees, resolver)"
        author      = "Marjoriefort"
        date        = "2026-09-16"
        reference   = "Veille fraicheur 2026-09-14 / jar 792550b4"
        confidence  = "medium"
        yarahub_uuid            = "480038a1-9c52-4391-9f74-0d16d3c87032"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5   = "fe327e91fb8557e6c35cc6fa8958bb15"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "murzrynResolver" ascii wide
        $b = "LICENSE_github" ascii wide
    condition:
        all of them
}
