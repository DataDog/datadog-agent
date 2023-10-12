package testdns

//func TestMe(t *testing.T) {
//	srv := NewServer()
//
//	srv.Start()
//
//	var dnsClient dns.Client
//
//	q := dns.Msg{
//		Question: []dns.Question{{
//			Name:   "good.com.",
//			Qclass: dns.ClassINET,
//			Qtype:  dns.TypeA,
//		}},
//	}
//	a, _, err := dnsClient.Exchange(&q, "192.1.1.1:53")
//	fmt.Println(a)
//
//	require.NoError(t, err)
//
//	fmt.Println("here")
//}
