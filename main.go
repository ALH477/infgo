// Copyright (c) 2026 ALH477
// SPDX-License-Identifier: MIT

// infgo is a real-time terminal system-resource monitor built with the
// Bubble Tea TUI framework (Elm Architecture).  It surfaces CPU usage
// (aggregate + per-core), memory usage, load averages, and basic host
// information, and refreshes every 500 ms without blocking the event loop.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"

	syslogger "github.com/ALH477/infgo/logger"
	"github.com/ALH477/infgo/metrics"
)

// ── Tuning constants ──────────────────────────────────────────────────────────

const (
	// statsInterval is how often gopsutil is queried for new readings.
	statsInterval = 500 * time.Millisecond

	// animInterval drives the spinner and live-dot pulse; kept well below the
	// stats interval so animations stay smooth without any extra I/O.
	animInterval = 110 * time.Millisecond

	// historyLen is the number of samples retained for sparkline graphs.
	// At 500 ms per sample this represents a 19-second rolling window.
	historyLen = 38

	// maxCoresShown caps the per-core grid so it doesn't overflow on
	// machines with many logical CPUs (e.g. 32-core servers).
	maxCoresShown = 8

	// minWidth / maxWidth are the content-width bounds used by innerWidth().
	minInnerWidth = 68
	maxInnerWidth = 102
)

// sparkChars is the Unicode block-element ramp used for sparklines.
var sparkChars = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// spinnerFrames is a 10-frame braille spinner.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// liveDotColors cycles to produce a breathing green pulse effect.
var liveDotColors = []lipgloss.Color{"#10b981", "#34d399", "#6ee7b7", "#34d399"}

// ── Colour palette ────────────────────────────────────────────────────────────

var (
	cViolet  = lipgloss.Color("#a78bfa")
	cViolet2 = lipgloss.Color("#7c3aed")
	cCyan    = lipgloss.Color("#06b6d4")
	cGreen   = lipgloss.Color("#10b981")
	cAmber   = lipgloss.Color("#f59e0b")
	cRed     = lipgloss.Color("#ef4444")
	cGray700 = lipgloss.Color("#374151")
	cGray500 = lipgloss.Color("#6b7280")
	cGray50  = lipgloss.Color("#f9fafb")
)

// ── Package-level base styles ─────────────────────────────────────────────────
//
// These are intentionally immutable value types; every .Foreground() /
// .Bold() call on them returns a *new* style, leaving the originals intact.

var (
	boldSt   = lipgloss.NewStyle().Bold(true)
	dimSt    = lipgloss.NewStyle().Foreground(cGray500)
	brightSt = lipgloss.NewStyle().Foreground(cGray50)
	labelSt  = lipgloss.NewStyle().Bold(true).Foreground(cViolet)
	accentSt = lipgloss.NewStyle().Foreground(cCyan)
)

// ── Tea messages ──────────────────────────────────────────────────────────────

// animTickMsg is sent by the fast animation timer (110 ms).
type animTickMsg time.Time

// statsTickMsg is sent by the slower stats timer (500 ms).
type statsTickMsg time.Time

// statsMsg carries a fresh snapshot of system metrics.
type statsMsg struct {
	cpuTotal   float64   // aggregate CPU % (averaged across all cores)
	cpuCores   []float64 // per-logical-core CPU %
	memPercent float64
	memUsedGB  float64
	memTotalGB float64
	load1      float64
	load5      float64
	load15     float64
}

// sysInfoMsg carries one-time host metadata fetched on startup.
type sysInfoMsg struct {
	hostname string
	platform string
	uptime   uint64 // seconds since boot
}

// ── Model ─────────────────────────────────────────────────────────────────────

