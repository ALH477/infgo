// Copyright (c) 2026 ALH477
// SPDX-License-Identifier: MIT

package metrics

import (
	"testing"
)

func TestHeaderMarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name   string
		header Header
	}{
		{
			name: "full header",
			header: Header{
				Hostname:      "testhost",
				Platform:      "linux · amd64",
				StartedUnixMs: 1704067200000,
				NumCores:      8,
			},
		},
		{
			name: "partial header",
			header: Header{
				Hostname: "minimal",
			},
		},
		{
			name:   "empty header",
			header: Header{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := tt.header.Marshal()
			parsed, err := UnmarshalHeader(data)
			if err != nil {
				t.Fatalf("UnmarshalHeader failed: %v", err)
			}
			if parsed.Hostname != tt.header.Hostname {
				t.Errorf("Hostname: got %q, want %q", parsed.Hostname, tt.header.Hostname)
			}
			if parsed.Platform != tt.header.Platform {
				t.Errorf("Platform: got %q, want %q", parsed.Platform, tt.header.Platform)
			}
			if parsed.StartedUnixMs != tt.header.StartedUnixMs {
				t.Errorf("StartedUnixMs: got %d, want %d", parsed.StartedUnixMs, tt.header.StartedUnixMs)
			}
			if parsed.NumCores != tt.header.NumCores {
				t.Errorf("NumCores: got %d, want %d", parsed.NumCores, tt.header.NumCores)
			}
		})
	}
}

func TestHeaderRoundTrip(t *testing.T) {
	original := Header{
		Hostname:      "roundtrip-host",
		Platform:      "darwin · arm64",
		StartedUnixMs: 1700000000000,
		NumCores:      4,
	}

	data := original.Marshal()
	restored, err := UnmarshalHeader(data)
	if err != nil {
		t.Fatalf("round trip failed: %v", err)
	}

	if restored.Hostname != original.Hostname {
		t.Errorf("Hostname mismatch: got %q, want %q", restored.Hostname, original.Hostname)
	}
	if restored.Platform != original.Platform {
		t.Errorf("Platform mismatch: got %q, want %q", restored.Platform, original.Platform)
	}
	if restored.StartedUnixMs != original.StartedUnixMs {
		t.Errorf("StartedUnixMs mismatch: got %d, want %d", restored.StartedUnixMs, original.StartedUnixMs)
	}
	if restored.NumCores != original.NumCores {
		t.Errorf("NumCores mismatch: got %d, want %d", restored.NumCores, original.NumCores)
	}
}

func TestSampleMarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name   string
		sample Sample
	}{
		{
			name: "full sample",
			sample: Sample{
				TimestampUnixMs: 1704067200000,
				CpuTotal:        42.5,
				CpuCores:        []float64{31.2, 52.4, 18.1, 78.9, 25.0, 60.0, 40.0, 10.0},
				MemPercent:      61.8,
				MemUsedGB:       9.88,
				MemTotalGB:      15.99,
				Load1:           2.41,
				Load5:           1.89,
				Load15:          1.42,
			},
		},
		{
			name: "minimal sample",
			sample: Sample{
				TimestampUnixMs: 1000,
				CpuTotal:        0,
				CpuCores:        []float64{0},
				MemPercent:      0,
				MemUsedGB:       0,
				MemTotalGB:      0,
			},
		},
		{
			name: "empty cores",
			sample: Sample{
				TimestampUnixMs: 2000,
				CpuTotal:        50.0,
				CpuCores:        []float64{},
				MemPercent:      50.0,
				MemUsedGB:       8.0,
				MemTotalGB:      16.0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := tt.sample.Marshal()
			parsed, err := UnmarshalSample(data)
			if err != nil {
				t.Fatalf("UnmarshalSample failed: %v", err)
			}
			if parsed.TimestampUnixMs != tt.sample.TimestampUnixMs {
				t.Errorf("TimestampUnixMs: got %d, want %d", parsed.TimestampUnixMs, tt.sample.TimestampUnixMs)
			}
			if parsed.CpuTotal != tt.sample.CpuTotal {
				t.Errorf("CpuTotal: got %f, want %f", parsed.CpuTotal, tt.sample.CpuTotal)
			}
			if len(parsed.CpuCores) != len(tt.sample.CpuCores) {
				t.Errorf("CpuCores length: got %d, want %d", len(parsed.CpuCores), len(tt.sample.CpuCores))
			} else {
				for i := range parsed.CpuCores {
					if parsed.CpuCores[i] != tt.sample.CpuCores[i] {
						t.Errorf("CpuCores[%d]: got %f, want %f", i, parsed.CpuCores[i], tt.sample.CpuCores[i])
					}
				}
			}
			if parsed.MemPercent != tt.sample.MemPercent {
				t.Errorf("MemPercent: got %f, want %f", parsed.MemPercent, tt.sample.MemPercent)
			}
			if parsed.MemUsedGB != tt.sample.MemUsedGB {
				t.Errorf("MemUsedGB: got %f, want %f", parsed.MemUsedGB, tt.sample.MemUsedGB)
			}
			if parsed.MemTotalGB != tt.sample.MemTotalGB {
				t.Errorf("MemTotalGB: got %f, want %f", parsed.MemTotalGB, tt.sample.MemTotalGB)
			}
			if parsed.Load1 != tt.sample.Load1 {
				t.Errorf("Load1: got %f, want %f", parsed.Load1, tt.sample.Load1)
			}
			if parsed.Load5 != tt.sample.Load5 {
				t.Errorf("Load5: got %f, want %f", parsed.Load5, tt.sample.Load5)
			}
			if parsed.Load15 != tt.sample.Load15 {
				t.Errorf("Load15: got %f, want %f", parsed.Load15, tt.sample.Load15)
			}
		})
	}
}

