rule win32_mystic_stealer {
    meta:
        author = "Reedus0"
        description = "Rule for detecting Mystic Stealer malware"
        date = "2024-07-19"
        yarahub_reference_link = "https://malpedia.caad.fkie.fraunhofer.de/details/win.mystic_stealer"
        yarahub_reference_md5 = "1baba2d74f12915a3b89ecb883315008"
        yarahub_uuid = "288dfe16-1a9e-4d0f-8b2b-4ab80ffd15e9"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        malpedia_family = "win.mystic_stealer"
        version = "1"
    strings:
        $create_mutex_a = { F1 6F EB D6 }
        $get_last_error = { 16 8A 16 1C }
        $create_file_w = { 7B D8 E4 F0 }
        $get_system_windows_directory_a = { 3D 08 FE D2 }
        $get_volume_information_a = { 59 ED 0D 98 }
        $snprintf = { B6 BF 4F 53 }
    condition:
        uint16(0) == 0x5A4D and all of them
}