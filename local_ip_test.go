package raft

import (
	"fmt"
	"net"
	"testing"
)

func TestExternalIP(t *testing.T) {
	result := ExternalIP()
	fmt.Println(result)
}

func TestUdp(t *testing.T) {
	local, err := net.ResolveUDPAddr("udp4", "10.94.62.44:8821")
	if err != nil {
		panic(err)
	}
	remote, err := net.ResolveUDPAddr("udp4", "255.255.255.255:8829")
	if err != nil {
		panic(err)
	}
	list, err := net.DialUDP("udp4", local, remote)
	if err != nil {
		panic(err)
	}
	defer list.Close()

	_, err = list.Write([]byte("data to transmit"))
	if err != nil {
		panic(err)
	}
}
