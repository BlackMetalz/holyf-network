package tui

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/BlackMetalz/holyf-network/internal/collector"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	tracePacketPageForm     = "trace-packet-form"
	tracePacketPageProgress = "trace-packet-progress"
	tracePacketPageResult   = "trace-packet-result"

	tracePacketDefaultDurationSec = 10
	tracePacketMaxDurationSec     = 60
	tracePacketDefaultPacketCap   = 2000
	tracePacketMaxPacketCap       = 20000
	tracePacketSampleLineLimit    = 24

	tracePacketSavedDir  = "/tmp/holyf-network-captures"
	tracePacketStopGrace = 1500 * time.Millisecond
)

type tracePacketScope int

const (
	traceScopePeerPort tracePacketScope = iota
	traceScopePeerOnly
)

func (s tracePacketScope) Label() string {
	if s == traceScopePeerOnly {
		return "Peer only"
	}
	return "Peer + Port"
}

type tracePacketFilterPreset int

const (
	traceFilterPresetPeerPort tracePacketFilterPreset = iota
	traceFilterPresetPeerOnly
	traceFilterPresetFiveTuple
	traceFilterPresetSynRstOnly
	traceFilterPresetCustom
)

func (p tracePacketFilterPreset) Label() string {
	switch p {
	case traceFilterPresetPeerOnly:
		return "Peer only"
	case traceFilterPresetFiveTuple:
		return "5-tuple"
	case traceFilterPresetSynRstOnly:
		return "SYN/RST only"
	case traceFilterPresetCustom:
		return "Custom (base + clause)"
	default:
		return "Peer + Port"
	}
}

func (p tracePacketFilterPreset) Slug() string {
	switch p {
	case traceFilterPresetPeerOnly:
		return "peer-only"
	case traceFilterPresetFiveTuple:
		return "five-tuple"
	case traceFilterPresetSynRstOnly:
		return "syn-rst"
	case traceFilterPresetCustom:
		return "custom"
	default:
		return "peer-port"
	}
}

type tracePacketCaptureProfile int

const (
	traceCaptureProfileGeneral tracePacketCaptureProfile = iota
	traceCaptureProfileHandshake
	traceCaptureProfilePacketLoss
	traceCaptureProfileResetStorm
	traceCaptureProfileCustom
)

func (p tracePacketCaptureProfile) Label() string {
	switch p {
	case traceCaptureProfileHandshake:
		return "Handshake"
	case traceCaptureProfilePacketLoss:
		return "Packet loss"
	case traceCaptureProfileResetStorm:
		return "Reset storm"
	case traceCaptureProfileCustom:
		return "Custom"
	default:
		return "General triage"
	}
}

func (p tracePacketCaptureProfile) Description() string {
	switch p {
	case traceCaptureProfileHandshake:
		return "Focus handshake health (SYN/SYN-ACK/RST) with short bounded capture."
	case traceCaptureProfilePacketLoss:
		return "Collect larger sample to inspect drop ratio and retrans symptoms."
	case traceCaptureProfileResetStorm:
		return "Prioritize reset pressure and abrupt connection teardown signals."
	case traceCaptureProfileCustom:
		return "Manual tuning. Keep your current form values."
	default:
		return "Balanced first pass for operators new to packet triage."
	}
}

type tracePacketDirection int

const (
	traceDirectionAny tracePacketDirection = iota
	traceDirectionIn
	traceDirectionOut
)

func (d tracePacketDirection) Label() string {
	switch d {
	case traceDirectionIn:
		return "IN"
	case traceDirectionOut:
		return "OUT"
	default:
		return "ANY"
	}
}

func (d tracePacketDirection) TcpdumpQArg() string {
	switch d {
	case traceDirectionIn:
		return "in"
	case traceDirectionOut:
		return "out"
	default:
		return ""
	}
}

type tracePacketSeed struct {
	PeerIP     string
	Port       int
	LocalIP    string
	LocalPort  int
	RemoteIP   string
	RemotePort int
}

func (s tracePacketSeed) SupportsFiveTuple() bool {
	return strings.TrimSpace(s.LocalIP) != "" &&
		strings.TrimSpace(s.RemoteIP) != "" &&
		s.LocalPort > 0 &&
		s.RemotePort > 0
}

type tracePacketRequest struct {
	Interface       string
	PeerIP          string
	Port            int
	Profile         tracePacketCaptureProfile
	Scope           tracePacketScope
	Preset          tracePacketFilterPreset
	CustomClause    string
	TupleLocalIP    string
	TupleRemoteIP   string
	TupleLocalPort  int
	TupleRemotePort int
	Direction       tracePacketDirection
	DurationSec     int
	PacketCap       int
	SavePCAP        bool
}

type tracePacketResult struct {
	Request tracePacketRequest
	Filter  string

	StartedAt time.Time
	EndedAt   time.Time

	PCAPPath string
	Saved    bool

	Captured          int
	CapturedEstimated bool
	ReceivedByFilter  int
	DroppedByKernel   int

	DecodedPackets int
	SynCount       int
	SynAckCount    int
	RstCount       int
	SampleLines    []string

	Aborted  bool
	TimedOut bool

	CaptureErr error
	ReadErr    error
}

