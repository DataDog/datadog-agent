package netpath

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/netpath/traceroute"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"sort"
	"strings"
)

func getHops(options traceroute.TracerouteOptions, times int, err error, host string) [][]traceroute.TracerouteHop {
	log.Debugf("options %+v\n\n", options)
	allhops := [][]traceroute.TracerouteHop{}
	for i := 0; i < times; i++ {
		hops := []traceroute.TracerouteHop{}
		c := make(chan traceroute.TracerouteHop, 0)
		go func() {
			for {
				hop, ok := <-c
				if !ok {
					fmt.Println()
					return
				}
				printHop(hop)
				hops = append(hops, hop)
			}
		}()

		//fmt.Printf("== Round %d ==\n", i)
		//time.Sleep(50 * time.Millisecond)
		_, err = traceroute.Traceroute(host, &options, c)
		if err != nil {
			log.Debugf("Error: %s", err)
		}
		allhops = append(allhops, hops)
	}
	return allhops
}

func printHops(allhops [][]traceroute.TracerouteHop) {
	//for _, hop := range hops {
	//	printHop(hop)
	//}
	combinedHops := []traceroute.TracerouteHop{}
	for _, hops := range allhops {
		combinedHops = append(combinedHops, hops...)
	}
	replies := make(map[int][]traceroute.TracerouteHop)
	for _, reply := range combinedHops {
		replies[reply.TTL] = append(replies[reply.TTL], reply)
	}

	hops := []int{}
	for hop := range replies {
		hops = append(hops, hop)
	}
	sort.Ints(hops)
	for _, hop := range hops {
		replyList := replies[hop]
		//fmt.Printf("%d\n", hopTTL)
		prevAddr := ""
		prevTTL := 0
		hopByAddr := make(map[[4]byte][]traceroute.TracerouteHop)
		for _, hop := range replyList {
			hopByAddr[hop.Address] = append(hopByAddr[hop.Address], hop)
		}
		for _, hops := range hopByAddr {
			for _, hop := range hops {
				addr := fmt.Sprintf("%v.%v.%v.%v", hop.Address[0], hop.Address[1], hop.Address[2], hop.Address[3])
				hostOrAddr := addr
				if hop.Host != "" {
					hostOrAddr = hop.Host
				}
				printAddr := fmt.Sprintf("%v (%v)", hostOrAddr, addr)
				if hop.Success {
					if hostOrAddr == prevAddr {
						fmt.Printf(" %v", hop.ElapsedTime)
					} else {
						ttl := fmt.Sprintf("%d", hop.TTL)
						if hop.TTL == prevTTL {
							ttl = strings.Repeat(" ", len(ttl))
						}
						fmt.Printf("%s  %v  %v", ttl, printAddr, hop.ElapsedTime)
						prevTTL = hop.TTL
					}
				} else {
					fmt.Printf("   *")
				}
				prevAddr = hostOrAddr
			}
			fmt.Printf("\n")
		}

	}
}

func printHop(hop traceroute.TracerouteHop) {
	addr := fmt.Sprintf("%v.%v.%v.%v", hop.Address[0], hop.Address[1], hop.Address[2], hop.Address[3])
	hostOrAddr := addr
	if hop.Host != "" {
		hostOrAddr = hop.Host
	}
	if hop.Success {
		log.Debugf("%-3d %v (%v)  %v\n", hop.TTL, hostOrAddr, addr, hop.ElapsedTime)
	} else {
		log.Debugf("%-3d *\n", hop.TTL)
	}
}
