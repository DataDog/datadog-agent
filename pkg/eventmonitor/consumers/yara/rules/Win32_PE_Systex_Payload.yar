rule Win32_PE_Systex_Payload {
    meta:
        description = "Detects Systex PE executable based on unique network C2 domains, targeted Windows service manipulation, and process control Native APIs."
        author = "Serhii Kocherhan"
        date = "2026-09-16"
        yarahub_twitter = "@skocherhan"
        yarahub_uuid = "9032629f-19d6-4a2a-8d25-ceda936ff0ae"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "bbd29918babe59132c3b6f384da02c72"

    strings:
        // C2 Infrastructure String
        $c2_domain = "xytets.com:2345" ascii wide nocase

        // Native Process Control APIs
        $api_suspend = "NtSuspendProcess" ascii wide
        $api_resume  = "NtResumeProcess" ascii wide

        // Service Manipulation Target Strings
        $svc_vault   = "VaultSvc" ascii wide nocase
        $svc_clip    = "clipsvc" ascii wide nocase
        $svc_wsc     = "wscsvc" ascii wide nocase
        $svc_bits    = "BITS" ascii wide nocase
        $svc_wsearch = "WSearch" ascii wide nocase

    condition:
        // Validate Portable Executable (PE) header boundary and file size limit
        uint16(0) == 0x5A4D and 
        filesize < 5MB and
        (
            // Primary Match: Explicit C2 domain presence
            $c2_domain or

            // Secondary Match: Combination of process control Native APIs and service manipulation targets
            (
                all of ($api_suspend, $api_resume) and
                3 of ($svc_vault, $svc_clip, $svc_wsc, $svc_bits, $svc_wsearch)
            )
        )
}