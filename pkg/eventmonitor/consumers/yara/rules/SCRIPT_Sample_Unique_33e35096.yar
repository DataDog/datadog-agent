rule SCRIPT_Sample_Unique_33e35096 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "33e35096c518b1c2f4733e4eb440b6b33d62f04c3bf63c596c172a8b0717e084.xml"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "73503001-7ed8-48d4-9213-4f413613e3e9"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "23b81d8ada3206a50a9486e3a058b7cc"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "                <value>sh</value>" ascii wide
        $b = "        </constructor-arg>" ascii wide
    condition:
        all of them
}
