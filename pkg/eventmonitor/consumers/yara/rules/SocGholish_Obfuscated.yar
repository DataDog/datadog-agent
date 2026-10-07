rule SocGholish_Obfuscated {

  meta:
      author = "Ankit Anubhav -ankitanubhav.info"
      description = "Detects reverse obfuscated socgholish string"
      date = "2022-06-25"
      yarahub_author_twitter = "@ankit_anubhav"
      yarahub_author_email = "ankit.yara@inbox.ru"
      yarahub_reference_link = "https://twitter.com/ankit_anubhav/status/1540395958428504064"
      yarahub_reference_md5 = "7fb296f96e098bdaaaa518c2ba176ece"
      yarahub_uuid = "e32059b3-f685-42a7-9f45-1d977046611a"
      yarahub_license = "CC0 1.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
      malpedia_family = "js.fakeupdates"

   strings:
       $x = { 70 ?? 74 ?? 74 ?? 68 }
       $y = { 67 ?? 6e ?? 70 ?? 2e [1-3] 6c ?? 65 ?? 78 ?? 69 ?? 70 }
       $z = { 66 ?? 69 ?? 67 ?? 2e ?? 31 ?? 78 ?? 31 }
    condition:
       $x and ($y or $z)  and filesize > 500 and filesize < 3000



}
