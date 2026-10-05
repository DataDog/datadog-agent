rule JAVA_Sample_Unique_f35d7502
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "f35d75027cdf74b4b80f349b3639f680fa5e89015eab3179b53b1e3f1a4e0d8b"
        yarahub_uuid = "730c1417-913d-4b8a-9849-f07f03e530c7"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "850822b98b20a8d9bdb112b638ba2f2e"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "com/foure/client/auth/LicenseDecryptor" ascii
        $s1 = "validateLicenseBytes" ascii
        $s2 = "decryptLicenseBytes" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them
}
