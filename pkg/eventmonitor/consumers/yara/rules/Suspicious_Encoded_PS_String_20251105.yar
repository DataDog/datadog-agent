rule Suspicious_Encoded_PS_String_20251105
{
    meta:
        author       = "ShadowOpCode"
        date         = "2025-11-05"
        description  = "Detects ASCII string"
        reference    = "internally crafted rule"
        yarahub_uuid = "8b0a9b66-c3a2-4d4e-8d7d-ac7c43b1d6f8"
		yarahub_reference_md5 = "04428fba0f6c5caaffcc55dd73e911e7"
		yarahub_license = "CC0 1.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        $encoded_str = "DQojIEVuY3J5cHRlZCBQb3dlclNoZWxsIFNjcmlwd" ascii

    condition:
        any of ($encoded_str)
}