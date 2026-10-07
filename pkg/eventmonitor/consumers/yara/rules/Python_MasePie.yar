rule Python_MasePie
{
  meta:
    author = "Yashraj Solanki - Cyber Threat Intelligence Analyst at Bridewell"
    description = "Detects the Masepie malware Python script based on matched strings"
    author = "Yashraj Solanki - Cyber Threat Intelligence Analyst at Bridewell"
    source = "https://cert.gov.ua/article/6276894"
    hash = "18f891a3737bb53cd1ab451e2140654a376a43b2d75f6695f3133d47a41952b6"
    date = "2024-01-19"
    yarahub_author_twitter = "@RustyNoob619"
    yarahub_reference_md5 = "47f4b4d8f95a7e842691120c66309d5b"
    yarahub_uuid = "21490ae6-79ce-4fe2-89bf-c4ea66931336"
    yarahub_license = "CC0 1.0"
    yarahub_rule_matching_tlp = "TLP:WHITE"
    yarahub_rule_sharing_tlp = "TLP:WHITE"
    malpedia_family = "py.masepie"
    
  strings:
    $msg1 = "message == 'check'"
    $msg2 = "message == 'send_file'"
    $msg3 = "message == 'get_file'"
    $clnt1 = "client.sendall(enc_answer)"
    $clnt2 = "client.recv(1024).decode()"
    $clnt3 = "client.sendall(bytes_enc)"
    $clnt4 = "client.send(okenc)"
    $clnt5 = "client.send(enc_answ)"
    $clnt6 = "client.send(user.encode('ascii'))"
    $clnt7 = "client.recv(1024)"
    $clnt8 = "client2.send('Error transporting file'.encode())"
    $clnt9 = "client2.recv(BUFFER_SIZE)"
    $clnt10 = "client2.send(ok_enc)"
    $othr1 = "enc_mes('ok', k)"
    $othr2 = "receive_file_thread.start()"
    $othr3 = "threading.Thread(target=receive_file)"
    $othr4 = "dec_mes(enc_received, k).decode()"
    $othr5 = "socket.socket(socket.AF_INET, socket.SOCK_STREAM)"
    $othr6 = "cypher.encrypt(pad(mes, cypher_block))"
    $othr7 = "ES.new(key.encode(), AES.MODE_CBC, key.encode())"

  condition:
    all of ($msg*)
    and 6 of ($clnt*)
    and 4 of ($othr*)
}
