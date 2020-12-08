package raft

import "eddy.org/raft/proto"

type VoteResult uint8

const (
	VotePending VoteResult = 1 + iota
	VoteLost
	VoteWon
)

type progressTracker struct {
	votes map[string]bool
}

func newProgressTracker() *progressTracker {
	return &progressTracker{votes: make(map[string]bool)}
}

func (p *progressTracker) reset() {
	p.votes = make(map[string]bool)
}

func (p *progressTracker) recodeVote(ip string, agree bool) {
	p.votes[ip] = agree
}

func (p *progressTracker) result(peers []*proto.Peer) VoteResult {
	if len(peers) == 0 {
		return VoteWon
	}

	ny := [2]int{}

	var missing int
	for _, id := range peers {
		v, ok := p.votes[id.Ip]
		if !ok {
			missing++
			continue
		}
		if v {
			ny[1]++
		} else {
			ny[0]++
		}
	}

	q := len(peers)/2 + 1
	if ny[1] >= q {
		return VoteWon
	}
	if ny[1]+missing >= q {
		return VotePending
	}
	return VoteLost
}
