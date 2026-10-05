rule QBOT_HTMLSmuggling_a {

  meta:
      author = "Ankit Anubhav - ankitanubhav.info"
      description = "Detects QBOT HTML smuggling variants"
      date = "2022-06-26"
      yarahub_author_twitter = "@ankit_anubhav"
      yarahub_author_email = "ankit.yara@inbox.ru"
      yarahub_reference_link = "https://twitter.com/ankit_anubhav"
      yarahub_reference_md5 = "1807f10ee386d0702bbfcd1a4da76fd1"
      yarahub_uuid = "8db8aecd-53ae-4772-8d9c-38b121cfe0e0"
      yarahub_license = "CC0 1.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
      malpedia_family = "win.qakbot"

   strings:
       $x = "html"
       $y = "UEsDB"
       $z = "atob("
       $c1 = "viewport"
       $c2 = "initial-scale=1"
       $escaped = { 5c 78 36 44 5c 78 37 33 5c 78 35 33 5c 78 36 31 5c 78 37 36 5c 78 36 35 5c 78 34 46 5c 78 37 32 5c 78 34 46 5c 78 37 30 5c 78 36 35 5c 78 36 45 5c 78 34 32 5c 78 36 43 5c 78 36 46 5c 78 36 32 }
       $normal = "msSaveOrOpenBlob"
       $qbot26092022 = { 2e 7a 69 70 3c 2f 62 3e }
       $qbotmagic = "VUVzREJC"
       $qbotmagic_reversed = "CJERzVUV"
       $obama211 = "IHImERWP"
    condition:
       ($x and $y and $z and (($c1 and $c2) or $qbot26092022 ) and ($escaped or $normal)) or ($x and ($qbotmagic or $qbotmagic_reversed or $obama211))  and filesize > 500
}
