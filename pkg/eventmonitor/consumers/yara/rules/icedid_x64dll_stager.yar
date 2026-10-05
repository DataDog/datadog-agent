rule icedid_x64dll_stager {
  meta:
      author = "0x0d4y"
      description = "This rule detects samples from the IcedID family unpacked in memory, identifying code reuse of new config decryption function."
      date = "2024-04-05"
      score = 100
      reference = "https://0x0d4y.blog/icedid-technical-analysis-of-x64-dll-version/"
      yarahub_reference_md5 = "06cc2fdfd408c15a1e16adfb46e8bb38"
      yarahub_uuid = "8a16ee05-4d94-4d62-83e0-1168b11db3f5"
      yarahub_license = "CC BY 4.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
      malpedia_family = "win.icedid"
    strings:
    $conf_decrypt_algorithm = { 
        45 33 C0 ?? ?? ?? ?? ?? ?? ?? 49 2B C9 4B 8D 14 08 49 FF C0 8A 42 40 32 02 88 44 11 40 49 83 F8 20
        }
    $botnet_info_struct_build = {
        44 8B CB 4C 8D 05 ?? ?? ?? ?? 48 8D ?? ?? ?? ?? ?? 48 8B CF FF 15 ?? ?? ?? ?? 48 63 D8 44 8B CD 48 8D 15 ?? ?? ?? ?? 48 8D 2D ?? ?? ?? ?? 4C 8B C5 48 8D 0C 5F FF 15 ?? ?? ?? ?? 48 63 C8 48 03 D9 E8 ?? ?? ?? ?? 48 8D 0C 5F 44 8B C8 4C 8b C5 48 8D 15 ?? ?? ?? ?? FF 15 ?? ?? ?? ?? 48 63 C8 48 03 D9 E8 ?? ?? ?? ?? 48 8D 0C 5F 44 8B C8 4C 8B C5 48 8D ?? ?? ?? ?? ?? FF 15 ?? ?? ?? ?? 48 63 C8 48 03 D9 48 8d 0C 5F E8 ?? ?? ?? ?? 48 03 D8 48 8D 0C 5F E8 ?? ?? ?? ??
    }
    condition:
        uint16(0) == 0x5a4d and
        ($conf_decrypt_algorithm or $botnet_info_struct_build)
}