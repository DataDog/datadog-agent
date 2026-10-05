rule ZeroDay_Firebase_Project : config firebase malware
{
    meta:
        author = "CyberStrike"
        description = "Firebase project andromeda-d389d used by ZeroDay RAT infrastructure"
        date = "2026-09-05"
        yarahub_author_twitter = "@XenoSheesh"
        yarahub_reference_md5 = "35b2d781134a7bf705a357ec3b314910"
        yarahub_uuid = "df729f5e-a21a-4edb-8e82-d87640e707ee"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        
    strings:
        $proj = "andromeda-d389d" ascii wide
        $sender = "651390976306" ascii wide
        $api_key = "AIzaSyBMHTORCrdpWwTBah5Lrid3iGP8hJuBnbI" ascii wide
        $app_id = "1:651390976306:android:75b2e2d669a5b3936f972b" ascii wide
        
    condition:
        2 of them
}
