// Copyright (c) 2026 ALH477
// SPDX-License-Identifier: MIT

// Package metrics provides the Go types that mirror the proto/metrics.proto
// schema, together with hand-authored Marshal / Unmarshal implementations that
// use the official google.golang.org/protobuf/encoding/protowire package.
//
// The output of Marshal is byte-for-byte compatible with what protoc-gen-go
// would produce for the same .proto schema, so the log files can be consumed
// by any protobuf tooling.  The Makefile `proto` target shows how to
// regenerate code from the schema if you prefer that workflow instead.
package metrics

import (
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
)

// ── Field-number constants ────────────────────────────────────────────────────
// These MUST match the field numbers in proto/metrics.proto.

const (
	// Header fields
	hfHostname      protowire.Number = 1
	hfPlatform      protowire.Number = 2
	hfStartedUnixMs protowire.Number = 3
	hfNumCores      protowire.Number = 4

	// Sample fields
	sfTimestampUnixMs protowire.Number = 1
	sfCpuTotal        protowire.Number = 2
	sfCpuCores        protowire.Number = 3 // packed repeated double
	sfMemPercent      protowire.Number = 4
	sfMemUsedGB       protowire.Number = 5
	sfMemTotalGB      protowire.Number = 6
	sfLoad1           protowire.Number = 7
	sfLoad5           protowire.Number = 8
	sfLoad15          protowire.Number = 9
)

// ── Header ────────────────────────────────────────────────────────────────────

// Header is written once as the first record of every .infgo log file.
type Header struct {
	Hostname      string
	Platform      string
	StartedUnixMs int64
	NumCores      int32
}

// StartedTime converts StartedUnixMs to a time.Time in UTC.
func (h *Header) StartedTime() time.Time {
	return time.UnixMilli(h.StartedUnixMs).UTC()
}

// Marshal serialises h to protobuf binary.  Fields that hold zero/empty values
// are omitted to match the proto3 default-omit behaviour.
func (h *Header) Marshal() []byte {
	var b []byte
	if h.Hostname != "" {
		b = protowire.AppendTag(b, hfHostname, protowire.BytesType)
		b = protowire.AppendString(b, h.Hostname)
	}
	if h.Platform != "" {
		b = protowire.AppendTag(b, hfPlatform, protowire.BytesType)
		b = protowire.AppendString(b, h.Platform)
	}
	if h.StartedUnixMs != 0 {
		b = protowire.AppendTag(b, hfStartedUnixMs, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(h.StartedUnixMs))
	}
	if h.NumCores != 0 {
		b = protowire.AppendTag(b, hfNumCores, protowire.VarintType)
		b = protowire.AppendVarint(b, uint64(h.NumCores))
	}
	return b
}

// UnmarshalHeader deserialises a Header from protobuf binary.
func UnmarshalHeader(b []byte) (Header, error) {
	var h Header
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return h, fmt.Errorf("header: consume tag: %w", protowire.ParseError(n))
		}
		b = b[n:]

		switch {
		case num == hfHostname && typ == protowire.BytesType:
			v, n := protowire.ConsumeString(b)
			if n < 0 {
				return h, fmt.Errorf("header: hostname: %w", protowire.ParseError(n))
			}
			h.Hostname = v
			b = b[n:]

		case num == hfPlatform && typ == protowire.BytesType:
			v, n := protowire.ConsumeString(b)
			if n < 0 {
				return h, fmt.Errorf("header: platform: %w", protowire.ParseError(n))
			}
			h.Platform = v
			b = b[n:]

		case num == hfStartedUnixMs && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return h, fmt.Errorf("header: started_unix_ms: %w", protowire.ParseError(n))
			}
			h.StartedUnixMs = int64(v)
			b = b[n:]

		case num == hfNumCores && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return h, fmt.Errorf("header: num_cores: %w", protowire.ParseError(n))
			}
			h.NumCores = int32(v)
			b = b[n:]

		default:
			// Skip unknown fields for forward-compatibility.
			n := protowire.ConsumeFieldValue(num, typ, b)
			if n < 0 {
				return h, fmt.Errorf("header: skip unknown field %d: %w", num, protowire.ParseError(n))
			}
			b = b[n:]
		}
	}
	return h, nil
}

// ── Sample ────────────────────────────────────────────────────────────────────

// Sample is one snapshot of system metrics written every ~500 ms.
type Sample struct {
	TimestampUnixMs int64
	CpuTotal        float64   // aggregate 0-100 %
	CpuCores        []float64 // per-logical-core 0-100 %
	MemPercent      float64
	MemUsedGB       float64
	MemTotalGB      float64
	Load1           float64
	Load5           float64
	Load15          float64
}

// Time converts TimestampUnixMs to a time.Time in UTC.
func (s *Sample) Time() time.Time {
	return time.UnixMilli(s.TimestampUnixMs).UTC()
}

