// Copyright 2018-present the CoreDHCP Authors. All rights reserved
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package main

/*
 * Sample DHCPv6 client to test on the local interface
 */

import (
	"flag"
	"net"

	"github.com/coredhcp/coredhcp/logger"
	"github.com/insomniacslk/dhcp/dhcpv6"
	"github.com/insomniacslk/dhcp/dhcpv6/client6"
	"github.com/insomniacslk/dhcp/iana"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/client4"
)

var log = logger.GetLogger("main")

func main() {
	flag.Parse()

	var macString string
	if len(flag.Args()) > 0 {
		macString = flag.Arg(0)
	} else {
		macString = "00:11:22:33:44:55"
	}

	c := client6.NewClient()
	c.LocalAddr = &net.UDPAddr{
		IP:   net.ParseIP("::1"),
		Port: 546,
	}
	c.RemoteAddr = &net.UDPAddr{
		IP:   net.ParseIP("::1"),
		Port: 547,
	}
	c.SimulateRelay = true
	c.RelayOptions = []dhcpv6.Option {dhcpv6.OptInterfaceID([]byte("router1.us-ca-sfba.prod.example.com:Eth12/1(Port12)")) }
	log.Printf("%+v", c)

	mac, err := net.ParseMAC(macString)
	if err != nil {
		log.Fatal(err)
	}
	duid := dhcpv6.Duid{
		Type:          dhcpv6.DUID_LLT,
		HwType:        iana.HWTypeEthernet,
		Time:          dhcpv6.GetTime(),
		LinkLayerAddr: mac,
	}

	conv, err := c.Exchange("eth0", dhcpv6.WithClientID(duid))
	for _, p := range conv {
		log.Print(p.Summary())
		if p.IsRelay() {
			relay := p.(*dhcpv6.RelayMessage)
			pp, err := relay.GetInnerMessage()
			if err == nil {
				log.Print("decapsulated " + pp.Summary())
			} else {
				log.Error(err)
			}
		}
	}
	if err != nil {
		log.Fatal(err)
	}
	do_dhcp4(macString)
}

func do_dhcp4(macString string) {
	//giaddr := net.ParseIP("0.0.0.0")   // use this if we want to get a response
	giaddr := net.ParseIP("10.99.99.1")  // use this if we want the server to allocate us an IP
	c := client4.NewClient()

	log.Printf("%+v", c)

	mac, err := net.ParseMAC(macString)
	if err != nil {
		log.Fatal(err)
	}

	rai := dhcpv4.OptRelayAgentInfo(
		dhcpv4.OptGeneric(dhcpv4.AgentCircuitIDSubOption, []byte("router1.us-ca-sfba.prod.example.com:Eth12/1(Port12)")),
	)

	conv, err := c.Exchange("eth0", dhcpv4.WithHwAddr(mac), dhcpv4.WithGatewayIP(giaddr), dhcpv4.WithOption(rai))
	for _, p := range conv {
		log.Print(p.Summary())
	}
	if err != nil {
		log.Fatal(err)
	}
}