func (a *App) promptTracePacket() {
	if a.focusIndex != 2 {
		a.setStatusNote("Focus Top Connections before trace-packet", 5*time.Second)
		return
	}
	if a.traceCaptureRunning {
		a.setStatusNote("Trace packet is already running", 4*time.Second)
		return
	}
	seed, ok := a.selectedTracePacketSeed()
	if !ok {
		a.setStatusNote("No row selected for trace-packet", 4*time.Second)
		return
	}
	if _, err := exec.LookPath("tcpdump"); err != nil {
		a.setStatusNote("tcpdump not found on host", 6*time.Second)
		return
	}

	a.enterTraceFlowPause()

	peerInput := tview.NewInputField().
		SetLabel("Peer IP: ").
		SetFieldWidth(30).
		SetText(seed.PeerIP)
	portInput := tview.NewInputField().
		SetLabel("Port: ").
		SetFieldWidth(8)
	if seed.Port > 0 {
		portInput.SetText(strconv.Itoa(seed.Port))
	}
	portInput.SetAcceptanceFunc(tview.InputFieldInteger)

	ifaceInput := tview.NewInputField().
		SetLabel("Interface: ").
		SetFieldWidth(12).
		SetText(strings.TrimSpace(a.ifaceName))

	durationInput := tview.NewInputField().
		SetLabel("Duration (s): ").
		SetFieldWidth(4).
		SetText(strconv.Itoa(tracePacketDefaultDurationSec))
	durationInput.SetAcceptanceFunc(tview.InputFieldInteger)

	packetCapInput := tview.NewInputField().
		SetLabel("Packet cap: ").
		SetFieldWidth(7).
		SetText(strconv.Itoa(tracePacketDefaultPacketCap))
	packetCapInput.SetAcceptanceFunc(tview.InputFieldInteger)

	customClauseInput := tview.NewInputField().
		SetLabel("Custom clause: ").
		SetFieldWidth(56).
		SetText("")
	profileSelection := traceCaptureProfileGeneral
	presetSelection := defaultProfilePresetForSeed(seed)
	customPresetSelection := presetSelection
	presetOptions := []string{
		traceFilterPresetPeerPort.Label() + " (Recommended)",
		traceFilterPresetPeerOnly.Label(),
		traceFilterPresetFiveTuple.Label(),
		traceFilterPresetSynRstOnly.Label(),
	}
	modeOptions := []string{
		traceCaptureProfileGeneral.Label() + " (Recommended)",
		traceCaptureProfileHandshake.Label(),
		traceCaptureProfilePacketLoss.Label(),
		traceCaptureProfileResetStorm.Label(),
		traceCaptureProfileCustom.Label(),
	}
	directionSelection := traceDirectionAny
	if a.topDirection == topConnectionIncoming {
		directionSelection = traceDirectionIn
	} else if a.topDirection == topConnectionOutgoing {
		directionSelection = traceDirectionOut
	}
	if defaults, ok := traceCaptureProfileDefaultsFor(profileSelection, seed, a.topDirection); ok {
		presetSelection = defaults.Preset
		customPresetSelection = presetSelection
		directionSelection = defaults.Direction
		durationInput.SetText(strconv.Itoa(defaults.DurationSec))
		packetCapInput.SetText(strconv.Itoa(defaults.PacketCap))
	}
	savePCAP := true

	form := tview.NewForm()
	form.SetItemPadding(0)
	form.SetButtonsAlign(tview.AlignRight)
	guideBox := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetTextAlign(tview.AlignLeft)
	guideBox.SetBorder(true)
	guideBox.SetTitle(" Mode / Strategy Guide ")
	previewBox := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetTextAlign(tview.AlignLeft)
	previewBox.SetBorder(true)
	previewBox.SetTitle(" Tcpdump Command Preview ")
	flagGuideBox := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetTextAlign(tview.AlignLeft)
	flagGuideBox.SetBorder(true)
	flagGuideBox.SetTitle(" Tcpdump Flags Guide ")

	var presetDropDown *tview.DropDown
	var directionDropDown *tview.DropDown
	var modeDropDown *tview.DropDown
	strategyGuideText := func(p tracePacketFilterPreset) string {
		switch p {
		case traceFilterPresetPeerOnly:
			return "Capture this peer across all ports."
		case traceFilterPresetFiveTuple:
			return "Lock to one exact flow (full 5-tuple)."
		case traceFilterPresetSynRstOnly:
			return "Focus on SYN/RST handshake/reset signals."
		default:
			return "Focus on one peer + one port."
		}
	}
	modeGuideText := func(p tracePacketCaptureProfile) string {
		switch p {
		case traceCaptureProfileHandshake:
			return "Preset for handshake triage. Bias to SYN/RST signals with short bounded capture."
		case traceCaptureProfilePacketLoss:
			return "Preset for packet-loss sampling. Captures broader traffic with larger sample."
		case traceCaptureProfileResetStorm:
			return "Preset for reset pressure. Looks for high reset ratio and abrupt teardown."
		case traceCaptureProfileCustom:
			return "Manual mode. Choose filter strategy and optional extra BPF clause."
		default:
			return "Balanced first pass for quick triage."
		}
	}
	buildGuideText := func() string {
		if profileSelection != traceCaptureProfileCustom {
			return fmt.Sprintf(
				"  [yellow]Mode:[white] %s\n  [dim]%s[white]",
				profileSelection.Label(),
				modeGuideText(profileSelection),
			)
		}
		return fmt.Sprintf(
			"  [yellow]Mode:[white] %s\n  [dim]%s[white]\n  [yellow]Strategy intent:[white] %s",
			profileSelection.Label(),
			modeGuideText(profileSelection),
			strategyGuideText(customPresetSelection),
		)
	}
	buildPreviewText := func() string {
		peerIP, ok := parsePeerIPInput(peerInput.GetText())
		if !ok {
			return "  [yellow]Base cmd:[white]\n  [red]Preview unavailable: invalid peer IP.[white]"
		}
		ifaceName := strings.TrimSpace(ifaceInput.GetText())
		if ifaceName == "" {
			return "  [yellow]Base cmd:[white]\n  [red]Preview unavailable: interface is required.[white]"
		}
		_, err := parseTracePacketIntRange(durationInput.GetText(), 1, tracePacketMaxDurationSec, "Duration")
		if err != nil {
			return "  [yellow]Base cmd:[white]\n  [red]Preview unavailable: " + err.Error() + ".[white]"
		}
		packetCap, err := parseTracePacketIntRange(packetCapInput.GetText(), 1, tracePacketMaxPacketCap, "Packet cap")
		if err != nil {
			return "  [yellow]Base cmd:[white]\n  [red]Preview unavailable: " + err.Error() + ".[white]"
		}

		port := 0
		if rawPort := strings.TrimSpace(portInput.GetText()); rawPort != "" {
			port, err = parseTracePacketIntRange(rawPort, 1, 65535, "Port")
			if err != nil {
				return "  [yellow]Base cmd:[white]\n  [red]Preview unavailable: " + err.Error() + ".[white]"
			}
		}

		effectivePreset := presetSelection
		if profileSelection == traceCaptureProfileCustom {
			effectivePreset = customPresetSelection
		}

		scope := traceScopePeerPort
		switch effectivePreset {
		case traceFilterPresetPeerOnly:
			scope = traceScopePeerOnly
			port = 0
		case traceFilterPresetFiveTuple:
			if !seed.SupportsFiveTuple() {
				return "  [yellow]Base cmd:[white]\n  [red]Preview unavailable: 5-tuple needs concrete selected connection row.[white]"
			}
			if a.topDirection == topConnectionIncoming {
				if port <= 0 {
					port = seed.LocalPort
				}
			} else {
				if port <= 0 {
					port = seed.RemotePort
				}
			}
			if port <= 0 {
				return "  [yellow]Base cmd:[white]\n  [red]Preview unavailable: port is required for 5-tuple.[white]"
			}
			scope = traceScopePeerPort
		case traceFilterPresetSynRstOnly:
			if port > 0 {
				scope = traceScopePeerPort
			} else {
				scope = traceScopePeerOnly
			}
		default:
			if port <= 0 {
				return "  [yellow]Base cmd:[white]\n  [red]Preview unavailable: port is required for selected strategy.[white]"
			}
			scope = traceScopePeerPort
		}

		customClause := ""
		if profileSelection == traceCaptureProfileCustom {
			customClause = strings.TrimSpace(customClauseInput.GetText())
		}
		req := tracePacketRequest{
			Interface:       ifaceName,
			PeerIP:          peerIP,
			Port:            port,
			Profile:         profileSelection,
			Scope:           scope,
			Preset:          effectivePreset,
			CustomClause:    customClause,
			TupleLocalIP:    seed.LocalIP,
			TupleRemoteIP:   peerIP,
			TupleLocalPort:  seed.LocalPort,
			TupleRemotePort: seed.RemotePort,
			Direction:       directionSelection,
			PacketCap:       packetCap,
		}
		if req.Preset == traceFilterPresetFiveTuple {
			if a.topDirection == topConnectionIncoming {
				req.TupleLocalPort = req.Port
			} else {
				req.TupleRemotePort = req.Port
			}
		}
		qPart := ""
		if q := req.Direction.TcpdumpQArg(); q != "" {
			qPart = " -Q " + q
		}
		saveTarget := "<temp-file>"
		if savePCAP {
			peerPart := strings.NewReplacer(":", "-", ".", "_").Replace(req.PeerIP)
			portPart := "peer-only"
			if req.Port > 0 {
				portPart = strconv.Itoa(req.Port)
			}
			saveTarget = fmt.Sprintf(
				"/tmp/holyf-network-captures/trace-<timestamp>-%s-%s-%s.pcap",
				req.Preset.Slug(),
				peerPart,
				portPart,
			)
		}
		filter := buildTracePacketFilter(req)
		return fmt.Sprintf(
			"  [yellow]Base cmd:[white]\n  tcpdump -i %s -nn -tt -s 128 -c %d%s -w %s \"%s\"\n  [yellow]BPF:[white]\n  %s",
			ifaceName,
			packetCap,
			qPart,
			saveTarget,
			filter,
			filter,
		)
	}
	buildFlagGuideText := func() string {
		qHint := "-Q <omit> (ANY)"
		if directionSelection != traceDirectionAny {
			qHint = "-Q " + directionSelection.TcpdumpQArg()
		}
		packetCap := strings.TrimSpace(packetCapInput.GetText())
		if packetCap == "" {
			packetCap = strconv.Itoa(tracePacketDefaultPacketCap)
		}
		return fmt.Sprintf(
			"  [yellow]-nn[white]: disable DNS/service-name lookups for faster, cleaner output.\n  [yellow]-tt[white]: print epoch timestamps for precise packet delta analysis.\n  [yellow]-s 128[white]: capture first 128 bytes per packet (enough headers, lower overhead).\n  [yellow]-c %s[white]: stop after at most %s packets. [yellow]%s[white]: capture direction.",
			packetCap,
			packetCap,
			qHint,
		)
	}
	refreshProfileHint := func() {
		guideBox.SetText(buildGuideText())
		previewBox.SetText(buildPreviewText())
		flagGuideBox.SetText(buildFlagGuideText())
	}
	applyProfileDefaults := func(profile tracePacketCaptureProfile) {
		defaults, ok := traceCaptureProfileDefaultsFor(profile, seed, a.topDirection)
		if !ok {
			refreshProfileHint()
			return
		}
		presetSelection = defaults.Preset
		if presetSelection == traceFilterPresetCustom {
			presetSelection = defaultProfilePresetForSeed(seed)
		}
		directionSelection = defaults.Direction
		durationInput.SetText(strconv.Itoa(defaults.DurationSec))
		packetCapInput.SetText(strconv.Itoa(defaults.PacketCap))
		if presetDropDown != nil {
			presetDropDown.SetCurrentOption(traceProfileDropdownIndexForPreset(defaults.Preset))
		}
		if directionDropDown != nil {
			directionDropDown.SetCurrentOption(traceDirectionDropdownIndex(defaults.Direction))
		}
		refreshProfileHint()
	}
	findFormItemIndex := func(item tview.FormItem) int {
		for i := 0; i < form.GetFormItemCount(); i++ {
			if form.GetFormItem(i) == item {
				return i
			}
		}
		return -1
	}
	ensureFormItemPresent := func(item tview.FormItem) {
		if findFormItemIndex(item) >= 0 {
			return
		}
		form.AddFormItem(item)
	}
	removeFormItem := func(item tview.FormItem) {
		if idx := findFormItemIndex(item); idx >= 0 {
			form.RemoveFormItem(idx)
		}
	}
	updateCustomModeFields := func() {
		if profileSelection == traceCaptureProfileCustom {
			presetSelection = customPresetSelection
			if presetDropDown != nil {
				presetDropDown.SetCurrentOption(traceProfileDropdownIndexForPreset(customPresetSelection))
			}
			ensureFormItemPresent(presetDropDown)
			ensureFormItemPresent(customClauseInput)
		} else {
			removeFormItem(customClauseInput)
			removeFormItem(presetDropDown)
		}
		refreshProfileHint()
	}

	form.AddFormItem(peerInput)
	form.AddFormItem(portInput)
	form.AddFormItem(ifaceInput)
	modeDropDown = tview.NewDropDown().
		SetLabel("Mode: ").
		SetOptions(modeOptions, func(_ string, index int) {
			switch index {
			case 1:
				profileSelection = traceCaptureProfileHandshake
			case 2:
				profileSelection = traceCaptureProfilePacketLoss
			case 3:
				profileSelection = traceCaptureProfileResetStorm
			case 4:
				profileSelection = traceCaptureProfileCustom
			default:
				profileSelection = traceCaptureProfileGeneral
			}
			if profileSelection == traceCaptureProfileCustom {
				updateCustomModeFields()
				return
			}
			applyProfileDefaults(profileSelection)
			updateCustomModeFields()
		})
	modeDropDown.SetCurrentOption(0)
	form.AddFormItem(modeDropDown)
	presetDropDown = tview.NewDropDown().
		SetLabel("Filter strategy: ").
		SetOptions(presetOptions, func(_ string, index int) {
			switch index {
			case 1:
				presetSelection = traceFilterPresetPeerOnly
			case 2:
				presetSelection = traceFilterPresetFiveTuple
			case 3:
				presetSelection = traceFilterPresetSynRstOnly
			default:
				presetSelection = traceFilterPresetPeerPort
			}
			customPresetSelection = presetSelection
			refreshProfileHint()
		})
	presetDropDown.SetCurrentOption(traceProfileDropdownIndexForPreset(presetSelection))
	customClauseInput.SetLabel("Extra BPF clause: ")
	directionDropDown = tview.NewDropDown().
		SetLabel("Direction: ").
		SetOptions([]string{"ANY", "IN", "OUT"}, func(_ string, index int) {
			switch index {
			case 1:
				directionSelection = traceDirectionIn
			case 2:
				directionSelection = traceDirectionOut
			default:
				directionSelection = traceDirectionAny
			}
			refreshProfileHint()
		})
	directionDropDown.SetCurrentOption(traceDirectionDropdownIndex(directionSelection))
	form.AddFormItem(directionDropDown)
	form.AddFormItem(durationInput)
	form.AddFormItem(packetCapInput)
	form.AddCheckbox("Save pcap: ", savePCAP, func(checked bool) {
		savePCAP = checked
		refreshProfileHint()
	})
	updateCustomModeFields()

	peerInput.SetChangedFunc(func(text string) { refreshProfileHint() })
	portInput.SetChangedFunc(func(text string) { refreshProfileHint() })
	ifaceInput.SetChangedFunc(func(text string) { refreshProfileHint() })
	durationInput.SetChangedFunc(func(text string) { refreshProfileHint() })
	packetCapInput.SetChangedFunc(func(text string) { refreshProfileHint() })
	customClauseInput.SetChangedFunc(func(text string) { refreshProfileHint() })
	refreshProfileHint()

	closeForm := func() {
		a.pages.RemovePage(tracePacketPageForm)
		a.app.SetFocus(a.panels[a.focusIndex])
		a.exitTraceFlowPause()
		a.updateStatusBar()
	}

	submit := func() {
		peerIP, ok := parsePeerIPInput(peerInput.GetText())
		if !ok {
			a.setStatusNote("Invalid peer IP", 5*time.Second)
			return
		}
		ifaceName := strings.TrimSpace(ifaceInput.GetText())
		if ifaceName == "" {
			a.setStatusNote("Interface is required", 5*time.Second)
			return
		}
		durationSec, err := parseTracePacketIntRange(
			durationInput.GetText(),
			1,
			tracePacketMaxDurationSec,
			"Duration",
		)
		if err != nil {
			a.setStatusNote(err.Error(), 5*time.Second)
			return
		}
		packetCap, err := parseTracePacketIntRange(
			packetCapInput.GetText(),
			1,
			tracePacketMaxPacketCap,
			"Packet cap",
		)
		if err != nil {
			a.setStatusNote(err.Error(), 5*time.Second)
			return
		}

		scope := traceScopePeerPort
		port := 0
		rawPort := strings.TrimSpace(portInput.GetText())
		if rawPort != "" {
			port, err = parseTracePacketIntRange(rawPort, 1, 65535, "Port")
			if err != nil {
				a.setStatusNote(err.Error(), 5*time.Second)
				return
			}
		}
		effectivePreset := presetSelection
		if profileSelection == traceCaptureProfileCustom {
			effectivePreset = customPresetSelection
		}

		switch effectivePreset {
		case traceFilterPresetPeerOnly:
			scope = traceScopePeerOnly
			port = 0
		case traceFilterPresetPeerPort:
			scope = traceScopePeerPort
			if port <= 0 {
				a.setStatusNote("Port is required for selected strategy", 5*time.Second)
				return
			}
		case traceFilterPresetSynRstOnly:
			if port > 0 {
				scope = traceScopePeerPort
			} else {
				scope = traceScopePeerOnly
			}
		case traceFilterPresetFiveTuple:
			scope = traceScopePeerPort
			if !seed.SupportsFiveTuple() {
				a.setStatusNote("5-tuple strategy requires a concrete connection row (not aggregate-only)", 6*time.Second)
				return
			}
			if a.topDirection == topConnectionIncoming {
				if port <= 0 {
					port = seed.LocalPort
				}
				if port <= 0 {
					a.setStatusNote("Port is required for 5-tuple strategy", 5*time.Second)
					return
				}
			} else {
				if port <= 0 {
					port = seed.RemotePort
				}
				if port <= 0 {
					a.setStatusNote("Port is required for 5-tuple strategy", 5*time.Second)
					return
				}
			}
		default:
			scope = traceScopePeerPort
			if port <= 0 {
				a.setStatusNote("Port is required for selected strategy", 5*time.Second)
				return
			}
		}

		customClause := ""
		if profileSelection == traceCaptureProfileCustom {
			customClause = strings.TrimSpace(customClauseInput.GetText())
		}

		req := tracePacketRequest{
			Interface:       ifaceName,
			PeerIP:          peerIP,
			Port:            port,
			Profile:         profileSelection,
			Scope:           scope,
			Preset:          effectivePreset,
			CustomClause:    customClause,
			TupleLocalIP:    seed.LocalIP,
			TupleRemoteIP:   peerIP,
			TupleLocalPort:  seed.LocalPort,
			TupleRemotePort: seed.RemotePort,
			Direction:       directionSelection,
			DurationSec:     durationSec,
			PacketCap:       packetCap,
			SavePCAP:        savePCAP,
		}
		if req.Preset == traceFilterPresetFiveTuple {
			if a.topDirection == topConnectionIncoming {
				req.TupleLocalPort = req.Port
			} else {
				req.TupleRemotePort = req.Port
			}
		}
		a.pages.RemovePage(tracePacketPageForm)
		a.updateStatusBar()
		a.startTracePacketCapture(req)
	}

	form.AddButton("Start", submit)
	form.AddButton("Cancel", closeForm)
	form.SetCancelFunc(closeForm)

	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEsc {
			if shouldCloseTracePacketFormOnEsc(modeDropDown, presetDropDown, directionDropDown) {
				closeForm()
				return nil
			}
			return event
		}
		if event.Key() == tcell.KeyEnter {
			if shouldSubmitTracePacketOnEnter(form, modeDropDown, presetDropDown, directionDropDown) {
				submit()
				return nil
			}
			return event
		}
		return event
	})

	form.SetBorder(true)
	form.SetTitle(" Trace Packet ")

	helpLine := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false).
		SetTextAlign(tview.AlignLeft)
	helpLine.SetText(" [dim]Use selected top row as seed. Enter=start (except dropdown/checkbox/button focus). Esc=close.[white]")

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
				AddItem(helpLine, 1, 0, false).
				AddItem(form, 0, 1, true).
				AddItem(guideBox, 6, 0, false).
				AddItem(previewBox, 8, 0, false).
				AddItem(flagGuideBox, 6, 0, false),
				92, 0, true).
			AddItem(nil, 0, 1, false),
			38, 0, true).
		AddItem(nil, 0, 1, false)

	a.pages.RemovePage(tracePacketPageForm)
	a.pages.AddPage(tracePacketPageForm, modal, true, true)
	a.updateStatusBar()
	form.SetFocus(0)
	a.app.SetFocus(form)
}

