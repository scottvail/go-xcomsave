// SPDX-License-Identifier: GPL-2.0-or-later
// Copyright (C) 2026 Scott Vail
// Ported from xcomsave, Copyright (C) 2015 tracktwo (https://github.com/tracktwo/xcomsave).

package xcomsave

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"unicode/utf16"
)

var le = binary.LittleEndian

// crcTable is for the non-reflected CRC-32 (polynomial 0x04c11db7) used by
// Unreal Engine 3 saves.
var crcTable = func() (t [256]uint32) {
	for i := range t {
		c := uint32(i) << 24
		for j := 0; j < 8; j++ {
			if c&0x80000000 != 0 {
				c = c<<1 ^ 0x04c11db7
			} else {
				c <<= 1
			}
		}
		t[i] = c
	}
	return
}()

func crc32b(b []byte) uint32 {
	crc := uint32(0xffffffff)
	for _, c := range b {
		crc = crc<<8 ^ crcTable[byte(crc>>24)^c]
	}
	return ^crc
}

// reader reads primitive values from a save buffer. Errors are reported by
// panicking with *FormatError, which the public entry points recover.
type reader struct {
	b   []byte
	pos int
}

func (r *reader) fail(format string, a ...any) {
	panic(&FormatError{Offset: r.pos, Msg: fmt.Sprintf(format, a...)})
}

func (r *reader) eof() bool { return r.pos >= len(r.b) }

func (r *reader) need(n int, what string) {
	if n < 0 || r.pos+n > len(r.b) {
		r.fail("%s: unexpected end of data", what)
	}
}

func (r *reader) int32() int32 {
	r.need(4, "read_int")
	v := int32(le.Uint32(r.b[r.pos:]))
	r.pos += 4
	return v
}

func (r *reader) float32() float32 {
	r.need(4, "read_float")
	v := math.Float32frombits(le.Uint32(r.b[r.pos:]))
	r.pos += 4
	return v
}

func (r *reader) bool32() bool { return r.int32() != 0 }

func (r *reader) byte() byte {
	r.need(1, "read_byte")
	v := r.b[r.pos]
	r.pos++
	return v
}

func (r *reader) raw(n int) []byte {
	r.need(n, "read_raw_bytes")
	v := append([]byte(nil), r.b[r.pos:r.pos+n]...)
	r.pos += n
	return v
}

// string reads a narrow (Latin-1) string; a UTF-16 string here is an error.
func (r *reader) string() string {
	s, ok := r.unicodeString(true)
	if ok && s.IsWide {
		r.fail("found UTF-16 string in unexpected location")
	}
	return s.Str
}

// unicodeString reads a Latin-1 or UTF-16 string. If strict is false,
// malformed strings yield ok=false (with the read position undefined)
// instead of failing.
func (r *reader) unicodeString(strict bool) (s String, ok bool) {
	if !strict && !r.has(4) {
		return String{}, false
	}
	length := int(r.int32())
	if length == 0 {
		return String{}, true
	}
	bad := func(format string, a ...any) (String, bool) {
		if strict {
			r.fail(format, a...)
		}
		return String{}, false
	}
	if length < 0 {
		length = -length
		if !r.has(2 * length) {
			return bad("read_string found an invalid string length")
		}
		units := make([]uint16, 0, length)
		for i := 0; i < length; i++ {
			u := le.Uint16(r.b[r.pos+2*i:])
			if u == 0 {
				break
			}
			units = append(units, u)
		}
		r.pos += 2 * length
		return String{Str: string(utf16.Decode(units)), IsWide: true}, true
	}
	if !r.has(length) {
		return bad("read_string found an invalid string length")
	}
	b := r.b[r.pos : r.pos+length]
	if n := indexByte(b, 0); n != length-1 {
		return bad("string mismatch: expected length %d but found %d", length-1, n)
	}
	r.pos += length
	return String{Str: latin1ToUTF8(b[:length-1])}, true
}

func (r *reader) has(n int) bool { return n >= 0 && r.pos+n <= len(r.b) }

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return len(b)
}

func latin1ToUTF8(b []byte) string {
	var sb strings.Builder
	sb.Grow(len(b))
	for _, c := range b {
		sb.WriteRune(rune(c))
	}
	return sb.String()
}

// writer appends primitive values to a buffer.
type writer struct {
	b   []byte
	err error
}

func (w *writer) int32(v int32)     { w.b = le.AppendUint32(w.b, uint32(v)) }
func (w *writer) float32(v float32) { w.b = le.AppendUint32(w.b, math.Float32bits(v)) }
func (w *writer) byte(v byte)       { w.b = append(w.b, v) }
func (w *writer) raw(v []byte)      { w.b = append(w.b, v...) }

func (w *writer) bool32(v bool) {
	if v {
		w.int32(1)
	} else {
		w.int32(0)
	}
}

func (w *writer) setErr(err error) {
	if w.err == nil {
		w.err = err
	}
}

func (w *writer) string(s string) { w.unicodeString(String{Str: s}) }

func (w *writer) unicodeString(s String) {
	if s.Str == "" {
		w.int32(0)
		return
	}
	if s.IsWide {
		units := utf16.Encode([]rune(s.Str))
		w.int32(-int32(len(units) + 1))
		for _, u := range units {
			w.b = le.AppendUint16(w.b, u)
		}
		w.b = append(w.b, 0, 0)
		return
	}
	w.int32(int32(len([]rune(s.Str)) + 1))
	for _, c := range s.Str {
		if c > 0xff {
			w.setErr(fmt.Errorf("string %q contains characters that cannot be stored as Latin-1; set is_wide to true", s.Str))
			c = '?'
		}
		w.b = append(w.b, byte(c))
	}
	w.b = append(w.b, 0)
}
