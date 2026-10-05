rule Qakbot_WSF_loader {

  meta:
      author = "Ankit Anubhav -ankitanubhav.info"
      description = "Detects a WSF loader used to deploy Qakbot DLL"
      date = "2023-02-15"
      yarahub_author_twitter = "@ankit_anubhav"
      yarahub_author_email = "ankit.yara@inbox.ru"
      yarahub_reference_link = "https://twitter.com/ankit_anubhav"
      yarahub_reference_md5 = "ff19670725eaf5df6f3d2ca656d3db27"
      yarahub_uuid = "211e3eac-1acf-45af-bac9-e0a4c353560c"
      yarahub_license = "CC0 1.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
      malpedia_family = "win.qakbot"

   strings:

    $y = "noitcnuf" nocase
    $z = "BEGIN CERTIFICATE REQUEST" nocase

    condition:
    $y and $z and filesize < 20000

}
