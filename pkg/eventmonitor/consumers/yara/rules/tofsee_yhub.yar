rule tofsee_yhub {
    meta:
        date = "2022-10-23"
        yarahub_uuid = "a2863cf2-6b6e-42e4-b78a-7e3fe72659ce"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "92e466525e810b79ae23eac344a52027"
        yarahub_author_twitter = "@billyaustintx"
        author = "Billy Austin"
        description = "Detects Tofsee botnet, also known as Gheg"
        malpedia_family = "Tofsee"
    strings:
        $s1 = "Too many errors in the block" ascii
        $s2 = "%OUTLOOK_BND_" ascii
        $s3 = "no locks and using MX is disabled" ascii
        $s4 = "mx connect error" ascii
        $s5 = "Too big smtp respons" ascii
        $s6 = "INSERT_ORIGINAL_EMAIL" ascii
        $s7 = "integr_nl = %d" ascii
        $s8 = "mail.ru" ascii
        $s9 = "smtp_herr" ascii
        $s10 = "%OUTLOOK_MID" ascii

    condition:
        uint16(0) == 0x5a4d and filesize < 200KB and 7 of ($s*)
}