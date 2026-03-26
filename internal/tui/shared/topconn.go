package shared

import "github.com/BlackMetalz/holyf-network/internal/collector"

type SortMode int

const (
	SortByBandwidth SortMode = iota
	SortByConns
	SortByPort
)

const (
	DefaultTopConnectionsPanelWidth = 120
	TopConnectionsGroupCap          = 20
)

func (m SortMode) Label() string {
	switch m {
	case SortByBandwidth:
		return "BW"
	case SortByConns:
		return "CONNS"
	case SortByPort:
		return "PORT"
	default:
		return "BW"
	}
}

func SortLabelWithDirection(mode SortMode, desc bool) string {
	dir := "ASC"
	if desc {
		dir = "DESC"
	}
	return mode.Label() + ":" + dir
}

type TopConnectionDirection int

const (
	TopConnectionIncoming TopConnectionDirection = iota
	TopConnectionOutgoing
)

func (d TopConnectionDirection) Label() string {
	if d == TopConnectionOutgoing {
		return "OUT"
	}
	return "IN"
}

func (d TopConnectionDirection) PanelTitle() string {
	if d == TopConnectionOutgoing {
		return " 1. Top Outgoing "
	}
	return " 1. Top Incoming "
}

func (d TopConnectionDirection) GroupPortHeader() string {
	if d == TopConnectionOutgoing {
		return "RPORTS"
	}
	return "PORTS"
}

func (d TopConnectionDirection) GroupPortLabel() string {
	if d == TopConnectionOutgoing {
		return "RPorts"
	}
	return "Ports"
}

func (d TopConnectionDirection) FilterMatchesPort(conn collector.Connection, port int) bool {
	if d == TopConnectionOutgoing {
		return conn.RemotePort == port
	}
	return conn.LocalPort == port
}

func (d TopConnectionDirection) SortPort(conn collector.Connection) int {
	if d == TopConnectionOutgoing {
		return conn.RemotePort
	}
	return conn.LocalPort
}