// model is the single source of truth for the entire TUI (Elm Architecture).
type model struct {
	// Terminal geometry
	width  int
	height int

	// CPU state
	cpuTotal   float64
	cpuPrev    float64   // reading from the previous tick; used for trend arrow
	cpuCores   []float64 // per-core readings; may be nil before first fetch
	cpuHistory []float64 // rolling ring of historyLen readings
	cpuPeak    float64   // session high-watermark

	// Memory state
	memPercent float64
	memUsedGB  float64
	memTotalGB float64
	memHistory []float64

	// Load averages (unsupported on Windows; gopsutil returns 0 gracefully)
	load1  float64
	load5  float64
	load15 float64

	// Host info
	hostname string
	platform string
	uptime   uint64
	numCores int // logical CPU count, set once from runtime.NumCPU()

	// Animation counters (driven by animTick, no I/O)
	spinFrame  int
	liveDotIdx int
	frameCount int

	// Bubbles progress bar for memory (handles its own easing animation).
	memProgress progress.Model

	// ready is false until the first statsMsg arrives; prevents a blank frame.
	ready bool

	// logger writes binary protobuf records to a .infgo file.
	// nil when -log flag is not provided.
	logger  *syslogger.Logger
	logPath string // display-only; shown in the footer when active
}

func initialModel() model {
	p := progress.New(
		progress.WithGradient("#7c3aed", "#06b6d4"),
		progress.WithoutPercentage(), // we render our own value
		progress.WithWidth(50),
	)
	return model{
		width:       80,
		height:      24,
		cpuHistory:  make([]float64, historyLen),
		memHistory:  make([]float64, historyLen),
		numCores:    runtime.NumCPU(),
		memProgress: p,
	}
}

// ── Commands ──────────────────────────────────────────────────────────────────

func animTick() tea.Cmd {
	return tea.Tick(animInterval, func(t time.Time) tea.Msg {
		return animTickMsg(t)
	})
}

func statsTick() tea.Cmd {
	return tea.Tick(statsInterval, func(t time.Time) tea.Msg {
		return statsTickMsg(t)
	})
}

// fetchStats runs in a Bubble Tea goroutine (returned as a tea.Cmd) so it
// never blocks the event loop.
//
// FIX: Previously this called cpu.Percent(0, false) *and* cpu.Percent(0, true)
// in sequence.  Because interval=0 means "delta since last call", the second
// call measured a near-zero interval and returned garbage (0 % or 100 %).
// We now call only the per-core variant and derive the aggregate by averaging,
// which is consistent and requires a single kernel round-trip.
func fetchStats() tea.Cmd {
	return func() tea.Msg {
		// Per-core readings; interval=0 means delta since the previous call
		// (gopsutil stores the last sample in package-level state).
		cores, err := cpu.Percent(0, true)
		if err != nil || len(cores) == 0 {
			// Return a zero-value msg; model keeps its previous readings.
			return statsMsg{}
		}

		// Derive aggregate by averaging — avoids a second kernel round-trip
		// and keeps both readings temporally consistent.
		var total float64
		for _, c := range cores {
			total += c
		}
		total /= float64(len(cores))

		vm, err := mem.VirtualMemory()
		if err != nil {
			return statsMsg{cpuTotal: total, cpuCores: cores}
		}

		// load.Avg() is a no-op on Windows; gopsutil returns (nil, nil) there.
		avg, _ := load.Avg()
		var l1, l5, l15 float64
		if avg != nil {
			l1, l5, l15 = avg.Load1, avg.Load5, avg.Load15
		}

		const gb = 1 << 30
		return statsMsg{
			cpuTotal:   total,
			cpuCores:   cores,
			memPercent: vm.UsedPercent,
			memUsedGB:  float64(vm.Used) / gb,
			memTotalGB: float64(vm.Total) / gb,
			load1:      l1,
			load5:      l5,
			load15:     l15,
		}
	}
}

// fetchSysInfo is dispatched once at startup; result cached in model.
func fetchSysInfo() tea.Cmd {
	return func() tea.Msg {
		info, err := host.Info()
		if err != nil {
			return sysInfoMsg{hostname: "unknown", platform: "unknown"}
		}
		return sysInfoMsg{
			hostname: info.Hostname,
			platform: info.Platform + " · " + info.KernelArch,
			uptime:   info.Uptime,
		}
	}
}

// ── Init ──────────────────────────────────────────────────────────────────────

func (m model) Init() tea.Cmd {
	return tea.Batch(fetchStats(), fetchSysInfo(), animTick(), statsTick())
}

// ── Update ────────────────────────────────────────────────────────────────────

