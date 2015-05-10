package probe

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/internal/iana"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

func dnsResolve(host string) (net.Addr, error) {
	// Check if host is already an IP address
	if ip := net.ParseIP(host); ip != nil {
		return &net.IPAddr{IP: ip}, nil
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	if ips[0].To16() != nil && ips[0].To4() != nil {
		return &net.IPAddr{IP: ips[0]}, nil
	}
	return nil, errors.New("no A or AAAA record found")
}

func Ping(host string) (time.Duration, error) {
	zeroTime := 0 * time.Second

	dst, err := dnsResolve(host)
	if err != nil {
		return zeroTime, err
	}

	var listen, network string
	var icmpType icmp.Type
	ip := dst.(*net.IPAddr)
	if ip.IP.To4() != nil {
		listen = "0.0.0.0"
		network = "ip4:icmp"
		icmpType = ipv4.ICMPTypeEcho
	} else if ip.IP.To16() != nil && ip.IP.To4() != nil {
		listen = "::"
		network = "ip6:ipv6-icmp"
		icmpType = ipv6.ICMPTypeEchoRequest
	}

	c, err := icmp.ListenPacket(network, listen)
	if err != nil {
		log.Printf("Could not open %s socket: %q\n", network, err)
		return zeroTime, err
	}
	defer c.Close()

	// Data is just the word "ping" 14 times (to make 56 bytes)
	msg := icmp.Message{
		Type: icmpType,
		Code: 0,
		Body: &icmp.Echo{
			ID:   os.Getpid() & 0xffff,
			Seq:  1 << 1,
			Data: []byte(strings.Repeat("ping", 14)),
		},
	}

	bmsg, err := msg.Marshal(nil)
	if err != nil {
		fmt.Println(err)
		return zeroTime, err
	}
	// Mark when we sent the packet
	t0 := time.Now()
	if n, err := c.WriteTo(bmsg, dst); err != nil {
		fmt.Println(err)
		return zeroTime, err
	} else if n != len(bmsg) {
		fmt.Printf("got %v; want %v\n", n, len(bmsg))
		return zeroTime, err
	}

	readBytes := make([]byte, 1500)
	n, peer, err := c.ReadFrom(readBytes)
	if err != nil {
		fmt.Println(err)
		return zeroTime, err
	}
	readMsg, err := icmp.ParseMessage(iana.ProtocolICMP, readBytes[:n])
	if err != nil {
		fmt.Println(err)
		return zeroTime, err
	}
	switch readMsg.Type {
	case ipv4.ICMPTypeEchoReply, ipv6.ICMPTypeEchoReply:
		log.Printf("got reply from %v\n", peer)
	default:
		log.Printf("got %+v; want echo reply\n", readMsg)
	}
	// Mark when we finished receiving
	t1 := time.Now()
	// Return the duration
	return t1.Sub(t0), nil
}
