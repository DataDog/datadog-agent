rule detect_braodo_stealer {

meta: 
    author = "Priya"
    description = "This rule detects Broaodo Stealer"
    date = "2024-10-02"
    yarahub_reference_md5 = "e7f57ef84d7c3ab8fbdb985d5bc7735c"
    yarahub_uuid = "2b7264f6-0af4-4d95-8009-7a9a4cf1871f"
    yarahub_license = "CC0 1.0"
    yarahub_rule_matching_tlp = "TLP:WHITE"
    yarahub_rule_sharing_tlp = "TLP:WHITE"

strings:
    $string_one = "cookiefb.txt"
    $string_two = "https://api.telegram.org/bot"
    $string_three = "taskkill /f /im chrome.exe"
    $string_four = "logins.json"
    $string_five = "https://ipinfo.io"
	


condition:
    (3 of them) or (all of them)



}