func shouldSubmitTracePacketOnEnter(
	form *tview.Form,
	modeDropDown *tview.DropDown,
	presetDropDown *tview.DropDown,
	directionDropDown *tview.DropDown,
) bool {
	if form == nil {
		return false
	}
	if modeDropDown != nil && modeDropDown.IsOpen() {
		return false
	}
	if presetDropDown != nil && presetDropDown.IsOpen() {
		return false
	}
	if directionDropDown != nil && directionDropDown.IsOpen() {
		return false
	}

	formItemIndex, buttonIndex := form.GetFocusedItemIndex()
	if buttonIndex >= 0 {
		return false
	}
	if formItemIndex < 0 || formItemIndex >= form.GetFormItemCount() {
		return true
	}
	switch form.GetFormItem(formItemIndex).(type) {
	case *tview.DropDown, *tview.Checkbox:
		return false
	default:
		return true
	}
}

func shouldCloseTracePacketFormOnEsc(
	modeDropDown *tview.DropDown,
	presetDropDown *tview.DropDown,
	directionDropDown *tview.DropDown,
) bool {
	profileOpen := modeDropDown != nil && modeDropDown.IsOpen()
	presetOpen := presetDropDown != nil && presetDropDown.IsOpen()
	directionOpen := directionDropDown != nil && directionDropDown.IsOpen()
	return shouldCloseTracePacketFormOnEscByState(profileOpen, presetOpen, directionOpen)
}

