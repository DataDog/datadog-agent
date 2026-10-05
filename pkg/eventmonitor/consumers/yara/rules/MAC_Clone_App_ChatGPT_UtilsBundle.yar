rule MAC_Clone_App_ChatGPT_UtilsBundle
{
    meta:
        description = "Clone macOS usurpant ChatGPT - bundle identifier com.utils.chatgpt (l'officiel OpenAI est com.openai.chat)"
        author = "Marjoriefort"
        yarahub_reference_md5 = "4ea26bcb75f0115625644eb8e23a2cef"
        date = "2026-09-19"

        yarahub_uuid = "d484e5f2-b754-4df9-a789-8c73d064a975"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $a = "com.utils.chatgpt"

    condition:
        $a and filesize < 100KB
}
