//go:build linux

package kernelapi

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
	"golang.org/x/sys/unix"
)

const (
	nftTableName = "holyf-network"
	nftTagPrefix = "holyf-network-peer-block:"
)

// NftFirewall implements Firewall using the nftables kernel API.
type NftFirewall struct{}

func (n *NftFirewall) BackendName() string { return "nftables" }

// NewNftFirewall creates a new nftables-based firewall manager.
func NewNftFirewall() (*NftFirewall, error) {
	fw := &NftFirewall{}
	if err := fw.ensureTable(); err != nil {
		return nil, fmt.Errorf("nft ensure table: %w", err)
	}
	return fw, nil
}

// ListBlockedPeers returns all peer blocks managed by holyf-network.
func (fw *NftFirewall) ListBlockedPeers() ([]PeerBlockSpec, error) {
	c, err := nftables.New()
	if err != nil {
		return nil, fmt.Errorf("nft conn: %w", err)
	}

	table := fw.findTable(c)
	if table == nil {
		return nil, nil
	}

	rules, err := c.GetRules(table, nil)
	if err != nil {
		return nil, fmt.Errorf("nft get rules: %w", err)
	}

	seen := make(map[string]bool)
	var result []PeerBlockSpec
	for _, r := range rules {
		spec, ok := parseUserData(r.UserData)
		if !ok {
			continue
		}
		key := fmt.Sprintf("%s:%d", spec.PeerIP, spec.LocalPort)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, spec)
	}
	return result, nil
}

// BlockPeer inserts DROP rules for peer IP on the target local port.
func (fw *NftFirewall) BlockPeer(spec PeerBlockSpec) error {
	c, err := nftables.New()
	if err != nil {
		return fmt.Errorf("nft conn: %w", err)
	}

	table := fw.findTable(c)
	if table == nil {
		return fmt.Errorf("nft table %q not found", nftTableName)
	}

	chains, err := c.ListChainsOfTableFamily(nftables.TableFamilyINet)
	if err != nil {
		return fmt.Errorf("nft list chains: %w", err)
	}

	var inputChain, outputChain *nftables.Chain
	for _, ch := range chains {
		if ch.Table.Name != nftTableName {
			continue
		}
		switch ch.Name {
		case "input":
			inputChain = ch
		case "output":
			outputChain = ch
		}
	}

	if inputChain == nil || outputChain == nil {
		return fmt.Errorf("nft chains not found in table %q", nftTableName)
	}

	userData := []byte(nftTagPrefix + strconv.Itoa(spec.LocalPort) + ":" + spec.PeerIP)
	ip := net.ParseIP(spec.PeerIP)
	if ip == nil {
		return fmt.Errorf("invalid peer IP: %s", spec.PeerIP)
	}

	// Input chain: match src IP = peerIP, dst port = localPort, DROP.
	c.AddRule(&nftables.Rule{
		Table:    table,
		Chain:    inputChain,
		Exprs:    buildMatchExprs(ip, spec.LocalPort, true),
		UserData: userData,
	})

	// Output chain: match dst IP = peerIP, src port = localPort, DROP.
	c.AddRule(&nftables.Rule{
		Table:    table,
		Chain:    outputChain,
		Exprs:    buildMatchExprs(ip, spec.LocalPort, false),
		UserData: userData,
	})

	if err := c.Flush(); err != nil {
		return fmt.Errorf("nft flush: %w", err)
	}
	return nil
}

// UnblockPeer removes previously inserted block rules matching the spec.
func (fw *NftFirewall) UnblockPeer(spec PeerBlockSpec) error {
	c, err := nftables.New()
	if err != nil {
		return fmt.Errorf("nft conn: %w", err)
	}

	table := fw.findTable(c)
	if table == nil {
		return nil
	}

	rules, err := c.GetRules(table, nil)
	if err != nil {
		return fmt.Errorf("nft get rules: %w", err)
	}

	tag := nftTagPrefix + strconv.Itoa(spec.LocalPort) + ":" + spec.PeerIP
	for _, r := range rules {
		if string(r.UserData) == string(tag) {
			if err := c.DelRule(r); err != nil {
				return fmt.Errorf("nft del rule: %w", err)
			}
		}
	}

	if err := c.Flush(); err != nil {
		return fmt.Errorf("nft flush: %w", err)
	}
	return nil
}

