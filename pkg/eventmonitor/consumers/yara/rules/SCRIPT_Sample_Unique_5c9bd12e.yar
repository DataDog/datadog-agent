rule SCRIPT_Sample_Unique_5c9bd12e {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "5c9bd12ecc64164884e476b3183f73758fbfadf55d571805b053addd07776f17.unknown"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "da8b7bdd-175f-4287-83eb-697ae9dc8da4"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "84e7186f472622e02a9585a73e517bc6"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "hxxp://31.56.39.60/memory_bin_dir/memory_load.mips" ascii wide
        $b = "hxxp://star.zcnet.net:7766/Server.exe" ascii wide
    condition:
        all of them
}