func shouldCloseTracePacketFormOnEscByState(profileOpen, presetOpen, directionOpen bool) bool {
	return !(profileOpen || presetOpen || directionOpen)
}

func (a *App) selectedTracePacketSeed() (tracePacketSeed, bool) {
	if a.groupView {
		groups := a.visiblePeerGroups()
		if len(groups) == 0 {
			return tracePacketSeed{}, false
		}
		a.clampTopConnectionSelection()
		group := groups[a.selectedTalkerIndex]
		seed := tracePacketSeed{
			PeerIP:   normalizeIP(group.PeerIP),
			RemoteIP: normalizeIP(group.PeerIP),
		}
		if a.topDirection == topConnectionIncoming {
			if target, ok := a.selectedPeerPortTarget(seed.PeerIP); ok {
				seed.Port = target.LocalPort
			} else {
				ports := sortedPeerGroupPorts(group.LocalPorts)
				if len(ports) > 0 {
					seed.Port = ports[0]
				}
			}
		} else {
			ports := sortedPeerGroupPorts(group.RemotePorts)
			if len(ports) > 0 {
				seed.Port = ports[0]
			}
		}
		if conn, ok := a.traceSeedRepresentative(seed.PeerIP, seed.Port); ok {
			seed.LocalIP = normalizeIP(conn.LocalIP)
			seed.LocalPort = conn.LocalPort
			seed.RemoteIP = normalizeIP(conn.RemoteIP)
			seed.RemotePort = conn.RemotePort
		}
		return seed, true
	}

	rows := a.visibleTopConnections()
	if len(rows) == 0 {
		return tracePacketSeed{}, false
	}
	a.clampTopConnectionSelection()
	row := rows[a.selectedTalkerIndex]
	seed := tracePacketSeed{
		PeerIP:     normalizeIP(row.RemoteIP),
		LocalIP:    normalizeIP(row.LocalIP),
		LocalPort:  row.LocalPort,
		RemoteIP:   normalizeIP(row.RemoteIP),
		RemotePort: row.RemotePort,
	}
	if a.topDirection == topConnectionOutgoing {
		seed.Port = row.RemotePort
	} else {
		seed.Port = row.LocalPort
	}
	return seed, true
}

