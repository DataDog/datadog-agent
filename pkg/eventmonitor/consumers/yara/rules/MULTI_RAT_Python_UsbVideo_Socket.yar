rule MULTI_RAT_Python_UsbVideo_Socket {
    meta:
        description = "RAT Python : camouflage pilote 'usbvideo', C2 base64+XOR (cle 'w3x'), socket brut + API HTTP, launcher VBS silencieux. C2 decode : 198.37.111.206"
        author      = "Marjoriefort"
        yarahub_reference_md5 = "3abb157e20d940a8e7032cb40e6f7913"
        date        = "2026-09-19"
        reference   = "Veille glissante-2026-09-19_08 / zip 0d0cf937 (arbre source complet)"
        confidence  = "high"
        yarahub_uuid = "51de89f4-a249-4c98-937c-bf82f7e68437"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $a = "usbvideo.connection" ascii
        $b = "log_server_connected" ascii
        $c = "log_server_disconnected" ascii
        $d = "_XOR_KEY" ascii
        $e = "ENC_SOCKET_HOST" ascii
    condition:
        2 of ($a, $b, $c, $d, $e)
}
