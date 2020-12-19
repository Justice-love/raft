package raft

import (
	"eddy.org/raft/proto"
	"encoding/json"
	"fmt"
	"net"
)

const (
	localeServerPort = ":18829"
	localeFromPort   = ":18821"
	protocol         = "udp4"
	limit            = 2048
	broadcast        = "255.255.255.255"
)

func Server() {
	raft := &Node{}
	raft.Start()
	udpAddr, err := net.ResolveUDPAddr(protocol, localeServerPort)
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
		display(raft, pc)
	}
}

func display(raft *Node, conn *net.UDPConn) {

	buf := make([]byte, limit)
	_, _, err := conn.ReadFrom(buf)
	if err != nil {
		panic("read error")
	}
	var raftMessage *proto.RaftMessage
	err = json.Unmarshal(buf, raftMessage)
	if err != nil {
		return
	}
	raft.Receive() <- raftMessage

}

func sendOne(msg *proto.RaftMessage, ip string) {
	sendUdp(msg, ip)
}

func sendAll(msg *proto.RaftMessage) {
	sendUdp(msg, broadcast)
}

func sendUdp(msg *proto.RaftMessage, ip string) {
	local, err := net.ResolveUDPAddr("udp4", fmt.Sprint(ExternalIP(), localeFromPort))
	if err != nil {
		panic(err)
	}
	remote, err := net.ResolveUDPAddr("udp4", fmt.Sprint(ip, localeServerPort))
	if err != nil {
		panic(err)
	}
	list, err := net.DialUDP("udp4", local, remote)
	if err != nil {
		panic(err)
	}
	defer list.Close()

	byteMsg, _ := json.Marshal(msg)
	_, err = list.Write(byteMsg)
	if err != nil {
		panic(err)
	}
}
