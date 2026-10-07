rule Ransom_DB_Hijack
{
    meta:
        author       = "Mizuho Mori"
        description  = "Hunt: extortion note dropped by automated exposed-DB hijack campaigns"
        date         = "2026-07-17"
        mitre_attack = "T1485, T1657, T1190"
        /*
         * YARAhub mandatory metadata
         *
         * Replace this value with the MD5 of a real file
         * that matches this rule.
         */
        yarahub_reference_md5 = "f0648ae541e9ad7ecc80c34265d40b6a"
        /*
         * Generate once with:
         * python3 -c 'import uuid; print(uuid.uuid4())'
         */
        yarahub_uuid = "50a44b6c-bd7b-44dd-b458-8fe8b68cf063"

        yarahub_license           = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"

    strings:
        $full1 = "All your data is backed up. You must pay" ascii wide nocase
        $full2 = "your data will be publicly disclosed and deleted" ascii wide nocase

        $t1 = "You must pay"       ascii wide nocase
        $t2 = "publicly disclosed" ascii wide nocase
        $t3 = "backed up"          ascii wide nocase
        $t4 = "48 hours"           ascii wide nocase
        $t5 = "DBCODE"             ascii wide nocase
        $t6 = "onionmail.org"      ascii wide nocase

        $d1 = "ak+28u5u" ascii wide nocase
        $d2 = "bc1qa83x6l2dlgkx7cc9rmrymscp5sa3ljepu42w2r" ascii

    condition:
        any of ($full*) or
        3 of ($t*) or
        any of ($d*)
}
