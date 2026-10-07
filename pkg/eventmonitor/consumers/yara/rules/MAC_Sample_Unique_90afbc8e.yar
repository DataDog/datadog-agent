rule MAC_Sample_Unique_90afbc8e {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "90afbc8e8c3f9ad84d663cb6ba17f60cc97cad23d39f2548bc3dfa752ed9a9c5.scpt"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "182ed1fb-3096-4887-acac-f9666b64cc0f"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "4162b613281c9370a6de2e081e7f091b"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "-/:System:Applications:Utilities:Terminal.app/" ascii wide
        $b = ")/:System:Library:CoreServices:Finder.app/" ascii wide
    condition:
        all of them
}
