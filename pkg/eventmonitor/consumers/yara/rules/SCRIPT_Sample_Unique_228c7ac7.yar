rule SCRIPT_Sample_Unique_228c7ac7 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "228c7ac798b4ffe26fb8439a53fbcfe2453b3a7d6f606f7116e319063f5a9c05.py"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "dcba5d32-7f6d-4bc5-a1dd-83b9ed5e1a51"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "2c0b232536823a77dba8a399965c6b64"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "b5VyGDBp3Thmd = iQ6GzMIAqS_([90-65,0,260-184,58^113,83^3,153,29+112,155,282-184,124,158-108,135^112,-74+128,-118+149,15^139,210,107-97,52,124+60,87-39,247-124,-99+166,169^235,50^102,49+124,59*2,-111+162,-199+200,152-47])" ascii wide
        $b = "gBxAyFeSSyoh   = ERch9BNWh99Dr4f2[str(bytes(b-21 for b in [129,122,131]),str(bytes(c^191 for c in bytes.fromhex(\"deccdcd6d6\")),bytes(b^77 for b in [44,62,46,36,36]).decode()))]" ascii wide
    condition:
        all of them
}