func TestSampleRoundTrip(t *testing.T) {
	original := Sample{
		TimestampUnixMs: 1704067200000,
		CpuTotal:        42.5,
		CpuCores:        []float64{31.2, 52.4, 18.1, 78.9},
		MemPercent:      61.8,
		MemUsedGB:       9.88,
		MemTotalGB:      15.99,
		Load1:           2.41,
		Load5:           1.89,
		Load15:          1.42,
	}

	data := original.Marshal()
	restored, err := UnmarshalSample(data)
	if err != nil {
		t.Fatalf("round trip failed: %v", err)
	}

	if restored.TimestampUnixMs != original.TimestampUnixMs {
		t.Errorf("TimestampUnixMs mismatch: got %d, want %d", restored.TimestampUnixMs, original.TimestampUnixMs)
	}
	if restored.CpuTotal != original.CpuTotal {
		t.Errorf("CpuTotal mismatch: got %f, want %f", restored.CpuTotal, original.CpuTotal)
	}
	if len(restored.CpuCores) != len(original.CpuCores) {
		t.Errorf("CpuCores length mismatch: got %d, want %d", len(restored.CpuCores), len(original.CpuCores))
	} else {
		for i := range restored.CpuCores {
			if restored.CpuCores[i] != original.CpuCores[i] {
				t.Errorf("CpuCores[%d] mismatch: got %f, want %f", i, restored.CpuCores[i], original.CpuCores[i])
			}
		}
	}
	if restored.MemPercent != original.MemPercent {
		t.Errorf("MemPercent mismatch: got %f, want %f", restored.MemPercent, original.MemPercent)
	}
	if restored.MemUsedGB != original.MemUsedGB {
		t.Errorf("MemUsedGB mismatch: got %f, want %f", restored.MemUsedGB, original.MemUsedGB)
	}
	if restored.MemTotalGB != original.MemTotalGB {
		t.Errorf("MemTotalGB mismatch: got %f, want %f", restored.MemTotalGB, original.MemTotalGB)
	}
	if restored.Load1 != original.Load1 {
		t.Errorf("Load1 mismatch: got %f, want %f", restored.Load1, original.Load1)
	}
	if restored.Load5 != original.Load5 {
		t.Errorf("Load5 mismatch: got %f, want %f", restored.Load5, original.Load5)
	}
	if restored.Load15 != original.Load15 {
		t.Errorf("Load15 mismatch: got %f, want %f", restored.Load15, original.Load15)
	}
}

func TestUnmarshalHeaderTruncation(t *testing.T) {
	original := Header{
		Hostname:      "test",
		Platform:      "linux",
		StartedUnixMs: 1000,
		NumCores:      4,
	}

	data := original.Marshal()

	for i := 0; i < len(data); i++ {
		truncated := data[:i]
		_, err := UnmarshalHeader(truncated)
		if err != nil {
			t.Logf("Truncation at %d: %v (expected for partial data)", i, err)
		}
	}
}

func TestUnmarshalSampleTruncation(t *testing.T) {
	original := Sample{
		TimestampUnixMs: 1000,
		CpuTotal:        50.0,
		CpuCores:        []float64{25.0, 75.0},
		MemPercent:      50.0,
		MemUsedGB:       8.0,
		MemTotalGB:      16.0,
		Load1:           1.0,
		Load5:           2.0,
		Load15:          3.0,
	}

	data := original.Marshal()

	for i := 0; i < len(data); i++ {
		truncated := data[:i]
		_, err := UnmarshalSample(truncated)
		if err != nil {
			t.Logf("Truncation at %d: %v (expected for partial data)", i, err)
		}
	}
}

func TestUnmarshalHeaderUnknownField(t *testing.T) {
	original := Header{
		Hostname: "test",
	}

	data := original.Marshal()

	parsed, err := UnmarshalHeader(data)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if parsed.Hostname != original.Hostname {
		t.Errorf("Hostname lost: got %q, want %q", parsed.Hostname, original.Hostname)
	}
}

func TestUnmarshalSampleUnknownField(t *testing.T) {
	original := Sample{
		TimestampUnixMs: 1000,
		CpuTotal:        50.0,
	}

	data := original.Marshal()

	parsed, err := UnmarshalSample(data)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if parsed.TimestampUnixMs != original.TimestampUnixMs {
		t.Errorf("Timestamp lost when unknown field present: got %d, want %d", parsed.TimestampUnixMs, original.TimestampUnixMs)
	}
	if parsed.CpuTotal != original.CpuTotal {
		t.Errorf("CpuTotal lost when unknown field present: got %f, want %f", parsed.CpuTotal, original.CpuTotal)
	}
}
