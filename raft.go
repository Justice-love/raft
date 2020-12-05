package raft

import (
	"eddy.org/raft/proto"
	"math/rand"
	"sync"
	"time"
)

type Raft interface {
	Start()
	Ticket()
	Campaign()
	Advance()
	Step(message proto.RaftMessage)
	Receive() chan <- proto.RaftMessage
}

type Node struct {
	ip string
	term uint64
	vote string
	voteTerm uint64

	peers []*proto.Peer

	state proto.RaftState
	tick func()
	step stepFunc

	electionTicket int
	electionTicketTimeout int

	tickc chan struct{}
	recvc chan *proto.RaftMessage
	advancec chan *proto.RaftMessage
	lock *sync.Mutex
}

func (n *Node) Advance() {
	panic("implement me")
}

func (n *Node) Start() {
	n.ip = ExternalIP()
	if len(n.ip) == 0 {
		n.state = proto.RaftState_Watcher
		n.step = WatcherStep
	} else {
		n.state = proto.RaftState_Follower
		n.step = FollowerStep
		n.tick = n.tickElection
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	n.electionTicketTimeout = r.Intn(100) + 1
	n.electionTicket = 10
	n.term = 0
	//不能阻塞
	n.tickc = make(chan struct{}, 100)
	//需要阻塞
	n.recvc = make(chan *proto.RaftMessage)
	n.advancec = make(chan *proto.RaftMessage)

	go func(node *Node) {
		for {
			select {
			case <- n.tickc:
				n.Ticket()
			case msg := <- n.recvc:
				n.Step(msg)
			case msg := <- n.advancec:
				n.Step(msg)
			}
		}
	}(n)
	//尽量保证计时器精准
	go func(node *Node) {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				node.tick()
			}
		}
	}(n)
}

func (n *Node) Ticket() {
	n.tick()
}

func (n *Node) Campaign() {
	n.lock.Lock()
	defer n.lock.Unlock()
	if n.state == proto.RaftState_Leader || n.state == proto.RaftState_Watcher || len(n.vote) > 0{
		return
	}
	n.state = proto.RaftState_Candidate
	n.step = CandidateStep
	n.vote = n.ip
	n.term = n.term + 1
	n.voteTerm = n.term
	messages := make([]*proto.RaftMessage, 0)
	for _, p := range n.peers {
		message := &proto.RaftMessage{
			MessageType: proto.MessageType_MsgVote,
			From:        n.ip,
			To:          p.Ip,
			Term:        n.term,
			Peers:       n.peers,
		}
		messages = append(messages, message)
	}
	n.send(messages)
}

func (n *Node) Step(message *proto.RaftMessage) {
	n.step(n, message)
}

func (n *Node) Receive() chan <- *proto.RaftMessage{
	return n.recvc
}

type stepFunc func(r *Node, m *proto.RaftMessage)

func(n *Node) IsLeader() bool {
	return n.state == proto.RaftState_Leader
}

func (n *Node) becameLeader()  {
	n.lock.Lock()
	defer n.lock.Unlock()
	if n.state == proto.RaftState_Watcher {
		return
	}
	n.step = LeaderStep
	n.tick = n.tickHeartbeat
	n.electionTicket = 0
}

func (n *Node) becameFollower()  {
	n.lock.Lock()
	defer n.lock.Unlock()
	if n.state == proto.RaftState_Watcher {
		return
	}
	n.step = FollowerStep
	n.tick = n.tickElection
	n.electionTicket = 0
}

func FollowerStep(r *Node, m *proto.RaftMessage)  {
	switch m.MessageType {
	case proto.MessageType_MsgUp:
		self, from := updatePeer(r, m)
		directUpdateSelf(r, self)
		if len(from) > 0 {
			r.send([]*proto.RaftMessage{
				{
					MessageType: proto.MessageType_MsgUpRsp,
					From:        r.ip,
					To:          m.From,
					Term:        r.term,
					Peers:       r.peers,
				},
			})
		}
	case proto.MessageType_MsgUpRsp:
		if m.Term > r.term {
			r.becameFollower()
			r.reset(m.Term)
		}
		if updateSelf(r, m.Peers) && r.state == proto.RaftState_Leader {
			r.becameFollower()
		}
	case proto.MessageType_MsgHeartbeat:

	case proto.MessageType_MsgVote:
	}
}

func updateSelf(r *Node, from []*proto.Peer) bool {
	if len(from) == 0 {
		return false
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	temp := make([]*proto.Peer, 0)
	t:
	for _, fp := range from {
		if fp.Ip == r.ip {
			continue
		}
		for _, p := range r.peers {
			if fp.Ip == p.Ip {
				continue t
			}
		}
		temp = append(temp, fp)
	}
	if len(temp) > 0 {
		r.peers = append(r.peers, temp...)
		return true
	}
	return false

}

func directUpdateSelf(r *Node, self []*proto.Peer) {
	if len(self) == 0 {
		return
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	r.peers = append(r.peers, self...)

}

func (n *Node) reset(term uint64) {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.term = term
}

func updatePeer(r *Node, m *proto.RaftMessage) ([]*proto.Peer, []*proto.Peer) {
	self := append(r.peers, &proto.Peer{Ip:r.ip})
	from := append(m.Peers, &proto.Peer{Ip:m.From})
	selfMiss := make([]*proto.Peer, 0)
	fromMiss := make([]*proto.Peer, 0)
t:
	for _, s := range self {
		for _, f := range from {
			if s.Ip == f.Ip {
				continue t
			}
		}
		//self 不在 from中
		fromMiss = append(fromMiss, s)
	}

t2:
	for _, f := range from {
		for _, s := range self {
			if s.Ip == f.Ip {
				continue t2
			}
		}
		// from 不在 self 中
		selfMiss = append(selfMiss, f)
	}
	if m.Term > r.term ||
		len(selfMiss) > 1 ||
		(len(selfMiss) == 1 && selfMiss[0].Ip != m.From) {
		r.becameFollower()
		r.reset(m.Term)
	}

	return selfMiss, fromMiss
}

func LeaderStep(r *Node, m *proto.RaftMessage)  {
	switch m.MessageType {
	case proto.MessageType_MsgUp:
	case proto.MessageType_MsgUpRsp:
	case proto.MessageType_MsgHeartbeat:
	case proto.MessageType_MsgVote:
	}
}

func CandidateStep(r *Node, m *proto.RaftMessage)  {
	switch m.MessageType {
	case proto.MessageType_MsgUp:
	case proto.MessageType_MsgUpRsp:
	case proto.MessageType_MsgHeartbeat:
	case proto.MessageType_MsgVote:
	}
}

func WatcherStep(r *Node, m *proto.RaftMessage)  {

}

func (n *Node) tickElection() {
}

func (n *Node) tickHeartbeat() {
}

func (n *Node) send(messages []*proto.RaftMessage) {

}