func (a *App) traceSeedRepresentative(peerIP string, port int) (collector.Connection, bool) {
	source := a.topConnectionsSource()
	if len(source) == 0 {
		return collector.Connection{}, false
	}

	filtered := a.applyTopConnectionFilters(source)
	if a.groupView {
		filtered = applyGroupConnectionFiltersByDirection(source, a.portFilter, a.textFilter, a.topDirection)
	}
	if len(filtered) == 0 {
		return collector.Connection{}, false
	}

	best := collector.Connection{}
	found := false
	for _, conn := range filtered {
		if normalizeIP(conn.RemoteIP) != peerIP {
			continue
		}
		if port > 0 {
			if a.topDirection == topConnectionOutgoing && conn.RemotePort != port {
				continue
			}
			if a.topDirection != topConnectionOutgoing && conn.LocalPort != port {
				continue
			}
		}

		if !found ||
			conn.Activity > best.Activity ||
			(conn.Activity == best.Activity && conn.TotalBytesDelta > best.TotalBytesDelta) {
			best = conn
			found = true
		}
	}
	return best, found
}

type traceCaptureProfileDefaults struct {
	Preset      tracePacketFilterPreset
	Direction   tracePacketDirection
	DurationSec int
	PacketCap   int
}

func defaultProfilePresetForSeed(seed tracePacketSeed) tracePacketFilterPreset {
	if seed.Port <= 0 {
		return traceFilterPresetPeerOnly
	}
	return traceFilterPresetPeerPort
}

func traceCaptureProfileDefaultsFor(
	profile tracePacketCaptureProfile,
	seed tracePacketSeed,
	topDir topConnectionDirection,
) (traceCaptureProfileDefaults, bool) {
	basePreset := defaultProfilePresetForSeed(seed)
	baseDir := traceDirectionAny
	if topDir == topConnectionIncoming {
		baseDir = traceDirectionIn
	} else if topDir == topConnectionOutgoing {
		baseDir = traceDirectionOut
	}

	switch profile {
	case traceCaptureProfileHandshake:
		return traceCaptureProfileDefaults{
			Preset:      traceFilterPresetSynRstOnly,
			Direction:   baseDir,
			DurationSec: 8,
			PacketCap:   1200,
		}, true
	case traceCaptureProfilePacketLoss:
		return traceCaptureProfileDefaults{
			Preset:      basePreset,
			Direction:   traceDirectionAny,
			DurationSec: 20,
			PacketCap:   6000,
		}, true
	case traceCaptureProfileResetStorm:
		return traceCaptureProfileDefaults{
			Preset:      traceFilterPresetSynRstOnly,
			Direction:   traceDirectionAny,
			DurationSec: 15,
			PacketCap:   5000,
		}, true
	case traceCaptureProfileCustom:
		return traceCaptureProfileDefaults{}, false
	default:
		return traceCaptureProfileDefaults{
			Preset:      basePreset,
			Direction:   baseDir,
			DurationSec: tracePacketDefaultDurationSec,
			PacketCap:   tracePacketDefaultPacketCap,
		}, true
	}
}

func traceProfileDropdownIndexForPreset(p tracePacketFilterPreset) int {
	switch p {
	case traceFilterPresetPeerOnly:
		return 1
	case traceFilterPresetFiveTuple:
		return 2
	case traceFilterPresetSynRstOnly:
		return 3
	default:
		return 0
	}
}

func traceDirectionDropdownIndex(d tracePacketDirection) int {
	switch d {
	case traceDirectionIn:
		return 1
	case traceDirectionOut:
		return 2
	default:
		return 0
	}
}

func parseTracePacketIntRange(raw string, minVal, maxVal int, field string) (int, error) {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v < minVal || v > maxVal {
		return 0, fmt.Errorf("%s must be %d-%d", field, minVal, maxVal)
	}
	return v, nil
}