// pushHistory appends val to buf, evicting the oldest element.
// The returned slice reuses the underlying array.
func pushHistory(buf []float64, val float64) []float64 {
	return append(buf[1:], val)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Keep the Bubbles progress bar in sync with the actual terminal width.
		m.memProgress.Width = innerWidth(msg.Width) - 6
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

	// Fast tick — only mutates animation counters; no I/O whatsoever.
	case animTickMsg:
		m.frameCount++
		m.spinFrame = m.frameCount % len(spinnerFrames)
		m.liveDotIdx = (m.frameCount / 3) % len(liveDotColors)
		return m, animTick()

	// Slow tick — schedules a stats fetch goroutine for the next cycle.
	case statsTickMsg:
		return m, tea.Batch(fetchStats(), statsTick())

	case statsMsg:
		// Guard against zero-value msgs emitted when gopsutil returns an error.
		if len(msg.cpuCores) == 0 && !m.ready {
			return m, nil
		}
		m.cpuPrev = m.cpuTotal
		m.cpuTotal = msg.cpuTotal
		m.cpuCores = msg.cpuCores
		m.cpuHistory = pushHistory(m.cpuHistory, msg.cpuTotal)
		if msg.cpuTotal > m.cpuPeak {
			m.cpuPeak = msg.cpuTotal
		}
		m.memPercent = msg.memPercent
		m.memUsedGB = msg.memUsedGB
		m.memTotalGB = msg.memTotalGB
		m.memHistory = pushHistory(m.memHistory, msg.memPercent)
		m.load1, m.load5, m.load15 = msg.load1, msg.load5, msg.load15
		m.ready = true
		// Persist the sample to the activity log if logging is active.
		if m.logger != nil {
			_ = m.logger.WriteSample(metrics.Sample{
				TimestampUnixMs: time.Now().UnixMilli(),
				CpuTotal:        m.cpuTotal,
				CpuCores:        m.cpuCores,
				MemPercent:      m.memPercent,
				MemUsedGB:       m.memUsedGB,
				MemTotalGB:      m.memTotalGB,
				Load1:           m.load1,
				Load5:           m.load5,
				Load15:          m.load15,
			})
		}
		// SetPercent returns a FrameMsg command that drives the easing loop.
		return m, m.memProgress.SetPercent(msg.memPercent / 100)

	case sysInfoMsg:
		m.hostname = msg.hostname
		m.platform = msg.platform
		m.uptime = msg.uptime
		// Write the session header now that we know hostname and platform.
		if m.logger != nil {
			_ = m.logger.WriteHeader(metrics.Header{
				Hostname:      msg.hostname,
				Platform:      msg.platform,
				StartedUnixMs: time.Now().UnixMilli(),
				NumCores:      int32(m.numCores),
			})
		}
		return m, nil

	// Forward Bubbles frame messages so the progress bar can animate smoothly.
	case progress.FrameMsg:
		pm, cmd := m.memProgress.Update(msg)
		m.memProgress = pm.(progress.Model)
		return m, cmd
	}

	return m, nil
}

// ── View helpers ──────────────────────────────────────────────────────────────

// innerWidth returns the content width clamped to [minInnerWidth, maxInnerWidth].
// The outer View wrapper adds 2 chars of horizontal padding on each side.
func innerWidth(termW int) int {
	w := termW - 4
	if w < minInnerWidth {
		return minInnerWidth
	}
	if w > maxInnerWidth {
		return maxInnerWidth
	}
	return w
}

// loadColor maps a 0-100 percentage to a traffic-light colour.
func loadColor(pct float64) lipgloss.Color {
	switch {
	case pct >= 90:
		return cRed
	case pct >= 70:
		return cAmber
	default:
		return cGreen
	}
}

// heatPanel returns a rounded-border panel whose border colour reacts to load.
// The border stays neutral (gray) below 70 % to avoid visual noise.
func heatPanel(pct float64, totalW int) lipgloss.Style {
	bc := cGray700
	if pct >= 70 {
		bc = loadColor(pct)
	}
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(bc).
		Padding(0, 2).
		Width(totalW)
}

