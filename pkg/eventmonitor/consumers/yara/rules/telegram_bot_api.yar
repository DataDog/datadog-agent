rule telegram_bot_api {
    meta:
        author = "rectifyq"
        yarahub_author_twitter = "@_rectifyq"
        date = "2024-09-07"
        description = "Detects file containing Telegram Bot API"
        yarahub_uuid = "58c9e4fe-d1e9-46ed-913c-dba943ac16d6"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "9DA48D34DC999B4E05E0C6716A3B3B83"
    
    strings:
        $str1 = "api.telegram.org/bot" nocase
        $str2 = "api.telegram.org/bot" wide
        $str3 = "api.telegram.org/bot" xor
        
    condition:
        any of them
}  