func (a *App) startTracePacketCapture(req tracePacketRequest) {
	if a.traceCaptureRunning {
		a.setStatusNote("Trace packet is already running", 4*time.Second)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(req.DurationSec)*time.Second)
	a.traceCaptureRunning = true
	a.traceCaptureCancel = cancel

	progressView := a.showTracePacketProgressModal(req)
	startedAt := time.Now()

	updateProgress := func() {
		if progressView == nil {
			return
		}
		remaining := req.DurationSec - int(time.Since(startedAt).Seconds())
		if remaining < 0 {
			remaining = 0
		}
		progressView.SetText(fmt.Sprintf(
			"  [yellow]Trace Packet Running[white]\n\n  Mode: [green]%s[white]\n  Interface: [green]%s[white]\n  Scope: [green]%s[white]\n  Direction: [green]%s[white]\n  Duration: [green]%ds[white] | Cap: [green]%d[white]\n  Remaining: [green]%ds[white]\n\n  [dim]Press Esc to abort.[white]",
			req.Profile.Label(),
			req.Interface,
			tracePacketScopeDisplay(req),
			req.Direction.Label(),
			req.DurationSec,
			req.PacketCap,
			remaining,
		))
	}

	updateProgress()

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				a.app.QueueUpdateDraw(func() {
					if !a.traceCaptureRunning {
						return
					}
					updateProgress()
				})
			case <-ctx.Done():
				return
			case <-a.stopChan:
				return
			}
		}
	}()

	go func() {
		result := runTracePacketCapture(ctx, req)
		a.app.QueueUpdateDraw(func() {
			a.traceCaptureRunning = false
			if a.traceCaptureCancel != nil {
				a.traceCaptureCancel()
			}
			a.traceCaptureCancel = nil
			a.pages.RemovePage(tracePacketPageProgress)
			a.exitTraceFlowPause()
			a.updateStatusBar()

			switch {
			case result.Aborted:
				a.setStatusNote("Trace packet aborted", 5*time.Second)
			case result.CaptureErr != nil:
				a.setStatusNote("Trace packet failed: "+shortStatus(maskSensitiveIPsInText(result.CaptureErr.Error(), a.sensitiveIP), 72), 8*time.Second)
			default:
				droppedText := "n/a"
				if result.DroppedByKernel >= 0 {
					droppedText = strconv.Itoa(result.DroppedByKernel)
				}
				a.setStatusNote(
					fmt.Sprintf("Trace packet done: captured=%d dropped=%s", result.Captured, droppedText),
					8*time.Second,
				)
			}

			a.appendTraceHistory(result)
			a.addActionLog(buildTracePacketActionSummary(result, a.sensitiveIP))
			a.showTracePacketResultModal(result)
		})
	}()
}

func (a *App) showTracePacketProgressModal(req tracePacketRequest) *tview.TextView {
	view := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetTextAlign(tview.AlignLeft)
	view.SetBorder(true)
	view.SetTitle(" Trace Packet Progress ")
	view.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEsc || (event.Key() == tcell.KeyRune && event.Rune() == 'q') {
			a.cancelTracePacketCapture()
			a.setStatusNote("Trace packet: abort requested", 4*time.Second)
			return nil
		}
		return event
	})

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(view, 88, 0, true).
			AddItem(nil, 0, 1, false),
			13, 0, true).
		AddItem(nil, 0, 1, false)

	a.pages.RemovePage(tracePacketPageProgress)
	a.pages.AddPage(tracePacketPageProgress, modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(view)
	return view
}

func (a *App) showTracePacketResultModal(result tracePacketResult) {
	body := buildTracePacketResultText(result, a.sensitiveIP)

	view := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetTextAlign(tview.AlignLeft).
		SetText(body)
	view.SetBorder(true)
	view.SetTitle(" Trace Packet Result ")

	closeModal := func() {
		a.pages.RemovePage(tracePacketPageResult)
		a.app.SetFocus(a.panels[a.focusIndex])
		a.updateStatusBar()
	}
	view.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc, tcell.KeyEnter:
			closeModal()
			return nil
		}
		return event
	})

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(view, 120, 0, true).
			AddItem(nil, 0, 1, false),
			26, 0, true).
		AddItem(nil, 0, 1, false)

	a.pages.RemovePage(tracePacketPageResult)
	a.pages.AddPage(tracePacketPageResult, modal, true, true)
	a.updateStatusBar()
	a.app.SetFocus(view)
}

func (a *App) cancelTracePacketCapture() {
	if a.traceCaptureCancel == nil {
		return
	}
	a.traceCaptureCancel()
}

func (a *App) enterTraceFlowPause() {
	if a.paused.Load() {
		return
	}
	a.paused.Store(true)
	a.traceFlowAutoPaused = true
	a.updateStatusBar()
}

func (a *App) exitTraceFlowPause() {
	if !a.traceFlowAutoPaused {
		return
	}
	a.traceFlowAutoPaused = false
	a.paused.Store(false)
	a.updateStatusBar()
}

func buildTracePacketFilter(req tracePacketRequest) string {
	base := tracePacketBaseFilter(req)
	filter := base
	switch req.Preset {
	case traceFilterPresetPeerOnly:
		filter = "tcp and host " + req.PeerIP
	case traceFilterPresetFiveTuple:
		if strings.TrimSpace(req.TupleLocalIP) == "" ||
			strings.TrimSpace(req.TupleRemoteIP) == "" ||
			req.TupleLocalPort <= 0 ||
			req.TupleRemotePort <= 0 {
			filter = base
			break
		}
		filter = fmt.Sprintf(
			"tcp and ((src host %s and src port %d and dst host %s and dst port %d) or (src host %s and src port %d and dst host %s and dst port %d))",
			req.TupleLocalIP,
			req.TupleLocalPort,
			req.TupleRemoteIP,
			req.TupleRemotePort,
			req.TupleRemoteIP,
			req.TupleRemotePort,
			req.TupleLocalIP,
			req.TupleLocalPort,
		)
	case traceFilterPresetSynRstOnly:
		filter = fmt.Sprintf("%s and (tcp[tcpflags] & (tcp-syn|tcp-rst) != 0)", base)
	case traceFilterPresetCustom:
		filter = base
	default:
		filter = base
	}
	clause := strings.TrimSpace(req.CustomClause)
	if clause != "" {
		filter = fmt.Sprintf("%s and (%s)", filter, clause)
	}
	return filter
}

func tracePacketBaseFilter(req tracePacketRequest) string {
	base := "tcp and host " + req.PeerIP
	if req.Scope == traceScopePeerOnly || req.Port <= 0 {
		return base
	}
	return fmt.Sprintf("%s and port %d", base, req.Port)
}

func tracePacketScopeDisplay(req tracePacketRequest) string {
	switch req.Preset {
	case traceFilterPresetFiveTuple:
		return "5-tuple"
	case traceFilterPresetSynRstOnly:
		return "SYN/RST only"
	case traceFilterPresetCustom:
		if req.Scope == traceScopePeerOnly {
			return "Custom (Peer)"
		}
		return "Custom (Peer+Port)"
	default:
		return req.Scope.Label()
	}
}

