package main

import (
	"eddy.org/raft"
	"fmt"
	"net"
	"time"
)

func main() {
	fmt.Println(raft.ExternalIP())
	fmt.Println("test")
	go raft.Server()
	time.Sleep(1 * time.Second)
	local, err := net.ResolveUDPAddr("udp4", fmt.Sprint(raft.ExternalIP(), ":18821"))
	if err != nil {
		panic(err)
	}
	remote, err := net.ResolveUDPAddr("udp4", "255.255.255.255:18829")
	if err != nil {
		panic(err)
	}
	list, err := net.DialUDP("udp4", local, remote)
	if err != nil {
		panic(err)
	}
	defer list.Close()

	_, err = list.Write([]byte(raft.ExternalIP() + " data to transmit"))
	if err != nil {
		panic(err)
	}
	time.Sleep(2 * time.Second)
}
