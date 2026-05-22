package podlookup

// PodLookupResult holds the resolved pod information for a port match.
type PodLookupResult struct {
	PID          int
	ProcName     string
	ContainerID  string
	PodName      string
	PodNamespace string
	Deployment   string
	NetNS        string // e.g. "net:[4026532261]"
	Port         int
	LocalIP      string
	State        string
	Peers        []PeerConnection // ESTABLISHED peers in the same namespace touching Port
}

// PeerConnection describes a single non-LISTEN socket touching the target port
// within the resolved network namespace.
type PeerConnection struct {
	LocalIP    string
	LocalPort  int
	RemoteIP   string
	RemotePort int
	State      string
}

// NetNSEntry represents a unique network namespace with a representative PID.
type NetNSEntry struct {
	Inode string // unique netns inode (e.g. "4026532261")
	PID   int    // representative PID in this namespace
}