func runTracePacketCapture(ctx context.Context, req tracePacketRequest) tracePacketResult {
	result := tracePacketResult{
		Request:          req,
		Filter:           buildTracePacketFilter(req),
		StartedAt:        time.Now(),
		Captured:         -1,
		ReceivedByFilter: -1,
		DroppedByKernel:  -1,
	}

	if _, err := exec.LookPath("tcpdump"); err != nil {
		result.CaptureErr = fmt.Errorf("tcpdump not found: %w", err)
		result.EndedAt = time.Now()
		return result
	}

	pcapPath, keepFile, err := prepareTracePacketPath(req)
	if err != nil {
		result.CaptureErr = err
		result.EndedAt = time.Now()
		return result
	}
	result.PCAPPath = pcapPath
	result.Saved = keepFile

	args := []string{
		"-i", req.Interface,
		"-nn",
		"-tt",
		"-s", "128",
		"-c", strconv.Itoa(req.PacketCap),
		"-w", pcapPath,
	}
	if qArg := req.Direction.TcpdumpQArg(); qArg != "" {
		args = append(args, "-Q", qArg)
	}
	args = append(args, result.Filter)

	cmd := exec.Command("tcpdump", args...)
	cmd.Stdout = io.Discard
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	runErr := runTracePacketCommandWithContext(ctx, cmd)
	captured, receivedByFilter, droppedByKernel := parseTracePacketCounters(stderr.String())
	result.Captured = captured
	result.ReceivedByFilter = receivedByFilter
	result.DroppedByKernel = droppedByKernel

	switch {
	case errors.Is(ctx.Err(), context.Canceled):
		result.Aborted = true
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		result.TimedOut = true
	case runErr != nil:
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = runErr.Error()
		}
		result.CaptureErr = fmt.Errorf("%s", msg)
	}

	if stat, err := os.Stat(pcapPath); err == nil && stat.Size() > 0 {
		sampleLines, decoded, syn, synAck, rst, readErr := inspectTracePacketPCAP(pcapPath)
		result.SampleLines = sampleLines
		result.DecodedPackets = decoded
		result.SynCount = syn
		result.SynAckCount = synAck
		result.RstCount = rst
		result.ReadErr = readErr
	}
	if result.Captured < 0 && result.DecodedPackets > 0 {
		result.Captured = result.DecodedPackets
		result.CapturedEstimated = true
	}

	if !keepFile {
		_ = os.Remove(pcapPath)
		result.PCAPPath = ""
	}

	result.EndedAt = time.Now()
	return result
}

func runTracePacketCommandWithContext(ctx context.Context, cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	select {
	case err := <-waitCh:
		return err
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = cmd.Process.Signal(os.Interrupt)
		}
		timer := time.NewTimer(tracePacketStopGrace)
		defer timer.Stop()
		select {
		case err := <-waitCh:
			return err
		case <-timer.C:
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			return <-waitCh
		}
	}
}

func prepareTracePacketPath(req tracePacketRequest) (path string, keep bool, err error) {
	if req.SavePCAP {
		if err := os.MkdirAll(tracePacketSavedDir, 0o755); err != nil {
			return "", false, fmt.Errorf("cannot create capture dir: %w", err)
		}
		presetPart := req.Preset.Slug()
		peerPart := strings.NewReplacer(":", "-", ".", "_").Replace(req.PeerIP)
		portPart := "peer-only"
		if req.Port > 0 {
			portPart = strconv.Itoa(req.Port)
		}
		name := fmt.Sprintf(
			"trace-%s-%s-%s-%s.pcap",
			time.Now().Format("20060102-150405"),
			presetPart,
			peerPart,
			portPart,
		)
		return filepath.Join(tracePacketSavedDir, name), true, nil
	}

	f, err := os.CreateTemp("", "holyf-network-trace-*.pcap")
	if err != nil {
		return "", false, fmt.Errorf("cannot create temp capture file: %w", err)
	}
	defer f.Close()
	return f.Name(), false, nil
}

var (
	traceCapturedRe = regexp.MustCompile(`^\s*(\d+)\s+packets captured`)
	traceRecvRe     = regexp.MustCompile(`^\s*(\d+)\s+packets received by filter`)
	traceDropRe     = regexp.MustCompile(`^\s*(\d+)\s+packets dropped by kernel`)
)

func parseTracePacketCounters(raw string) (captured int, receivedByFilter int, droppedByKernel int) {
	captured = -1
	receivedByFilter = -1
	droppedByKernel = -1

	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if m := traceCapturedRe.FindStringSubmatch(line); len(m) == 2 {
			if v, err := strconv.Atoi(m[1]); err == nil {
				captured = v
			}
			continue
		}
		if m := traceRecvRe.FindStringSubmatch(line); len(m) == 2 {
			if v, err := strconv.Atoi(m[1]); err == nil {
				receivedByFilter = v
			}
			continue
		}
		if m := traceDropRe.FindStringSubmatch(line); len(m) == 2 {
			if v, err := strconv.Atoi(m[1]); err == nil {
				droppedByKernel = v
			}
		}
	}
	return captured, receivedByFilter, droppedByKernel
}

func inspectTracePacketPCAP(path string) (sampleLines []string, decodedPackets int, syn int, synAck int, rst int, err error) {
	cmd := exec.Command("tcpdump", "-nn", "-tt", "-r", path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, 0, 0, 0, 0, fmt.Errorf("pcap read setup failed: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, 0, 0, 0, 0, fmt.Errorf("pcap read start failed: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	buf := make([]byte, 0, 1024*64)
	scanner.Buffer(buf, 1024*1024)

	samples := make([]string, 0, tracePacketSampleLineLimit)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		decodedPackets++
		if len(samples) < tracePacketSampleLineLimit {
			samples = append(samples, line)
		}

		if strings.Contains(line, "Flags [S.]") {
			synAck++
		} else if strings.Contains(line, "Flags [S]") {
			syn++
		}
		if strings.Contains(line, "Flags [R") {
			rst++
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		_ = cmd.Wait()
		return samples, decodedPackets, syn, synAck, rst, fmt.Errorf("pcap read scan failed: %w", scanErr)
	}
	if waitErr := cmd.Wait(); waitErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = waitErr.Error()
		}
		return samples, decodedPackets, syn, synAck, rst, fmt.Errorf("pcap read failed: %s", msg)
	}

	return samples, decodedPackets, syn, synAck, rst, nil
}

