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

	tracePacketSavedDir  = "/tmp/holyf-network/captures"
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
	PeerIP string
	Port   int
}

type tracePacketRequest struct {
	Interface   string
	PeerIP      string
	Port        int
	Scope       tracePacketScope
	Direction   tracePacketDirection
	DurationSec int
	PacketCap   int
	SavePCAP    bool
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

	scopeSelection := traceScopePeerPort
	scopeDefaultIndex := 0
	if seed.Port <= 0 {
		scopeSelection = traceScopePeerOnly
		scopeDefaultIndex = 1
	}
	directionSelection := traceDirectionAny
	if a.topDirection == topConnectionIncoming {
		directionSelection = traceDirectionIn
	} else if a.topDirection == topConnectionOutgoing {
		directionSelection = traceDirectionOut
	}
	savePCAP := true

	form := tview.NewForm()
	form.SetItemPadding(0)
	form.SetButtonsAlign(tview.AlignRight)
	form.AddFormItem(peerInput)
	form.AddFormItem(portInput)
	form.AddFormItem(ifaceInput)
	form.AddDropDown(
		"Scope: ",
		[]string{
			traceScopePeerPort.Label() + " (Recommended)",
			traceScopePeerOnly.Label(),
		},
		scopeDefaultIndex,
		func(_ string, index int) {
			if index == 1 {
				scopeSelection = traceScopePeerOnly
				return
			}
			scopeSelection = traceScopePeerPort
		},
	)
	form.AddDropDown(
		"Direction: ",
		[]string{"ANY", "IN", "OUT"},
		int(directionSelection),
		func(_ string, index int) {
			switch index {
			case 1:
				directionSelection = traceDirectionIn
			case 2:
				directionSelection = traceDirectionOut
			default:
				directionSelection = traceDirectionAny
			}
		},
	)
	form.AddFormItem(durationInput)
	form.AddFormItem(packetCapInput)
	form.AddCheckbox("Save pcap: ", savePCAP, func(checked bool) {
		savePCAP = checked
	})

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

		port := 0
		if scopeSelection == traceScopePeerPort {
			port, err = parseTracePacketIntRange(portInput.GetText(), 1, 65535, "Port")
			if err != nil {
				a.setStatusNote(err.Error(), 5*time.Second)
				return
			}
		}

		req := tracePacketRequest{
			Interface:   ifaceName,
			PeerIP:      peerIP,
			Port:        port,
			Scope:       scopeSelection,
			Direction:   directionSelection,
			DurationSec: durationSec,
			PacketCap:   packetCap,
			SavePCAP:    savePCAP,
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
			closeForm()
			return nil
		}
		return event
	})

	form.SetBorder(true)
	form.SetTitle(" Trace Packet ")

	helpLine := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetTextAlign(tview.AlignLeft).
		SetText("  [dim]Use selected Top Connections row as seed. Start runs bounded tcpdump capture with safe defaults.[white]")

	modal := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
				AddItem(helpLine, 2, 0, false).
				AddItem(form, 0, 1, true),
				92, 0, true).
			AddItem(nil, 0, 1, false),
			18, 0, true).
		AddItem(nil, 0, 1, false)

	a.pages.RemovePage(tracePacketPageForm)
	a.pages.AddPage(tracePacketPageForm, modal, true, true)
	a.updateStatusBar()
	form.SetFocus(0)
	a.app.SetFocus(form)
}

func (a *App) selectedTracePacketSeed() (tracePacketSeed, bool) {
	if a.groupView {
		groups := a.visiblePeerGroups()
		if len(groups) == 0 {
			return tracePacketSeed{}, false
		}
		a.clampTopConnectionSelection()
		group := groups[a.selectedTalkerIndex]
		seed := tracePacketSeed{PeerIP: normalizeIP(group.PeerIP)}
		if a.topDirection == topConnectionIncoming {
			if target, ok := a.selectedPeerPortTarget(seed.PeerIP); ok {
				seed.Port = target.LocalPort
				return seed, true
			}
			ports := sortedPeerGroupPorts(group.LocalPorts)
			if len(ports) > 0 {
				seed.Port = ports[0]
			}
			return seed, true
		}
		ports := sortedPeerGroupPorts(group.RemotePorts)
		if len(ports) > 0 {
			seed.Port = ports[0]
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
		PeerIP: normalizeIP(row.RemoteIP),
	}
	if a.topDirection == topConnectionOutgoing {
		seed.Port = row.RemotePort
	} else {
		seed.Port = row.LocalPort
	}
	return seed, true
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
			"  [yellow]Trace Packet Running[white]\n\n  Interface: [green]%s[white]\n  Scope: [green]%s[white]\n  Direction: [green]%s[white]\n  Duration: [green]%ds[white] | Cap: [green]%d[white]\n  Remaining: [green]%ds[white]\n\n  [dim]Press Esc to abort.[white]",
			req.Interface,
			req.Scope.Label(),
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
	base := "tcp and host " + req.PeerIP
	if req.Scope == traceScopePeerOnly || req.Port <= 0 {
		return base
	}
	return fmt.Sprintf("%s and port %d", base, req.Port)
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
		peerPart := strings.NewReplacer(":", "-", ".", "_").Replace(req.PeerIP)
		portPart := "peer-only"
		if req.Port > 0 {
			portPart = strconv.Itoa(req.Port)
		}
		name := fmt.Sprintf(
			"trace-%s-%s-%s.pcap",
			time.Now().Format("20060102-150405"),
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
	b.WriteString(fmt.Sprintf("  Interface: [green]%s[white]\n", result.Request.Interface))
	b.WriteString(fmt.Sprintf("  Filter: [green]%s[white]\n", buildTracePacketDisplayFilter(result.Request, sensitiveIP)))
	b.WriteString(fmt.Sprintf("  Scope: [green]%s[white] | Direction: [green]%s[white]\n", result.Request.Scope.Label(), result.Request.Direction.Label()))
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
		"Trace %s %s:%s | dir=%s scope=%s | captured=%s drop=%s rst=%d | saved=%s",
		status,
		formatPreviewIP(result.Request.PeerIP, sensitiveIP),
		portPart,
		result.Request.Direction.Label(),
		result.Request.Scope.Label(),
		tracePacketMetricValue(result.Captured),
		dropped,
		result.RstCount,
		saved,
	)
}

func buildTracePacketDisplayFilter(req tracePacketRequest, sensitiveIP bool) string {
	base := "tcp and host " + formatPreviewIP(req.PeerIP, sensitiveIP)
	if req.Scope == traceScopePeerOnly || req.Port <= 0 {
		return base
	}
	return fmt.Sprintf("%s and port %d", base, req.Port)
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