// ensureTable creates the nftables table and chains if they don't exist.
func (fw *NftFirewall) ensureTable() error {
	c, err := nftables.New()
	if err != nil {
		return err
	}

	table := c.AddTable(&nftables.Table{
		Family: nftables.TableFamilyINet,
		Name:   nftTableName,
	})

	inputPolicy := nftables.ChainPolicyAccept
	outputPolicy := nftables.ChainPolicyAccept

	c.AddChain(&nftables.Chain{
		Name:     "input",
		Table:    table,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookInput,
		Priority: nftables.ChainPriorityFilter,
		Policy:   &inputPolicy,
	})

	c.AddChain(&nftables.Chain{
		Name:     "output",
		Table:    table,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookOutput,
		Priority: nftables.ChainPriorityFilter,
		Policy:   &outputPolicy,
	})

	return c.Flush()
}

// findTable locates the holyf-network table.
func (fw *NftFirewall) findTable(c *nftables.Conn) *nftables.Table {
	tables, err := c.ListTablesOfFamily(nftables.TableFamilyINet)
	if err != nil {
		return nil
	}
	for _, t := range tables {
		if t.Name == nftTableName {
			return t
		}
	}
	return nil
}

// parseUserData extracts PeerBlockSpec from rule UserData.
// Format: "holyf-network-peer-block:<port>:<ip>"
func parseUserData(ud []byte) (PeerBlockSpec, bool) {
	s := string(ud)
	if !strings.HasPrefix(s, nftTagPrefix) {
		return PeerBlockSpec{}, false
	}
	rest := s[len(nftTagPrefix):]
	idx := strings.Index(rest, ":")
	if idx < 0 {
		return PeerBlockSpec{}, false
	}
	port, err := strconv.Atoi(rest[:idx])
	if err != nil {
		return PeerBlockSpec{}, false
	}
	ip := rest[idx+1:]
	if net.ParseIP(ip) == nil {
		return PeerBlockSpec{}, false
	}
	return PeerBlockSpec{PeerIP: ip, LocalPort: port}, true
}

// buildMatchExprs builds nftables match expressions for a TCP block rule.
// For input: match src IP = peerIP AND tcp dport = port, verdict DROP.
// For output (isInput=false): match dst IP = peerIP AND tcp sport = port, verdict DROP.
func buildMatchExprs(ip net.IP, port int, isInput bool) []expr.Any {
	var exprs []expr.Any

	// Determine IP version and offsets.
	ip4 := ip.To4()
	isIPv4 := ip4 != nil

	// Load IP header protocol field to check for TCP.
	// meta l4proto tcp
	exprs = append(exprs,
		&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     []byte{unix.IPPROTO_TCP},
		},
	)

	// Match IP address.
	if isIPv4 {
		var ipOffset uint32
		if isInput {
			ipOffset = 12 // src IP offset in IPv4 header
		} else {
			ipOffset = 16 // dst IP offset in IPv4 header
		}
		exprs = append(exprs,
			&expr.Payload{
				DestRegister: 1,
				Base:         expr.PayloadBaseNetworkHeader,
				Offset:       ipOffset,
				Len:          4,
			},
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     ip4,
			},
		)
	} else {
		ip16 := ip.To16()
		var ipOffset uint32
		if isInput {
			ipOffset = 8 // src IP offset in IPv6 header
		} else {
			ipOffset = 24 // dst IP offset in IPv6 header
		}
		exprs = append(exprs,
			&expr.Payload{
				DestRegister: 1,
				Base:         expr.PayloadBaseNetworkHeader,
				Offset:       ipOffset,
				Len:          16,
			},
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     ip16,
			},
		)
	}

	// Match TCP port.
	var portOffset uint32
	if isInput {
		portOffset = 2 // TCP destination port offset
	} else {
		portOffset = 0 // TCP source port offset
	}
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(port))
	exprs = append(exprs,
		&expr.Payload{
			DestRegister: 1,
			Base:         expr.PayloadBaseTransportHeader,
			Offset:       portOffset,
			Len:          2,
		},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     portBytes,
		},
	)

	// Verdict: DROP.
	exprs = append(exprs, &expr.Verdict{Kind: expr.VerdictDrop})

	return exprs
}

// Compile-time interface check.
var _ Firewall = (*NftFirewall)(nil)
