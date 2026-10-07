rule win_xwormmm_s1_6f74 {

    meta:
        author                    = "Johannes Bader"
        date                      = "2022-11-13"
        description               = "detects unpacked Xwormmm samples"
        hash1_md5                 = "6005e1ccaea62626a5481e09bbb653da"
        hash1_sha1                = "74138872ec0d0791b7f58eda8585250af40feaf9"
        hash1_sha256              = "7fc6a365af13150e7b1738129832ebd91f1010705b0ab0955a295e2c7d88be62"
        tlp                       = "TLP:WHITE"
        version                   = "v1.0"
        yarahub_author_email      = "yara@bin.re"
        yarahub_author_twitter    = "@viql"
        yarahub_license           = "CC BY-SA 4.0"
        yarahub_reference_md5     = "6005e1ccaea62626a5481e09bbb653da"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_uuid              = "6f74e598-0f7c-42f4-9730-1925d1b08ebe"

    strings:
        $str_01 = "Mutexx"
        $str_02 = "USBS"
        $str_03 = "_appMutex"
        $str_04 = "dTimer2"
        $str_05 = "dosstu"
        $str_06 = "nameee"
        $str_07 = "ruta"
        $str_08 = "usbSP"
        $str_09 = "GetEncoderInfo"
        $str_10 = "AppendOutputText"
        $str_11 = "capCreateCaptureWindowA"
        $str_12 = "capGetDriverDescriptionA"
        $str_13 = "MyProcess_ErrorDataReceived"
        $str_14 = "MyProcess_OutputDataReceived"
        $str_15 = "STOBS64"
        $str_16 = "keybd_event"
        $str_17 = "AES_Decryptor"
        $str_18 = "AES_Encryptor"
        $str_19 = "tickees"
        $str_20 = "INDATE"
        $str_21 = "GetHashT"
        $str_22 = "isDisconnected"

        $str_23   = "PING?" wide
        $str_24   = "IsInRole" wide
        $str_25   = "Select * from AntivirusProduct" wide
        $str_26   = "FileManagerSplitFileManagerSplit" wide
        $str_27   = "\nError: " wide
        $str_28   = "[Folder]" wide

        $str_29    = "XKlog.txt" wide
        $str_30    = "<Xwormmm>" wide
        $str_32    = "GfvaHzPAZuTqRREB" wide

    condition:
        uint16(0) == 0x5A4D and
        (
            20  of ($str*)
        )
}
