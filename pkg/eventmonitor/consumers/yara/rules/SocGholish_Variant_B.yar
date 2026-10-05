rule SocGholish_Variant_B {

  meta:
      author = "Ankit Anubhav -ankitanubhav.info"
      description = "Detects SocGholish obfuscated variant first observed in July 2022"
      date = "2022-07-19"
      yarahub_author_twitter = "@ankit_anubhav"
      yarahub_author_email = "ankit.yara@inbox.ru"
      yarahub_reference_link = "https://twitter.com/ankit_anubhav/status/1549246034831781888"
      yarahub_reference_md5 = "4fcc9569ca63cb2f5777954ac4c9290f"
      yarahub_uuid = "df3d194a-c6bc-4440-bad9-461e0e7962fd"
      yarahub_license = "CC0 1.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
      malpedia_family = "js.fakeupdates"

   strings:
       $x = { 3d 3d }
       $y = { 66 75 6e 63 74 69 6f 6e }
       $z = { 72 65 74 75 72 6e }
    
    condition:
       (#x > 200 and #x < 500)  and (#y > 200 and #y < 270) and (#z > 180 and #z < 190) and filesize > 37000 and filesize < 42000



}
