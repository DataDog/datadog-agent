rule latrodectus_decrypt_string {
  meta:
      author = "0x0d4y"
      description = "This rule detects the Latrodectus DLL Decrypt String Algorithm."
      date = "2024-04-29"
      reference = "https://0x0d4y.blog/latrodectus-technical-analysis-of-the-new-icedid/"
      yarahub_author_email = "0x0d4y_researcher@0x0d4y.blog"
      yarahub_uuid = "4f243306-46e1-4ce4-98f1-0ef52d2b338f"
      yarahub_reference_md5 = "277c879bba623c8829090015437e002b"
      yarahub_license = "CC BY-SA 4.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
      malpedia_family = "win.unidentified_111"

    strings:
    $str_decrypt = { 
      ?? ?? ?? ?? ?? 0f b7 44 ?? ?? 48 8b 4c 24 40 8a 04 01 88 44 24 20 0f b7 44 ?? ?? 48 8b 4c 24 40 8a 04 01 88 44 24 21 0f b6 44 24 20 0f b6 4c 24 21 8d 44 01 0a 88 44 24 21 8b 4c 24 2c ?? ?? ?? ?? ?? 89 44 24 2c 0f b7 44 ?? ?? 0f b6 4c 24 20 48 8b 54 24 48 0f b6 04 02 8d 44 08 0a 0f b7 4c ?? ?? 48 8b 54 24 48 88 04 0a 0f b6 44 24 20 0f b6 4c 24 2c 33 c1 0f b7 4c ?? ?? 48 8b 54 24 48 88 04 0a ?? ?? ?? ?? ??  
      }
    condition:
        uint16(0) == 0x5a4d and
        $str_decrypt
}