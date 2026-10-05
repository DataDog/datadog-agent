rule SocGholish_Custom_Base64 {

  meta:
      author = "Ankit Anubhav -ankitanubhav.info"
      description = "Detects custom base64 used by SocGholish"
      date = "2022-08-02"
      yarahub_author_twitter = "@ankit_anubhav"
      yarahub_author_email = "ankit.yara@inbox.ru"
      yarahub_reference_link = "https://twitter.com/ankit_anubhav"
      yarahub_reference_md5 = "28b01b187ecb0bdc1301da975b52a2fa"
      yarahub_uuid = "10fcd711-8af7-432e-89a7-ae3c109c7dc2"
      yarahub_license = "CC0 1.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
      malpedia_family = "js.fakeupdates"

   strings:
       $x = "&15)<<4)|("
       $y = { 69 6e 64 65 78 4f 66 28 ?? ?? 2e 63 68 61 72 41 74 28 ?? ?? 2b 2b 29 }
       $z = "ABCD"
    condition:
       $x and #y == 4 and (not $z) and filesize > 500 and filesize < 3000



}
