rule SCRIPT_Sample_Unique_5fb07744 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "5fb07744c952c7a763608059bd8ebb47245a7a697252177b2f5e1599ff4997e7.unknown"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "877c565b-f972-4e51-9356-615bbdf79d97"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "fdc82b68386809c6726f4bc28b63e8df"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))" ascii wide
        $b = "from app import run" ascii wide
    condition:
        all of them
}
