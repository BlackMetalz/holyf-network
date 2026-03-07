package collector

import "testing"

func TestMergeConntrackHostFlowsAddsMissingHostFacingTuple(t *testing.T) {
	t.Parallel()

	flows := []ConntrackFlow{
		{
			State: "ESTABLISHED",
			Orig: FlowTuple{
				SrcIP:   "172.25.110.116",
				SrcPort: 49410,
				DstIP:   "172.25.110.76",
				DstPort: 3306,
			},
			Reply: FlowTuple{
				SrcIP:   "172.20.0.2",
				SrcPort: 3306,
				DstIP:   "172.25.110.116",
				DstPort: 49410,
			},
		},
	}
	localSet := map[string]struct{}{
		"172.25.110.76": {},
	}

	got := mergeConntrackHostFlowsWithLocalSet(nil, flows, localSet)
	if len(got) != 1 {
		t.Fatalf("expected 1 synthetic conn, got=%d", len(got))
	}
	if got[0].LocalIP != "172.25.110.76" || got[0].LocalPort != 3306 {
		t.Fatalf("unexpected local endpoint: %+v", got[0])
	}
	if got[0].RemoteIP != "172.25.110.116" || got[0].RemotePort != 49410 {
		t.Fatalf("unexpected remote endpoint: %+v", got[0])
	}
	if got[0].State != "ESTABLISHED" {
		t.Fatalf("expected state ESTABLISHED, got=%q", got[0].State)
	}
}

func TestMergeConntrackHostFlowsSkipsDuplicateExistingTuple(t *testing.T) {
	t.Parallel()

	existing := []Connection{
		{
			LocalIP:    "172.25.110.76",
			LocalPort:  3306,
			RemoteIP:   "172.25.110.116",
			RemotePort: 49410,
			State:      "ESTABLISHED",
		},
	}
	flows := []ConntrackFlow{
		{
			State: "ESTABLISHED",
			Orig: FlowTuple{
				SrcIP:   "172.25.110.116",
				SrcPort: 49410,
				DstIP:   "172.25.110.76",
				DstPort: 3306,
			},
			Reply: FlowTuple{
				SrcIP:   "172.20.0.2",
				SrcPort: 3306,
				DstIP:   "172.25.110.116",
				DstPort: 49410,
			},
		},
	}
	localSet := map[string]struct{}{
		"172.25.110.76": {},
	}

	got := mergeConntrackHostFlowsWithLocalSet(existing, flows, localSet)
	if len(got) != 1 {
		t.Fatalf("expected duplicate tuple to be skipped, got=%d", len(got))
	}
}

func TestMergeConntrackHostFlowsIgnoresFlowsOutsideLocalHost(t *testing.T) {
	t.Parallel()

	flows := []ConntrackFlow{
		{
			State: "ESTABLISHED",
			Orig: FlowTuple{
				SrcIP:   "10.0.0.10",
				SrcPort: 50000,
				DstIP:   "10.0.0.11",
				DstPort: 443,
			},
			Reply: FlowTuple{
				SrcIP:   "10.0.0.11",
				SrcPort: 443,
				DstIP:   "10.0.0.10",
				DstPort: 50000,
			},
		},
	}
	localSet := map[string]struct{}{
		"172.25.110.76": {},
	}

	got := mergeConntrackHostFlowsWithLocalSet(nil, flows, localSet)
	if len(got) != 0 {
		t.Fatalf("expected 0 synthetic conns for non-local flow, got=%d", len(got))
	}
}

func TestExtendLocalSetFromConnectionsAddsLocalIPs(t *testing.T) {
	t.Parallel()

	set := map[string]struct{}{}
	conns := []Connection{
		{LocalIP: "172.25.110.76"},
		{LocalIP: "::ffff:10.0.0.10"},
	}

	extendLocalSetFromConnections(set, conns)
	if _, ok := set["172.25.110.76"]; !ok {
		t.Fatalf("expected local ip from connections to be added")
	}
	if _, ok := set["10.0.0.10"]; !ok {
		t.Fatalf("expected normalized mapped ipv4 to be added")
	}
}