// filledBar renders a heat-coded full-width Unicode block bar.
func filledBar(pct float64, width int) string {
	filled := int(math.Round(pct / 100 * float64(width)))
	if filled > width {
		filled = width
	}
	empty := width - filled
	fc := loadColor(pct)
	return lipgloss.NewStyle().Foreground(fc).Render(strings.Repeat("█", filled)) +
		lipgloss.NewStyle().Foreground(cGray700).Render(strings.Repeat("░", empty))
}

// miniBar renders a compact heat-coded block bar using ▮/▯ runes.
func miniBar(pct float64, width int) string {
	filled := int(math.Round(pct / 100 * float64(width)))
	if filled > width {
		filled = width
	}
	empty := width - filled
	fc := loadColor(pct)
	return lipgloss.NewStyle().Foreground(fc).Render(strings.Repeat("▮", filled)) +
		lipgloss.NewStyle().Foreground(cGray700).Render(strings.Repeat("▯", empty))
}

// sparkline renders the history slice as Unicode spark characters.
// col is the foreground colour applied to the entire rune sequence.
func sparkline(history []float64, width int, col lipgloss.Color) string {
	n := len(history)
	start := 0
	if n > width {
		start = n - width
	}
	var sb strings.Builder
	for i := start; i < n; i++ {
		v := history[i]
		idx := int(v/100*float64(len(sparkChars)-1) + 0.5)
		if idx < 0 {
			idx = 0
		} else if idx >= len(sparkChars) {
			idx = len(sparkChars) - 1
		}
		sb.WriteRune(sparkChars[idx])
	}
	return lipgloss.NewStyle().Foreground(col).Render(sb.String())
}

// trendArrow compares two consecutive readings and returns a directional glyph.
// A deadband of ±3 % prevents jitter on stable loads.
func trendArrow(curr, prev float64) string {
	delta := curr - prev
	switch {
	case delta > 3:
		return lipgloss.NewStyle().Foreground(cRed).Render("▲")
	case delta < -3:
		return lipgloss.NewStyle().Foreground(cGreen).Render("▼")
	default:
		return dimSt.Render("─")
	}
}

// formatUptime converts a seconds-since-boot value to a human-readable string.
func formatUptime(s uint64) string {
	d := s / 86400
	h := (s % 86400) / 3600
	m := (s % 3600) / 60
	switch {
	case d > 0:
		return fmt.Sprintf("%dd %dh %dm", d, h, m)
	case h > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	default:
		return fmt.Sprintf("%dm", m)
	}
}

// sparkWindowSeconds returns the total seconds covered by the history buffer.
func sparkWindowSeconds() int {
	return int(statsInterval/time.Millisecond) * historyLen / 1000
}

// padVisual right-pads (or truncates) s to n *visible* columns, correctly
// accounting for ANSI escape sequences in s by using lipgloss.Width().
//
// FIX: the previous implementation used padRunes() which counted raw bytes /
// runes including escape codes, causing column misalignment in the per-core
// grid where miniBar() embeds ANSI colour sequences.
func padVisual(s string, n int) string {
	vw := lipgloss.Width(s)
	if vw >= n {
		return s
	}
	return s + strings.Repeat(" ", n-vw)
}

// ── Section renderers ─────────────────────────────────────────────────────────

func (m model) renderHeader(iw int) string {
	spinner := lipgloss.NewStyle().Foreground(cViolet).Render(spinnerFrames[m.spinFrame])
	title := boldSt.Copy().Foreground(cViolet).Render("INFGO")
	dot := lipgloss.NewStyle().Foreground(liveDotColors[m.liveDotIdx]).Bold(true).Render("●")
	liveLabel := dimSt.Render(" LIVE")

	left := spinner + "  " + title
	right := dimSt.Render(m.hostname+"  ") + dot + liveLabel

	// innerLen is the renderable width inside the border+padding box.
	innerLen := iw + 2
	gap := innerLen - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.ThickBorder()).
		BorderForeground(cViolet2).
		Padding(0, 1).
		Width(iw + 4).
		Render(left + strings.Repeat(" ", gap) + right)
}

