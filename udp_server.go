package raft

import (
	"eddy.org/raft/proto"
	"encoding/json"
	"fmt"
	"net"
)

const (
	localePort = ":18829"
	protocol   = "udp4"
	limit      = 2048
)

func Server() {
	udpAddr, err := net.ResolveUDPAddr(protocol, localePort)
	if err != nil {
		fmt.Println("Wrong Address")
		return
	}
	pc, err := net.ListenUDP(protocol, udpAddr)
	if err != nil {
		panic(err)
	}
	defer pc.Close()

	for {
		display(pc)
	}
}

func display(conn *net.UDPConn) {

	buf := make([]byte, limit)
	n, source, err := conn.ReadFrom(buf)
	if err != nil {
		fmt.Println("Error Reading")
		return
	}
	//fmt.Printf("date %s\n", buf[0:n])
	//fmt.Println("Package Done")
	msg := fmt.Sprintf("%s", buf)
	var raftMessage *proto.RaftMessage
	err = json.Unmarshal(buf, raftMessage)
	if err != nil {
		return
	}

}

