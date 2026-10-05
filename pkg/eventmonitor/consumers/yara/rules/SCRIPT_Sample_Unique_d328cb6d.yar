rule SCRIPT_Sample_Unique_d328cb6d {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "d328cb6d3beca930f3a4db600e4179031a3dfc200874ffbebc55521cf3eed7ec.unknown"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "1c4ac4e2-dc91-4402-afc0-2b1b638f5760"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "8e5ee2f323d0a4fbc83b2e590829ce42"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "<p class=\"secondary-text\">We are doing some maintenance on our site. It won't take long, we promise. Come back and visit us again in a few days. Thank you for your patience!</p>" ascii wide
        $b = "    <meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">" ascii wide
    condition:
        all of them
}
