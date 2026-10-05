rule SUSP_Bitdefender_Impersonation_Zip_Bundle {
    meta:
        description = "ZIP bundle impersonating Bitdefender (bitdefender.exe + ServiceInstance.dll members)"
        author      = "Marjoriefort"
        date        = "2026-09-13"
        reference   = "InTheWild.0438 / fresh CS 2026-08-06"
        confidence  = "medium-high"
        yarahub_uuid            = "f7ce48c0-0567-4305-8201-298f689d88a7"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5   = "54a74f0b55029cdb6b808f3043495505"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $zip = { 50 4B 03 04 }
        $bde = "bitdefender.exe"
        $svc = "ServiceInstance.dll"
    condition:
        $zip and $bde and $svc
}

