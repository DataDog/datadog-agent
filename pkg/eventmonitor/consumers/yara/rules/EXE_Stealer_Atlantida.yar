
rule EXE_Stealer_Atlantida
{
  meta:
    author = "Yashraj Solanki - Cyber Threat Intelligence Analyst at Bridewell"
    description = "Detects the Atlantida Stealer malware based on matched strings"
    source = "https://www.rapid7.com/blog/post/2024/01/17/whispers-of-atlantida-safeguarding-your-digital-treasure/, "
    hash = "07f5e74ebd8a4c7edd1812f4c766052239b7da74ca67fd75f143c1f833a4672b"
    date = "2024-01-20"
    yarahub_author_twitter = "@RustyNoob619"
    yarahub_reference_md5 = "f7c5ba27cb34c2dc76ee711a9e57b938"
    yarahub_uuid = "316da0c6-4f95-4a39-8f6c-2bfbafa9b002"
    yarahub_license = "CC0 1.0"
    yarahub_rule_matching_tlp = "TLP:WHITE"
    yarahub_rule_sharing_tlp = "TLP:WHITE"
  
  strings:
    $crypto1 = "Exodus"
    $crypto2 = "Binance"
    $crypto3 = "MetaMask"
    $crypto4 = "BinanceWallet"
    $crypto5 = "Phantom"
    $crypto6 = "sollet"
    $crypto7 = "MetaWallet"
    $crypto8 = "CardWallet"
    $crypto9 = "guildwallet" nocase
    $crypto10 = "TronWallet"
    $crypto11 = "CryptoAirdrops"
    $crypto12 = "Bitoke"
    $crypto13 = "Coin89"
    $crypto14 = "XDefiWallet"
    $crypto15 = "FreaksAxie"
    $crypto16 = "MathWallet"
    $crypto17 = "NiftyWallet"
    $crypto18 = "Guarda"
    $crypto19 = "EQUALWallet"
    $crypto20 = "BitAppWallet"
    $crypto21 = "iWallet"
    $crypto22 = "Wombat"
    $crypto23 = "MEW CX"
    $crypto24 = "Saturn Wallet"
    $crypto25 = "CloverWallet"
    $crypto26 = "LiqualityWallet"
    $crypto27 = "TerraStation"
    $crypto28 = "AuroWallet"
    $crypto29 = "Polymesh Wallet"
    $crypto30 = "ICONex"
    $crypto31 = "NaboxWallet"
    $crypto32 = "Temple"
    $crypto33 = "TezBox"
    $crypto34 = "CyanoWallet"
    $crypto35 = "OneKey"
    $crypto36 = "Leaf Wallet"
    $crypto38 = "BitClip"
    $crypto39 = "NashExtension"
    $crypto40 = "HyconLiteClient"
    $creds1 = "config.vdf"
    $creds2 = "loginusers.vdf"
    $creds3 = "User:"
    $creds4 = "Password:"
    $creds5 = "Host:"
    $othr1 = "Steam"
    $othr2 = "\\Telegram Desktop\\tdata"
    $othr3 = "\\Network\\Cookies"
    $othr4 = "encrypted_key"
    $othr5 = "password_manager"
    $othr6 = "\\Login Data"
    $othr7 = "\\Local Extension Settings\\"
    $othr8 = "\\History"
    $othr9 = "moz_cookies"
    $wide1 = "Wallets\\" wide
    $wide2 = "Browsers\\Tokens\\" wide
    $wide3 = "Browsers\\Cards\\" wide
    $wide4 = "Browsers\\Autofills\\" wide
    $wide5 = "Browsers\\History\\" wide
    $wide6 = "Browsers\\Cookies\\Cookies_" wide
    $wide7 = "Browsers\\BroweserInfo.txt" wide
    $wide8 = "Passwords.txt" wide
    $wide9 = "Log Information.txt" wide
    $wide10 = "All domain.txt" wide
    $wide11= "FileZilla\\Servers.txt" wide
    $wide12= "User Information.txt" wide
    $wide13= "Geo Information.txt" wide

  condition:
    uint16(0) == 0x5A4D
    and 30 of ($crypto*)
    and 2 of ($creds*)
    and 5 of ($othr*)
    and 7 of ($wide*)
}