// Marshal serialises s to protobuf binary.
// CpuCores is encoded as a packed repeated double (field 3, wire type bytes),
// matching the `repeated double cpu_cores = 3` proto3 packed default.
func (s *Sample) Marshal() []byte {
	var b []byte

	// field 1: timestamp_unix_ms (int64 → varint)
	b = protowire.AppendTag(b, sfTimestampUnixMs, protowire.VarintType)
	b = protowire.AppendVarint(b, uint64(s.TimestampUnixMs))

	// field 2: cpu_total (double → fixed64)
	b = protowire.AppendTag(b, sfCpuTotal, protowire.Fixed64Type)
	b = protowire.AppendFixed64(b, math.Float64bits(s.CpuTotal))

	// field 3: cpu_cores (packed repeated double → bytes containing fixed64 values)
	if len(s.CpuCores) > 0 {
		packed := make([]byte, 0, len(s.CpuCores)*8)
		for _, c := range s.CpuCores {
			packed = binary.LittleEndian.AppendUint64(packed, math.Float64bits(c))
		}
		b = protowire.AppendTag(b, sfCpuCores, protowire.BytesType)
		b = protowire.AppendBytes(b, packed)
	}

	// fields 4-9: scalar doubles
	appendDouble := func(num protowire.Number, v float64) {
		b = protowire.AppendTag(b, num, protowire.Fixed64Type)
		b = protowire.AppendFixed64(b, math.Float64bits(v))
	}
	appendDouble(sfMemPercent, s.MemPercent)
	appendDouble(sfMemUsedGB, s.MemUsedGB)
	appendDouble(sfMemTotalGB, s.MemTotalGB)
	appendDouble(sfLoad1, s.Load1)
	appendDouble(sfLoad5, s.Load5)
	appendDouble(sfLoad15, s.Load15)

	return b
}

// UnmarshalSample deserialises a Sample from protobuf binary.
func UnmarshalSample(b []byte) (Sample, error) {
	var s Sample
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return s, fmt.Errorf("sample: consume tag: %w", protowire.ParseError(n))
		}
		b = b[n:]

		switch {
		case num == sfTimestampUnixMs && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return s, fmt.Errorf("sample: timestamp_unix_ms: %w", protowire.ParseError(n))
			}
			s.TimestampUnixMs = int64(v)
			b = b[n:]

		case num == sfCpuTotal && typ == protowire.Fixed64Type:
			v, n := protowire.ConsumeFixed64(b)
			if n < 0 {
				return s, fmt.Errorf("sample: cpu_total: %w", protowire.ParseError(n))
			}
			s.CpuTotal = math.Float64frombits(v)
			b = b[n:]

		case num == sfCpuCores && typ == protowire.BytesType:
			// Packed repeated double: payload is a sequence of little-endian uint64 values.
			raw, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return s, fmt.Errorf("sample: cpu_cores: %w", protowire.ParseError(n))
			}
			// Validate byte length is a multiple of 8.
			if len(raw)%8 != 0 {
				return s, fmt.Errorf("sample: cpu_cores packed length %d is not a multiple of 8", len(raw))
			}
			s.CpuCores = make([]float64, 0, len(raw)/8)
			for len(raw) >= 8 {
				bits := binary.LittleEndian.Uint64(raw[:8])
				s.CpuCores = append(s.CpuCores, math.Float64frombits(bits))
				raw = raw[8:]
			}
			b = b[n:]

		case num == sfMemPercent && typ == protowire.Fixed64Type:
			v, n := protowire.ConsumeFixed64(b)
			if n < 0 {
				return s, fmt.Errorf("sample: mem_percent: %w", protowire.ParseError(n))
			}
			s.MemPercent = math.Float64frombits(v)
			b = b[n:]

		case num == sfMemUsedGB && typ == protowire.Fixed64Type:
			v, n := protowire.ConsumeFixed64(b)
			if n < 0 {
				return s, fmt.Errorf("sample: mem_used_gb: %w", protowire.ParseError(n))
			}
			s.MemUsedGB = math.Float64frombits(v)
			b = b[n:]

		case num == sfMemTotalGB && typ == protowire.Fixed64Type:
			v, n := protowire.ConsumeFixed64(b)
			if n < 0 {
				return s, fmt.Errorf("sample: mem_total_gb: %w", protowire.ParseError(n))
			}
			s.MemTotalGB = math.Float64frombits(v)
			b = b[n:]

		case num == sfLoad1 && typ == protowire.Fixed64Type:
			v, n := protowire.ConsumeFixed64(b)
			if n < 0 {
				return s, fmt.Errorf("sample: load_1: %w", protowire.ParseError(n))
			}
			s.Load1 = math.Float64frombits(v)
			b = b[n:]

		case num == sfLoad5 && typ == protowire.Fixed64Type:
			v, n := protowire.ConsumeFixed64(b)
			if n < 0 {
				return s, fmt.Errorf("sample: load_5: %w", protowire.ParseError(n))
			}
			s.Load5 = math.Float64frombits(v)
			b = b[n:]

		case num == sfLoad15 && typ == protowire.Fixed64Type:
			v, n := protowire.ConsumeFixed64(b)
			if n < 0 {
				return s, fmt.Errorf("sample: load_15: %w", protowire.ParseError(n))
			}
			s.Load15 = math.Float64frombits(v)
			b = b[n:]

		default:
			// Skip unknown fields — forward-compatible with schema additions.
			n := protowire.ConsumeFieldValue(num, typ, b)
			if n < 0 {
				return s, fmt.Errorf("sample: skip unknown field %d: %w", num, protowire.ParseError(n))
			}
			b = b[n:]
		}
	}
	return s, nil
}
