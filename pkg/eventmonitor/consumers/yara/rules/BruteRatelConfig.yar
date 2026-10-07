rule BruteRatelConfig
{
    meta: 
        author = "@immersivelabs"
        date = "2022-07-07"
        yarahub_uuid = "8d659456-b774-46db-a36d-6dea912e5e43"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "6c044bddd01118d311681a9b2d1dd627"
    strings:
        $config_block = { 50 48 b8 [8] 50 68}
        $split_marker = { 50 48 b8 [8] 50 48 b8 }

    condition:
        filesize < 400KB and $config_block and #split_marker > 30
}
