package shared

// Color thresholds for warning states.
// When a state's count exceeds these values, it gets colored.
type StateWarning struct {
	Threshold int
	Color     string
	Reason    string
}

var StateWarnings = map[string]StateWarning{
	"TIME_WAIT":  {Threshold: 1000, Color: "yellow", Reason: "port exhaustion risk"},
	"CLOSE_WAIT": {Threshold: 100, Color: "red", Reason: "app not closing sockets"},
	"SYN_RECV":   {Threshold: 100, Color: "red", Reason: "possible SYN flood"},
	"FIN_WAIT1":  {Threshold: 100, Color: "yellow", Reason: "connection cleanup issues"},
}

func ShortStateName(state string) string {
	switch state {
	case "ESTABLISHED":
		return "EST"
	case "TIME_WAIT":
		return "TW"
	case "CLOSE_WAIT":
		return "CW"
	case "LISTEN":
		return "LS"
	case "SYN_SENT":
		return "SS"
	case "SYN_RECV":
		return "SR"
	case "FIN_WAIT1":
		return "FW1"
	case "FIN_WAIT2":
		return "FW2"
	case "LAST_ACK":
		return "LA"
	case "CLOSING":
		return "CLG"
	case "CLOSE":
		return "CLS"
	default:
		return state
	}
}
