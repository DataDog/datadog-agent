rule win_aurora_stealer_a_706a {

    meta:
        author                    = "Johannes Bader"
        date                      = "2022-12-14"
        description               = "detects Aurora Stealer samples"
        hash1_md5                 = "51c153501e991f6ce4901e6d9578d0c8"
        hash1_sha1                = "3816f17052b28603855bde3e57db77a8455bdea4"
        hash1_sha256              = "c148c449e1f6c4c53a7278090453d935d1ab71c3e8b69511f98993b6057f612d"
        hash2_md5                 = "65692e1d5b98225dbfb1b6b2b8935689"
        hash2_sha1                = "0b51765c175954c9e47c39309e020bcb0f90b783"
        hash2_sha256              = "5a42aa4fc8180c7489ce54d7a43f19d49136bd15ed7decf81f6e9e638bdaee2b"
        malpedia_family           = "win.aurora_stealer"
        tlp                       = "TLP:WHITE"
        version                   = "v1.0"
        yarahub_author_email      = "yara@bin.re"
        yarahub_author_twitter    = "@viql"
        yarahub_license           = "CC BY-SA 4.0"
        yarahub_reference_md5     = "51c153501e991f6ce4901e6d9578d0c8"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_uuid              = "706a5977-69fb-44ae-bfa7-f61e214148e7"

    strings:

        $str_func_01 = "main.(*DATA_BLOB).ToByteArray"
        $str_func_02 = "main.Base64Encode"
        $str_func_03 = "main.Capture"
        $str_func_04 = "main.CaptureRect"
        $str_func_05 = "main.ConnectToServer"
        $str_func_06 = "main.CreateImage"
        $str_func_07 = "main.FileExsist"
        $str_func_08 = "main.GetDisplayBounds"
        $str_func_09 = "main.GetInfoUser"
        $str_func_10 = "main.GetOS"
        $str_func_11 = "main.Grab"
        $str_func_12 = "main.MachineID"
        $str_func_13 = "main.NewBlob"
        $str_func_14 = "main.NumActiveDisplays"
        $str_func_15 = "main.PathTrans"
        $str_func_16 = "main.SendToServer_NEW"
        $str_func_17 = "main.SetUsermame"
        $str_func_18 = "main.Zip"
        $str_func_19 = "main.base64Decode"
        $str_func_20 = "main.countupMonitorCallback"
        $str_func_21 = "main.enumDisplayMonitors"
        $str_func_22 = "main.getCPU"
        $str_func_23 = "main.getDesktopWindow"
        $str_func_24 = "main.getGPU"
        $str_func_25 = "main.getMasterKey"
        $str_func_26 = "main.getMonitorBoundsCallback"
        $str_func_27 = "main.getMonitorRealSize"
        $str_func_28 = "main.sysTotalMemory"
        $str_func_29 = "main.xDecrypt"

        $str_type_01 = "type..eq.main.Browser_G"
        $str_type_02 = "type..eq.main.STRUSER"
        $str_type_03 = "type..eq.main.Telegram_G"
        $str_type_04 = "type..eq.main.Crypto_G"
        $str_type_05 = "type..eq.main.ScreenShot_G"
        $str_type_06 = "type..eq.main.FileGrabber_G"
        $str_type_07 = "type..eq.main.FTP_G"
        $str_type_08 = "type..eq.main.Steam_G"
        $str_type_09 = "type..eq.main.DATA_BLOB"
        $str_type_10 = "type..eq.main.Grabber"

        $varia_01 = "\\User Data\\Local State"
        $varia_02 = "\\\\Opera Stable\\\\Local State"
        $varia_03 = "Reconnect 1"
        $varia_04 = "@ftmone"
        $varia_05 = "^user^"
        $varia_06 = "wmic path win32_VideoController get name"
        $varia_07 = "\\AppData\\Roaming\\Telegram Desktop\\tdata"
        $varia_08 = "C:\\Windows.old\\Users\\"
        $varia_09 = "ScreenShot"
        $varia_10 = "Crypto"

    condition:
        uint16(0) == 0x5A4D and
        (
            32 of ($str_*) or
            9 of ($varia_*)
        )
}
