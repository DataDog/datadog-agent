rule dcrat_rkp {
    meta:
      author = "jeFF0Falltrades"
      description = "Detects DCRat payloads"
      date = "2024-09-19"
      yarahub_author_twitter = "@jeFF0Falltrades"
      yarahub_reference_link = "https://github.com/jeFF0Falltrades/rat_king_parser/blob/master/src/rat_king_parser/yara_utils/rules.yar"
      yarahub_reference_md5 = "47e15cd5700a72858107bb17a3ba459d"
      yarahub_uuid = "10daffb1-c7a2-4e84-9336-02bcf9dca598"
      yarahub_license = "CC BY 4.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
      malpedia_family = "win.dcrat"

    strings:
        $venom_1 = "VenomRAT" wide ascii nocase
        $venom_2 = "HVNC_REPLY_MESSAGE" wide ascii
        $str_aes_exc = "masterKey can not be null or empty" wide ascii
        $str_b64_amsi = "YW1zaS5kbGw=" wide ascii
        $str_b64_virtual_protect = "VmlydHVhbFByb3RlY3Q=" wide ascii
        $str_dcrat = "dcrat" wide ascii nocase
        $str_plugin = "save_Plugin" wide ascii
        $str_qwqdan = "qwqdan" wide ascii
        $byte_aes_key_base = { 7E [3] 04 73 [3] 06 80 }
        $patt_config = { 72 [3] 70 80 [3] 04 }
        $patt_verify_hash = { 7e [3] 04 6f [3] 0a 6f [3] 0a 74 [3] 01 }

    condition:
        (not any of ($venom*)) and 5 of them and #patt_config >= 10
 }