func (m model) renderCPU(iw int) string {
	barW := iw - 20
	if barW < 10 {
		barW = 10
	}

	// ── Title row ─────────────────────────────────────────────────────────
	pctStr := boldSt.Copy().Foreground(loadColor(m.cpuTotal)).
		Render(fmt.Sprintf("%5.1f%%", m.cpuTotal))
	titleRow := labelSt.Render("CPU") + "  " + pctStr + "  " +
		trendArrow(m.cpuTotal, m.cpuPrev) + "   " +
		dimSt.Render(fmt.Sprintf("peak %4.1f%%", m.cpuPeak))

	// ── Main bar ──────────────────────────────────────────────────────────
	bar := filledBar(m.cpuTotal, barW)

	// ── Sparkline ─────────────────────────────────────────────────────────
	spark := sparkline(m.cpuHistory, barW, cViolet)
	sparkRow := spark + "  " + dimSt.Render(fmt.Sprintf("←%ds", sparkWindowSeconds()))

	// ── Per-core 2-column grid ────────────────────────────────────────────
	// FIX: use padVisual() (lipgloss.Width-aware) instead of the old
	// padRunes() which miscounted ANSI escape bytes as visible characters.
	cores := m.cpuCores
	if len(cores) > maxCoresShown {
		cores = cores[:maxCoresShown]
	}
	const coreBarW = 8
	colW := iw/2 - 1

	var coreLines []string
	for i := 0; i < len(cores); i += 2 {
		lCell := dimSt.Render(fmt.Sprintf("[%d] ", i)) +
			miniBar(cores[i], coreBarW) +
			dimSt.Render(fmt.Sprintf(" %4.1f%%", cores[i]))

		var rCell string
		if i+1 < len(cores) {
			rCell = dimSt.Render(fmt.Sprintf("[%d] ", i+1)) +
				miniBar(cores[i+1], coreBarW) +
				dimSt.Render(fmt.Sprintf(" %4.1f%%", cores[i+1]))
		}
		coreLines = append(coreLines, padVisual(lCell, colW)+" "+rCell)
	}
	if len(m.cpuCores) > maxCoresShown {
		coreLines = append(coreLines,
			dimSt.Render(fmt.Sprintf("  (+%d more cores)", len(m.cpuCores)-maxCoresShown)))
	}

	sections := append(
		[]string{titleRow, "", bar, "", sparkRow, "", dimSt.Render("CORES")},
		coreLines...,
	)
	return heatPanel(m.cpuTotal, iw+4).Render(strings.Join(sections, "\n"))
}

func (m model) renderMemory(iw int) string {
	freeGB := m.memTotalGB - m.memUsedGB

	pctStr := boldSt.Copy().Foreground(loadColor(m.memPercent)).
		Render(fmt.Sprintf("%5.1f%%", m.memPercent))
	titleRow := labelSt.Render("MEMORY") + "  " + pctStr

	// Update width on the local copy so the bar fills the panel correctly.
	// (This is a value receiver so the stored model is unaffected.)
	m.memProgress.Width = iw - 2

	statsRow := dimSt.Render(fmt.Sprintf(
		"%.2f GiB used  ╱  %.2f GiB total  ╱  %.2f GiB free",
		m.memUsedGB, m.memTotalGB, freeGB,
	))

	sparkW := iw - 14
	if sparkW < 5 {
		sparkW = 5
	}
	spark := sparkline(m.memHistory, sparkW, cCyan)
	sparkRow := spark + "  " + dimSt.Render(fmt.Sprintf("←%ds", sparkWindowSeconds()))

	body := strings.Join([]string{
		titleRow, "",
		m.memProgress.View(),
		statsRow, "",
		sparkRow,
	}, "\n")
	return heatPanel(m.memPercent, iw+4).Render(body)
}

func (m model) renderSystem(w int) string {
	rows := []struct{ k, v string }{
		{"Host  ", m.hostname},
		{"OS    ", m.platform},
		{"Uptime", formatUptime(m.uptime)},
		{"Cores ", fmt.Sprintf("%d logical", m.numCores)},
	}
	lines := []string{labelSt.Render("SYSTEM"), ""}
	for _, r := range rows {
		lines = append(lines, dimSt.Render(r.k)+"  "+brightSt.Render(r.v))
	}
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(cGray700).
		Padding(0, 2).
		Width(w).
		Render(strings.Join(lines, "\n"))
}

