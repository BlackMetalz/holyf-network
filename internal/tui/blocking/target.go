package blocking

import "github.com/BlackMetalz/holyf-network/internal/actions"

type PeerKillTarget struct {
	PeerIP    string
	LocalPort int
	Count     int
}

func (t PeerKillTarget) ToSpec() actions.PeerBlockSpec {
	return actions.PeerBlockSpec{
		PeerIP:    t.PeerIP,
		LocalPort: t.LocalPort,
	}
}
