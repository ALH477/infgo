# ⚡ infgo

[![License: MIT](https://img.shields.io/badge/License-MIT-a78bfa.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.22+-06b6d4.svg)](https://go.dev)
[![Built with Bubble Tea](https://img.shields.io/badge/Built%20with-Bubble%20Tea-7c3aed.svg)](https://github.com/charmbracelet/bubbletea)

A real-time terminal system-resource monitor built with the
[Bubble Tea](https://github.com/charmbracelet/bubbletea) TUI framework
following the Elm Architecture (Model / Update / View).

```
 ┏━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓
 ┃ ⠹  INFGO                                    myhost.local  ● LIVE  ┃
 ┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛

 ╭─────────────────────────────────────────────────────────────────────╮
 │  CPU   42.3%  ▲   peak 67.1%                                        │
 │                                                                      │
 │  ██████████████████░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░               │
 │                                                                      │
 │  ▁▁▂▂▃▄▃▂▃▄▅▄▃▂▃▄▅▆▅▄▃▄▅▃▂▁▂▃▄▅▆▅▃  ←19s                          │
 │                                                                      │
 │  CORES                                                               │
 │  [0] ▮▮▮▮▯▯▯▯ 31.2%   [1] ▮▮▮▮▮▮▯▯ 52.4%                          │
 │  [2] ▮▮▯▯▯▯▯▯ 18.1%   [3] ▮▮▮▮▮▮▮▮ 78.9%                          │
 ╰──────────────────────────────────────────────────────────────────────╯

 ╭─────────────────────────────────────────────────────────────────────╮
 │  MEMORY   61.8%                                                      │
 │                                                                      │
 │  ██████████████████████████████░░░░░░░░░░░░░░░░░░░░░░░░             │
 │  9.88 GiB used  ╱  15.99 GiB total  ╱  6.11 GiB free               │
 │                                                                      │
 │  ▄▄▅▅▅▅▅▅▅▅▅▅▅▅▅▆▆▆▆▆▆▆▆▆▆▆▆▆▆▆  ←19s                             │
 ╰──────────────────────────────────────────────────────────────────────╯

 ╭─── SYSTEM ────────────────────────────╮  ╭─── LOAD AVG ────────────╮
 │  SYSTEM                               │  │  LOAD AVG               │
 │                                       │  │                         │
 │  Host    myhost.local                 │  │  1m   ▮▮▮▮▯▯▯▯▯  2.41  │
 │  OS      linux · amd64               │  │  5m   ▮▮▮▯▯▯▯▯▯  1.89  │
 │  Uptime  3d 4h 22m                   │  │  15m  ▮▮▮▯▯▯▯▯▯  1.42  │
 │  Cores   8 logical                   │  │                         │
 ╰───────────────────────────────────────╯  ╰─────────────────────────╯
 ─────────────────────────────────────────────────────────────────────
  q · ctrl+c  quit                                            ↺ 500ms
```

## Features

| Feature | Detail |
|---|---|
| CPU aggregate | % averaged across all logical cores, heat-coded bar, trend arrow |
| Per-core grid | Up to 8 cores shown in a 2-column layout; overflow count displayed |
| Sparklines | 19-second rolling history for CPU and memory |
| Session peak | CPU high-watermark tracked for the lifetime of the process |
| Memory | Animated gradient progress bar (Bubbles component) + GiB breakdown |
| Load averages | 1 / 5 / 15 minute bars normalised against logical CPU count |
| System info | Hostname, OS, kernel arch, uptime, core count (fetched once at boot) |
| Heat borders | Panel borders turn amber ≥ 70 %, red ≥ 90 % |
| Responsive | Reflows on terminal resize; width clamped to 68–102 columns |

## Protobuf activity logging

infgo can record every metric sample to a binary `.infgo` log file for
offline analysis and chart generation.

### Record a session

```bash
infgo -log session.infgo
```

A `● REC  session.infgo` indicator appears in the footer while recording.
When you quit, the final buffer is flushed and you get:

```
infgo: activity log written to session.infgo
        run `analyze session.infgo` to generate a report
```

### Generate a report

```bash
# Build the analyzer
make build

# Analyze a log — prints stats + generates session_report.png
./bin/analyze session.infgo

# Custom output path
./bin/analyze -out /tmp/report.png session.infgo

# Text summary only, no chart
./bin/analyze -no-graph session.infgo
```

**Text summary output:**

```
  ┌──────────────────────────────────────────────────────┐
  │  infgo  ·  session report                           │
  └──────────────────────────────────────────────────────┘

  Host       myhost.local
  OS         linux · amd64
  Started    2024-01-15 14:23:07 UTC
  Duration   4m 32s
  Samples    544  (2.00 Hz)
  Cores      8 logical

                    min      avg      p95      max
  ──────────────────────────────────────────────────
  CPU %          12.3%    38.7%    72.1%    91.4%
  Memory %       61.2%    63.4%    65.1%    67.8%
  Load 1m         0.82     2.41     3.12     4.21
  Load 5m         0.71     2.10     2.88     3.95
  Load 15m        0.60     1.87     2.51     3.40
```

**Chart output** (two-panel PNG):
- Top panel: CPU % (violet) and Memory % (cyan) time-series with 70 % / 90 %
  threshold reference lines.
- Bottom panel: Load averages 1m / 5m / 15m normalised against the logical
  CPU count so they sit on the same 0–100 % scale.

### Binary log format

```
[0:8]   Magic  "INFGO\x01\x00"
[record …]
  [0]     type    0x01=Header  0x02=Sample
  [1:5]   length  uint32 big-endian
  [5:N]   payload protobuf binary (see proto/metrics.proto)
```

The payload is valid protobuf binary — any tool that understands the schema
(e.g. `protoc --decode`, Python/Rust protobuf libraries) can read it:

```bash
# Decode samples with protoc (requires proto/metrics.proto accessible)
dd if=session.infgo bs=1 skip=14 | \
  protoc --decode=metrics.Sample proto/metrics.proto
```

### Regenerating the Go types from the schema

```bash
# Install protoc + protoc-gen-go if not already present
brew install protobuf
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

make proto   # writes metrics/metrics.pb.go alongside the hand-authored encoding
```

The hand-authored `metrics/metrics.go` is the active implementation (it avoids
the `protoc` tool-chain dependency in CI).  After running `make proto` you can
optionally delete `metrics.go` and update imports to use the generated file.



### With Nix (recommended — fully reproducible)

```bash
# Enter the dev shell (Go, gopls, golangci-lint, delve, air, gomod2nix)
nix develop

# Run in place
go run .

# Build a static binary
go build -o infgo .
./infgo
```

### Without Nix (Go 1.22+ required)

```bash
go mod download
go run .
```

### Reproducible Nix binary (`nix build`)

The flake uses `buildGoModule`, which requires a `vendorHash`.  On first run:

```bash
# Let Nix tell you the real hash
nix build 2>&1 | grep "got:" | awk '{print $2}'
```

Paste the printed `sha256-…` value into `flake.nix → vendorHash`, then:

```bash
nix build
./result/bin/infgo
```

## Platform support

| OS | CPU | Memory | Load avg |
|---|---|---|---|
| Linux | ✅ | ✅ | ✅ |
| macOS | ✅ | ✅ | ✅ |
| Windows | ✅ | ✅ | ⚠️ not supported by gopsutil; displays 0.00 |

## Architecture

```
infgo/
├── main.go              TUI application (-log flag, logger lifecycle)
├── proto/
│   └── metrics.proto    Schema source of truth (field numbers + types)
├── metrics/
│   └── metrics.go       Header + Sample types; hand-authored protowire encoding
├── logger/
│   └── logger.go        Logger (write) + Reader (read) for .infgo binary files
└── cmd/
    └── analyze/
        └── main.go      Log parser + gonum/plot two-panel PNG report generator
```

### Dual-tick design

Two independent timers decouple animation from I/O:

```
animTick (110 ms) ──► frameCount++ only — zero syscalls
                        → spinner frame, live-dot colour

statsTick (500 ms) ──► fetchStats() goroutine
                         │
                         ▼
                       statsMsg ──► model update ──► View re-renders
                                         │
                                   progress.SetPercent()
                                         │
                                   progress.FrameMsg (easing loop)
```

This means the braille spinner and breathing live-dot animate at ~9 fps
regardless of how long gopsutil takes to sample the kernel.

### CPU sampling

```go
// Single per-core call; aggregate derived by averaging.
// This avoids the double-sample bug where calling Percent(0, false)
// then Percent(0, true) back-to-back causes the second call to measure
// a near-zero interval and return 0 % or 100 %.
cores, _ := cpu.Percent(0, true)
total = sum(cores) / len(cores)
```

## Keybindings

| Key | Action |
|---|---|
| `q` | Quit |
| `ctrl+c` | Quit |

## Dependencies

| Module | Version | Purpose |
|---|---|---|
| `charmbracelet/bubbletea` | v0.26.6 | Elm-Architecture TUI runtime |
| `charmbracelet/lipgloss` | v0.11.0 | Declarative terminal styling |
| `charmbracelet/bubbles` | v0.18.0 | Progress bar component |
| `shirou/gopsutil/v3` | v3.24.5 | Cross-platform CPU / mem / host stats |

## Changelog

### v0.3.0 (current)

**New: Protobuf activity logging system**
- `infgo -log <file.infgo>` records every metric sample as a protobuf binary record.
- `proto/metrics.proto` defines `Header` and `Sample` messages; field numbers are locked.
- `metrics/` package hand-implements protowire encoding/decoding — valid protobuf binary, zero generated-code dependency.
- `logger/` package provides `Logger` (buffered write) and `Reader` (validated read) with a 10 MiB sanity cap on record size.
- `cmd/analyze/` binary parses any `.infgo` log, prints a p95/avg/min/max summary table, and generates a two-panel gonum/plot PNG chart.
- `● REC` indicator in the TUI footer when logging is active.
- `Makefile` with `build`, `proto`, `run-log`, `analyze`, `lint`, `tidy`, `clean` targets.

### v0.2.0
**Bug fixes:**
- Fixed double `cpu.Percent(0, …)` call producing garbage per-core readings.
- Fixed per-core column misalignment (`padRunes` → `padVisual` / `lipgloss.Width`).
- Fixed double ANSI wrap in load-average panel.
- Fixed silent zero exit on fatal error.

### v0.1.0
- Initial release: CPU aggregate + sparkline, memory progress bar, system info.


## License

[MIT](LICENSE) © 2024 infgo contributors
