rule Jeevan_SMS_Forwarder_APK
{
    meta:
        date = "2026-04-01"
        yarahub_uuid = "1ecefa81-c67f-4284-a79a-4be6ec32e3ea"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "e3dc06e8292aeb1efe674cf20e56c182"
        description = "Detects JeevanReward SMS stealer APK with Telegram exfiltration"
        author = "ShriTiger"
        type = "Android Malware"
        sha256 = "fcae342143181618c804177bc2ac9df11fe6ecd22fa639f639b4360848687ebf"

    strings:
        // Primary identifier - package name (confirmed 3x hits)
        $pkg = "com.idormy.sms.forwarder" ascii wide nocase
        
        // Confirmed malware components (28+ hits total)
        $sms = "SmsForwarder" ascii wide nocase
        $telegram = "telegram" ascii wide nocase
        
        // Domains from analysis report
        $domain1 = "okbuysuc.com" ascii wide nocase
        $domain2 = "okbinances.com" ascii wide nocase
        
        // APK hash (exact match)
        $hash = { fc ae 34 21 43 18 16 18 c8 04 17 7b c2 ac 9d f1 
                  1f e6 ec d2 2f a6 39 f6 39 b4 36 08 48 68 7e bf }

    condition:
        // Primary detection: Package + malware component
        $pkg and ( $sms or $telegram )
        or
        // Perfect hash match  
        $hash
        or
        // Fallback: All three indicators
        $sms and $telegram and ( $domain1 or $domain2 )
}