// Copyright (c) 2026 ALH477
// SPDX-License-Identifier: MIT

// Package logger implements the .infgo binary log format.
//
// File layout:
//
//	[0:8]   Magic bytes: "INFGO\x01\x00"
//	Then N records, each structured as:
//	  [0]     Record type byte  (RecordTypeHeader=0x01 | RecordTypeSample=0x02)
//	  [1:5]   uint32 big-endian payload length
//	  [5:5+N] protobuf-encoded payload (metrics.Header or metrics.Sample)
//
// The Logger type is safe to use from a single goroutine only (Bubble Tea's
// Update method is single-threaded, so no synchronisation is needed there).
// The Reader type is likewise single-goroutine.
package logger

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/ALH477/infgo/metrics"
)

// magic is the 8-byte file header that identifies a .infgo log.
// Bytes 6-7 encode the format version (currently 0x01 0x00 = v1.0).
var magic = [8]byte{'I', 'N', 'F', 'G', 'O', 0x00, 0x01, 0x00}

// maxPayloadBytes is a sanity cap on individual record size to prevent
// corrupt files from causing unbounded memory allocation on read.
const maxPayloadBytes = 10 * 1024 * 1024 // 10 MiB

// RecordType discriminates the two record kinds in a log file.
type RecordType byte

const (
	RecordTypeHeader RecordType = 0x01
	RecordTypeSample RecordType = 0x02
)

// ── Logger (write) ────────────────────────────────────────────────────────────

// Logger writes binary activity records to a .infgo file.
// Call New to create one, then WriteHeader once, WriteSample per tick,
// and Close when the session ends.
type Logger struct {
	w    *bufio.Writer
	f    *os.File
	path string
}

// New creates (or truncates) the file at path, writes the magic header, and
// returns a Logger ready to accept records.  The caller must call Close.
func New(path string) (*Logger, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("logger: create %q: %w", path, err)
	}
	lgr := &Logger{
		f:    f,
		w:    bufio.NewWriterSize(f, 64*1024),
		path: path,
	}
	if _, err := lgr.w.Write(magic[:]); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("logger: write magic: %w", err)
	}
	return lgr, nil
}

// Path returns the filesystem path of the underlying log file.
func (l *Logger) Path() string { return l.path }

// WriteHeader serialises hdr and appends it to the log as a Header record.
// This should be called exactly once, immediately after the TUI receives
// the first sysInfoMsg so that hostname and platform are known.
func (l *Logger) WriteHeader(hdr metrics.Header) error {
	return l.appendRecord(RecordTypeHeader, hdr.Marshal())
}

// WriteSample serialises s and appends it to the log as a Sample record.
func (l *Logger) WriteSample(s metrics.Sample) error {
	return l.appendRecord(RecordTypeSample, s.Marshal())
}

// Close flushes any buffered data and closes the underlying file.
// It is safe to call Close more than once; subsequent calls return nil.
func (l *Logger) Close() error {
	if l.f == nil {
		return nil
	}
	if err := l.w.Flush(); err != nil {
		_ = l.f.Close()
		l.f = nil
		return fmt.Errorf("logger: flush %q: %w", l.path, err)
	}
	if err := l.f.Close(); err != nil {
		l.f = nil
		return fmt.Errorf("logger: close %q: %w", l.path, err)
	}
	l.f = nil
	return nil
}

// appendRecord writes: [type:1][length:4][payload:N]
func (l *Logger) appendRecord(rt RecordType, payload []byte) error {
	if err := l.w.WriteByte(byte(rt)); err != nil {
		return err
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(payload)))
	if _, err := l.w.Write(lenBuf[:]); err != nil {
		return err
	}
	_, err := l.w.Write(payload)
	return err
}

// ── Reader (read) ─────────────────────────────────────────────────────────────

// Record is a decoded entry from a .infgo log file.
// Exactly one of Header or Sample will be non-nil, depending on Type.
type Record struct {
	Type   RecordType
	Header *metrics.Header
	Sample *metrics.Sample
}

// Reader reads records sequentially from a .infgo log file.
type Reader struct {
	f *os.File
	r *bufio.Reader
}

// Open opens path, validates the magic bytes, and returns a Reader
// positioned at the first record.  The caller must call Close.
func Open(path string) (*Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("reader: open %q: %w", path, err)
	}
	br := bufio.NewReaderSize(f, 64*1024)

	var got [8]byte
	if _, err := io.ReadFull(br, got[:]); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("reader: read magic: %w", err)
	}
	if got != magic {
		_ = f.Close()
		return nil, fmt.Errorf("reader: %q is not a valid infgo log file (bad magic bytes)", path)
	}
	return &Reader{f: f, r: br}, nil
}

// Next reads and decodes the next record from the log.
// It returns (nil, io.EOF) when the file is exhausted.
func (r *Reader) Next() (*Record, error) {
	// Read the 1-byte type tag.
	typByte, err := r.r.ReadByte()
	if err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("reader: read type: %w", err)
	}
	rt := RecordType(typByte)

	// Read the 4-byte big-endian payload length.
	var lenBuf [4]byte
	if _, err := io.ReadFull(r.r, lenBuf[:]); err != nil {
		return nil, fmt.Errorf("reader: read length: %w", err)
	}
	payloadLen := binary.BigEndian.Uint32(lenBuf[:])

	if payloadLen > maxPayloadBytes {
		return nil, fmt.Errorf("reader: record payload too large (%d bytes); possible file corruption", payloadLen)
	}

	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(r.r, payload); err != nil {
		return nil, fmt.Errorf("reader: read payload: %w", err)
	}

	rec := &Record{Type: rt}
	switch rt {
	case RecordTypeHeader:
		hdr, err := metrics.UnmarshalHeader(payload)
		if err != nil {
			return nil, fmt.Errorf("reader: unmarshal header: %w", err)
		}
		rec.Header = &hdr

	case RecordTypeSample:
		s, err := metrics.UnmarshalSample(payload)
		if err != nil {
			return nil, fmt.Errorf("reader: unmarshal sample: %w", err)
		}
		rec.Sample = &s

	default:
		// Unknown record type — skip (forward-compatible with future versions).
		// rec.Header and rec.Sample remain nil; callers should check for this.
	}

	return rec, nil
}

// Close closes the underlying file.
func (r *Reader) Close() error {
	return r.f.Close()
}