func buildTracePacketResultText(result tracePacketResult, sensitiveIP bool) string {
	var b strings.Builder
	b.WriteString("  [yellow]Trace Packet Summary[white]\n")
	b.WriteString("  ─────────────────────────────────────────\n")
	b.WriteString(fmt.Sprintf("  Mode: [green]%s[white]\n", result.Request.Profile.Label()))
	b.WriteString(fmt.Sprintf("  Interface: [green]%s[white]\n", result.Request.Interface))
	b.WriteString(fmt.Sprintf("  Filter: [green]%s[white]\n", buildTracePacketDisplayFilter(result.Request, sensitiveIP)))
	b.WriteString(fmt.Sprintf("  Scope: [green]%s[white] | Direction: [green]%s[white]\n", tracePacketScopeDisplay(result.Request), result.Request.Direction.Label()))
	b.WriteString(fmt.Sprintf("  Duration: [green]%ds[white] | Packet cap: [green]%d[white]\n", result.Request.DurationSec, result.Request.PacketCap))

	status := "[green]completed[white]"
	if result.Aborted {
		status = "[yellow]aborted[white]"
	} else if result.CaptureErr != nil {
		status = "[red]failed[white]"
	} else if result.TimedOut {
		status = "[green]completed (timeout boundary)[white]"
	}
	b.WriteString(fmt.Sprintf("  Status: %s\n", status))

	b.WriteString(fmt.Sprintf(
		"  Captured: [green]%s[white] | ReceivedByFilter: [green]%s[white] | DroppedByKernel: [green]%s[white]\n",
		tracePacketMetricDisplay(result.Captured, result.CapturedEstimated),
		tracePacketMetricValue(result.ReceivedByFilter),
		tracePacketMetricValue(result.DroppedByKernel),
	))
	b.WriteString(fmt.Sprintf(
		"  Decoded: [green]%d[white] | SYN: [green]%d[white] | SYN-ACK: [green]%d[white] | RST: [green]%d[white]\n",
		result.DecodedPackets,
		result.SynCount,
		result.SynAckCount,
		result.RstCount,
	))
	diag := analyzeTracePacket(result)
	b.WriteString("\n  [yellow]Trace Analyzer[white]\n")
	b.WriteString("  ─────────────────────────────────────────\n")
	b.WriteString(fmt.Sprintf(
		"  Severity: %s | Confidence: %s\n",
		tracePacketSeverityStyled(diag.Severity),
		tracePacketConfidenceStyled(diag.Confidence),
	))
	b.WriteString(fmt.Sprintf("  Issue: %s\n", maskSensitiveIPsInText(diag.Issue, sensitiveIP)))
	b.WriteString(fmt.Sprintf("  Signal: %s\n", maskSensitiveIPsInText(diag.Signal, sensitiveIP)))
	b.WriteString(fmt.Sprintf("  Likely: %s\n", maskSensitiveIPsInText(diag.Likely, sensitiveIP)))
	b.WriteString(fmt.Sprintf("  Check next: %s\n", maskSensitiveIPsInText(diag.Check, sensitiveIP)))

	if result.Saved && strings.TrimSpace(result.PCAPPath) != "" {
		b.WriteString(fmt.Sprintf("  PCAP: [aqua]%s[white]\n", maskTracePacketPath(result.PCAPPath, sensitiveIP)))
	} else {
		b.WriteString("  PCAP: [dim]not saved (summary-only mode)[white]\n")
	}

	if result.CaptureErr != nil {
		b.WriteString(fmt.Sprintf("  [red]Capture error:[white] %s\n", shortStatus(maskSensitiveIPsInText(result.CaptureErr.Error(), sensitiveIP), 180)))
	}
	if result.ReadErr != nil {
		if shouldDowngradeTracePacketReadWarning(result) {
			b.WriteString(fmt.Sprintf("  [dim]Read note:[white] partial capture near timeout boundary (%s)\n", shortStatus(maskSensitiveIPsInText(result.ReadErr.Error(), sensitiveIP), 120)))
		} else {
			b.WriteString(fmt.Sprintf("  [yellow]Read warning:[white] %s\n", shortStatus(maskSensitiveIPsInText(result.ReadErr.Error(), sensitiveIP), 180)))
		}
	}

	b.WriteString("\n  [yellow]Sample Packets[white]\n")
	b.WriteString("  ─────────────────────────────────────────\n")
	if len(result.SampleLines) == 0 {
		b.WriteString("  [dim]No decoded packet lines.[white]\n")
	} else {
		for _, line := range result.SampleLines {
			b.WriteString("  ")
			b.WriteString(maskSensitiveIPsInText(line, sensitiveIP))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n  [dim]Enter/Esc to close.[white]")
	return b.String()
}

func tracePacketMetricValue(v int) string {
	if v < 0 {
		return "n/a"
	}
	return strconv.Itoa(v)
}

func tracePacketMetricDisplay(v int, estimated bool) string {
	base := tracePacketMetricValue(v)
	if estimated && v >= 0 {
		return base + " (est.)"
	}
	return base
}

func shouldDowngradeTracePacketReadWarning(result tracePacketResult) bool {
	if !result.TimedOut || result.ReadErr == nil || result.DecodedPackets <= 0 {
		return false
	}
	msg := strings.ToLower(result.ReadErr.Error())
	if strings.Contains(msg, "pcap_loop") || strings.Contains(msg, "truncated") || strings.Contains(msg, "reading from file") {
		return true
	}
	return false
}

func buildTracePacketActionSummary(result tracePacketResult, sensitiveIP bool) string {
	status := "ok"
	if result.Aborted {
		status = "aborted"
	}
	if result.CaptureErr != nil {
		status = "failed"
	}
	portPart := "peer-only"
	if result.Request.Port > 0 {
		portPart = strconv.Itoa(result.Request.Port)
	}
	dropped := tracePacketMetricValue(result.DroppedByKernel)
	saved := "no"
	if result.Saved && strings.TrimSpace(result.PCAPPath) != "" {
		saved = maskTracePacketPath(result.PCAPPath, sensitiveIP)
	}

	return fmt.Sprintf(
		"Trace %s %s:%s | mode=%s dir=%s scope=%s | captured=%s drop=%s rst=%d | saved=%s",
		status,
		formatPreviewIP(result.Request.PeerIP, sensitiveIP),
		portPart,
		result.Request.Profile.Label(),
		result.Request.Direction.Label(),
		tracePacketScopeDisplay(result.Request),
		tracePacketMetricValue(result.Captured),
		dropped,
		result.RstCount,
		saved,
	)
}

func buildTracePacketDisplayFilter(req tracePacketRequest, sensitiveIP bool) string {
	return maskSensitiveIPsInText(buildTracePacketFilter(req), sensitiveIP)
}

var (
	traceIPv4Regex = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	traceIPv6Regex = regexp.MustCompile(`\b(?:[0-9a-fA-F]{1,4}:){2,}[0-9a-fA-F:.]*[0-9a-fA-F]{1,4}\b`)
)

func maskSensitiveIPsInText(raw string, sensitiveIP bool) string {
	if !sensitiveIP || strings.TrimSpace(raw) == "" {
		return raw
	}

	out := traceIPv4Regex.ReplaceAllStringFunc(raw, func(token string) string {
		ip := net.ParseIP(token)
		if ip == nil || ip.To4() == nil {
			return token
		}
		return maskIP(token)
	})
	out = traceIPv6Regex.ReplaceAllStringFunc(out, func(token string) string {
		ip := net.ParseIP(token)
		if ip == nil || ip.To4() != nil {
			return token
		}
		return maskIP(token)
	})
	return out
}

func maskTracePacketPath(path string, sensitiveIP bool) string {
	if !sensitiveIP {
		return path
	}
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return path
	}
	dir := filepath.Dir(trimmed)
	base := filepath.Base(trimmed)
	// Keep timestamp in display path to avoid "overwrite" confusion while masking peer/port.
	// Expected saved-file pattern: trace-YYYYMMDD-HHMMSS-<peer>-<port>.pcap
	if strings.HasPrefix(base, "trace-") && strings.HasSuffix(base, ".pcap") {
		name := strings.TrimSuffix(strings.TrimPrefix(base, "trace-"), ".pcap")
		parts := strings.Split(name, "-")
		if len(parts) >= 2 {
			return filepath.Join(dir, fmt.Sprintf("trace-%s-%s-masked.pcap", parts[0], parts[1]))
		}
	}
	// Avoid tview color-tag syntax (`[...]`) in displayed path.
	return filepath.Join(dir, "trace-masked.pcap")
}
