rule AtlasB_Batch_Crypter
{
    meta:
        date = "2025-11-28"
        yarahub_uuid = "e7805538-d023-4c31-b4b3-eb49a74a13ce"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "832d3c0a59038aa9da00046ce6270a30"
name = "AtlasB Batch Crypter"
        description = "Detects the AtlasB batch crypter using certutil decode + temp dropper"
        author = "Larp"
        family = "AtlasB"
        version = "1.1"

    strings:
        $family = "atlasb" nocase

        $s1 = "if not defined IS_MINIMIZED" nocase
        $s2 = "start /min \"\" \"%~f0\"" nocase

        $s3 = "certutil -decode \"%TEMP%\\atlasb" nocase

        $s4 = ">> \"%TEMP%\\atlasb" nocase

        $s5 = "start /b cmd /c" nocase

        $re_filename = /atlasb[0-9]{5,9}\.(bat|b64)/ nocase

    condition:
        $family and

        all of ($s1, $s2, $s3) and

        any of ($s4, $s5) and

        $re_filename
}
