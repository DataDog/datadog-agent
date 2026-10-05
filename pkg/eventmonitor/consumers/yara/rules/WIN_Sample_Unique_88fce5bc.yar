rule WIN_Sample_Unique_88fce5bc {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "88fce5bc260870ef6296c4c5967449d0dc38e83b3fcfea5a971446e8dfd1f5ff.exe"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "c4722a54-e539-4d4b-8fc8-908adf88b01c"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "ba6158ffb7ef5e0bc522fc0118c9d2cf"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "    <paragraph>The application supports multiple configuration profiles that can be customized to meet your organization requirements. Each profile can define different security policies and access controls.</paragraph>" ascii wide
        $b = "    <paragraph>Backup operations can be scheduled to run automatically at specified intervals. Full and incremental backup modes are supported. Backups can be stored locally or on network shares.</paragraph>" ascii wide
    condition:
        all of them
}
