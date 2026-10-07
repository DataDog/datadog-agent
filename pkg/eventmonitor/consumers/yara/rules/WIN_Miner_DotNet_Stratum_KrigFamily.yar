rule WIN_Miner_DotNet_Stratum_KrigFamily
{
    meta:
        description = "Mineur .NET moderne Stratum (ConnectAndAuthorize/RunStratumAsync) - famille krig/lpminer, pool kryptex"
        author = "Marjoriefort"
        yarahub_reference_md5 = "c008c4ca2d3e3ecddf3173eef1e4abe9"
        date = "2026-09-19"
        yarahub_uuid = "f7612d59-334f-40fa-b8ec-9ea8f2d153da"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $a = "ConnectAndAuthorizeAsync"
        $b = "RunStratumAsync"
        $c = "StratumJobRegistry"
        $d = "WarnIfPoolCertVersionOffConsensus"
        $e = "SubmitShareAsync"
    condition:
        2 of them and uint16(0) == 0x5A4D
}
