package raft

import (
	"eddy.org/raft/proto"
	"math/rand"
	"time"
)

type Raft interface {
	Start()
	Ticket()
	Campaign()
	Advance()
	Step(message proto.RaftMessage)
	Receive() chan<- proto.RaftMessage
}

var None = ""

type Node struct {
	ip       string
	term     uint64
	vote     string
	leader   string
	voteTerm uint64

	peers []*proto.Peer

	state proto.RaftState
	tick  func()
	step  stepFunc

	electionTicket        int
	electionTicketTimeout int

	heartbeat        int
	heartbeatTimeout int

	tickc    chan struct{}
	recvc    chan *proto.RaftMessage
	advancec chan *proto.RaftMessage

	tracker *progressTracker
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
	n.electionTicketTimeout = r.Intn(50) + 1
	n.electionTicket = 10
	n.term = 0
	n.tickc = make(chan struct{}, 100)
	n.recvc = make(chan *proto.RaftMessage)
	n.advancec = make(chan *proto.RaftMessage)
	n.tracker = newProgressTracker()
	n.heartbeatTimeout = 25

	go func(node *Node) {
		for {
			select {
			case <-n.tickc:
				n.Ticket()
			case msg := <-n.recvc:
				n.Step(msg)
			case msg := <-n.advancec:
				n.Step(msg)
			}
		}
	}(n)
	go func(node *Node) {
		ticker := time.NewTicker(100 * time.Millisecond)
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

	if n.state == proto.RaftState_Leader || n.state == proto.RaftState_Watcher || len(n.vote) > 0 {
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
	if message.Term > n.term && message.MessageType != proto.MessageType_MsgVote {
		n.becameFollower(message.Term, message.From)
	}

	switch message.MessageType {
	case proto.MessageType_MsgHub:
		if n.state != proto.RaftState_Leader {
			n.campaign()
		}
	default:
		n.step(n, message)

	}
}

func (n *Node) Receive() chan<- *proto.RaftMessage {
	return n.recvc
}

type stepFunc func(r *Node, m *proto.RaftMessage)

func (n *Node) IsLeader() bool {
	return n.state == proto.RaftState_Leader
}

func (n *Node) becameLeader() {

	if n.state == proto.RaftState_Watcher {
		return
	}
	n.step = LeaderStep
	n.tick = n.tickHeartbeat
	n.electionTicket = 0
}

func (r *Node) becomeCandidate() {
	if r.state == proto.RaftState_Leader || r.state == proto.RaftState_Watcher {
		return
	}
	r.step = CandidateStep
	r.reset(r.term + 1)
	r.tick = r.tickElection
	r.vote = r.ip
	r.state = proto.RaftState_Candidate
	r.leader = None

}

func (n *Node) becameFollower(term uint64, leader string) {

	if n.state == proto.RaftState_Watcher {
		return
	}
	n.step = FollowerStep
	n.tick = n.tickElection
	n.electionTicket = 0
	n.term = term
	if len(leader) > 0 {
		n.leader = leader
	}
}

func FollowerStep(r *Node, m *proto.RaftMessage) {
	switch m.MessageType {
	case proto.MessageType_MsgUp:
		if m.From == r.ip {
			return
		}
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
		if updateSelf(r, m.Peers) && r.state == proto.RaftState_Leader {
			r.becameFollower(r.term, None)
		}
	case proto.MessageType_MsgHeartbeat:
		if m.From == r.ip {
			return
		}
		r.send([]*proto.RaftMessage{
			{
				MessageType: proto.MessageType_MsgHeartbeatRsp,
				From:        r.ip,
				To:          m.From,
				Term:        r.term,
				Peers:       r.peers,
			},
		})
		_ = updateSelf(r, m.Peers)
	case proto.MessageType_MsgVote:
		if canVote(r, m) {
			r.send([]*proto.RaftMessage{
				{
					MessageType: proto.MessageType_MsgVoteRsp,
					From:        r.ip,
					To:          m.From,
					Term:        r.term,
					Peers:       r.peers,
				},
			})
			r.electionTicket = 0
			r.vote = m.From
		}
	}
}

func canVote(r *Node, m *proto.RaftMessage) bool {
	return r.vote == m.From ||
		(len(r.vote) == 0 && len(r.leader) == 0) ||
		(m.Term > r.term && m.MessageType == proto.MessageType_MsgVote)
}

func updateSelf(r *Node, from []*proto.Peer) bool {
	if len(from) == 0 {
		return false
	}
	if len(from) == 1 && from[0].Ip == r.ip {
		return false
	}
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
	r.peers = append(r.peers, self...)

}

func (n *Node) reset(term uint64) {

	n.term = term
}

func updatePeer(r *Node, m *proto.RaftMessage) ([]*proto.Peer, []*proto.Peer) {
	self := append(r.peers, &proto.Peer{Ip: r.ip})
	from := append(m.Peers, &proto.Peer{Ip: m.From})
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
	if (len(selfMiss) == 1 && selfMiss[0].Ip != m.From) ||
		len(selfMiss) > 1 {
		r.becameFollower(r.term, None)
	}
	return selfMiss, fromMiss
}

func LeaderStep(r *Node, m *proto.RaftMessage) {
	switch m.MessageType {
	case proto.MessageType_MsgUp:
		if m.From == r.ip {
			return
		}
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
		if updateSelf(r, m.Peers) && r.state == proto.RaftState_Leader {
			r.becameFollower(r.term, None)
		}
	case proto.MessageType_MsgVote:
		if canVote(r, m) {
			r.send([]*proto.RaftMessage{
				{
					MessageType: proto.MessageType_MsgVoteRsp,
					From:        r.ip,
					To:          m.From,
					Term:        r.term,
					Peers:       r.peers,
				},
			})
			r.electionTicket = 0
			r.vote = m.From
		}
	case proto.MessageType_MsgHeartbeatRsp:
		for _, one := range r.peers {
			if one.Ip == m.From {
				one.Active = true
			}
		}
	case proto.MessageType_MsgHeartbeat:
		r.send([]*proto.RaftMessage{
			{
				MessageType: proto.MessageType_MsgHeartbeat,
				From:        r.ip,
				Term:        r.term,
				Peers:       r.peers,
			},
		})
	}
}

func CandidateStep(r *Node, m *proto.RaftMessage) {
	switch m.MessageType {
	case proto.MessageType_MsgUp:
		if m.From == r.ip {
			return
		}
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
		if updateSelf(r, m.Peers) && r.state == proto.RaftState_Leader {
			r.becameFollower(r.term, None)
		}
	case proto.MessageType_MsgHeartbeat:
		if m.From == r.ip {
			return
		}
		r.send([]*proto.RaftMessage{
			{
				MessageType: proto.MessageType_MsgHeartbeatRsp,
				From:        r.ip,
				To:          m.From,
				Term:        r.term,
				Peers:       r.peers,
			},
		})
		r.becameFollower(r.term, m.From)
		_ = updateSelf(r, m.Peers)
	case proto.MessageType_MsgVote:
		if canVote(r, m) {
			r.send([]*proto.RaftMessage{
				{
					MessageType: proto.MessageType_MsgVoteRsp,
					From:        r.ip,
					To:          m.From,
					Term:        r.term,
					Peers:       r.peers,
				},
			})
			r.electionTicket = 0
			r.vote = m.From
		}
	case proto.MessageType_MsgVoteRsp:
		r.tracker.recodeVote(m.From, m.Reject)
		result := r.tracker.result(r.peers)
		if result == VoteWon {
			r.becameLeader()
		} else if result == VoteLost {
			r.becameFollower(r.term, None)
		}
	}
}

func WatcherStep(r *Node, m *proto.RaftMessage) {
	return
}

func (n *Node) pastElectionTimeout() bool {
	return n.electionTicket >= n.electionTicketTimeout
}

func (n *Node) tickElection() {
	n.electionTicket++

	if n.promotable() && n.pastElectionTimeout() {
		n.electionTicket = 0
		n.Step(&proto.RaftMessage{
			MessageType: proto.MessageType_MsgHub,
			From:        n.ip,
			Term:        n.term,
			Peers:       n.peers,
		})
	}
}

func (n *Node) promotable() bool {
	return n.state != proto.RaftState_Leader
}

func (n *Node) tickHeartbeat() {
	if n.state != proto.RaftState_Leader {
		return
	}
	if n.heartbeat >= n.heartbeatTimeout {
		n.heartbeat = 0
		n.Step(&proto.RaftMessage{
			MessageType: proto.MessageType_MsgHeartbeat,
			From:        n.ip,
			Term:        n.term,
			Peers:       n.peers,
		})
	}
}

func (n *Node) campaign() {
	if !n.promotable() {
		return
	}
	n.becomeCandidate()
	n.send([]*proto.RaftMessage{{
		MessageType: proto.MessageType_MsgVote,
		From:        n.ip,
		Term:        n.term,
		Peers:       n.peers,
	}})
}

func (n *Node) send(messages []*proto.RaftMessage) {

}