func (m model) renderLoad(w int) string {
	const lbW = 9
	maxLoad := float64(m.numCores)

	// barPct normalises a load-average value against the logical CPU count.
	barPct := func(v float64) float64 {
		p := v / maxLoad * 100
		if p > 100 {
			p = 100
		}
		return p
	}

	// FIX: previously wrapped miniBar() output in a redundant Foreground().Render()
	// which double-escaped the ANSI sequences already present inside miniBar.
	// Now we call miniBar directly.
	row := func(label string, v float64) string {
		pct := barPct(v)
		col := loadColor(pct)
		num := lipgloss.NewStyle().Foreground(col).Bold(true).Render(fmt.Sprintf("%.2f", v))
		return dimSt.Render(padVisual(label, 3)) + "  " + miniBar(pct, lbW) + "  " + num
	}

	body := strings.Join([]string{
		labelSt.Render("LOAD AVG"), "",
		row("1m", m.load1),
		row("5m", m.load5),
		row("15m", m.load15),
	}, "\n")

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(cGray700).
		Padding(0, 2).
		Width(w).
		Render(body)
}

func (m model) renderFooter(iw int) string {
	quit := accentSt.Copy().Bold(true).Render("q") + dimSt.Render(" · ") +
		accentSt.Copy().Bold(true).Render("ctrl+c") + dimSt.Render("  quit")
	badge := dimSt.Render("↺ 500ms")

	// Show a recording indicator when the activity log is active.
	if m.logPath != "" {
		recDot := lipgloss.NewStyle().Foreground(cRed).Bold(true).Render("●")
		recLabel := dimSt.Render(" REC  " + m.logPath)
		badge = recDot + recLabel + "  " + badge
	}

	totalW := iw + 4
	gap := totalW - lipgloss.Width(quit) - lipgloss.Width(badge) - 4
	if gap < 1 {
		gap = 1
	}

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderTop(true).
		BorderForeground(cGray700).
		Padding(0, 1).
		Width(totalW).
		Render(quit + strings.Repeat(" ", gap) + badge)
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m model) View() string {
	if !m.ready {
		sp := lipgloss.NewStyle().Foreground(cViolet).Render(spinnerFrames[m.spinFrame])
		return "\n  " + sp + dimSt.Render("  Initialising…") + "\n"
	}

	iw := innerWidth(m.width)

	// Bottom row: system info (wider) and load averages (narrower) side-by-side.
	sysW := (iw+4)*56/100 - 2
	loadW := iw + 4 - sysW - 3
	bottom := lipgloss.JoinHorizontal(lipgloss.Top,
		m.renderSystem(sysW),
		"  ",
		m.renderLoad(loadW),
	)

	out := strings.Join([]string{
		m.renderHeader(iw),
		"",
		m.renderCPU(iw),
		"",
		m.renderMemory(iw),
		"",
		bottom,
		m.renderFooter(iw),
	}, "\n")

	return lipgloss.NewStyle().Padding(0, 1).Render(out)
}

// ── Entry ─────────────────────────────────────────────────────────────────────

func main() {
	logPath := flag.String("log", "", "write activity log to `file.infgo` (binary protobuf)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: infgo [-log <file.infgo>]\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	m := initialModel()

	// Activate logging if -log was provided.
	if *logPath != "" {
		lgr, err := syslogger.New(*logPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "infgo: open log: %v\n", err)
			os.Exit(1)
		}
		m.logger = lgr
		m.logPath = *logPath
	}

	prog := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := prog.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "infgo: %v\n", err)
		os.Exit(1)
	}

	// Close the logger after the TUI exits so the final buffer is flushed.
	if fm, ok := finalModel.(model); ok && fm.logger != nil {
		if err := fm.logger.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "infgo: close log: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("infgo: activity log written to %s\n", fm.logPath)
		fmt.Printf("        run `analyze %s` to generate a report\n", fm.logPath)
	}
}
