rule SCRIPT_Sample_Unique_56aec3b5 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "56aec3b560f70e6cb88de9ef1abd252a4c0c90ae3bb6ed269d89cd6116257334.unknown"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "00549afb-67f8-4937-bc4a-dda2eca99e2f"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "28b3c4b2a39204e0fdcc1b493e0dbab3"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))" ascii wide
        $b = "from app import run" ascii wide
    condition:
        all of them
}
