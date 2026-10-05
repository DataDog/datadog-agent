rule ZeroDay_Phishing_HTML : phishing banking malware
{
    meta:
        author = "CyberStrike"
        description = "ZeroDay RAT phishing HTML for UPI PIN capture targeting Indian banks"
        date = "2026-09-05"
        yarahub_author_twitter = "@XenoSheesh"
        yarahub_reference_md5 = "35b2d781134a7bf705a357ec3b314910"
        yarahub_uuid = "79481ea0-0d6a-49ea-9e2f-8e68c88d756d"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        
    strings:
        $pin1 = "ENTER SECURE PIN" ascii wide nocase
        $pin2 = "Secure Keypad" ascii wide nocase
        $pin3 = "upi-pin" ascii wide nocase
        
        $pay1 = "Guest Registration Fee" ascii wide
        $pay2 = "PhonePe" ascii wide
        $pay3 = "Google Pay" ascii wide
        $pay4 = "Paytm" ascii wide
        
        $title1 = "Sexy Chat" ascii wide nocase
        $title2 = "Wedding" ascii wide nocase
        
    condition:
        filesize < 500KB and
        (
            (2 of ($pin*)) or
            (2 of ($pay*) and any of ($title*))
        )
}
