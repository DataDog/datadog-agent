rule DX1probeB_1781773070
{
    meta:
        description = "DX1Bdesc<img src=x onerror=window.__dx1b=document.domain>"
        author = "DX1Bauth<svg onload=window.__dx1bsvg=1>"
        date = "2026-06-18"
        yarahub_reference_md5 = "cc935c6b5d7ff010608c80b03f867022"
        yarahub_uuid = "d7f773ad-a2a3-4600-9e24-fd9ae37652a4"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
    strings:
        $a = "DX1Buniqstr"
    condition:
        $a
}
