// SPDX-License-Identifier: GPL-2.0-or-later
// Copyright (C) 2026 Scott Vail
// Ported from minilzo 2.09, Copyright (C) 1996-2015 Markus F.X.J. Oberhumer (http://www.oberhumer.com/opensource/lzo/).

// Package lzo implements LZO1X decompression and LZO1X-1 compression.
//
// The compressor is a straight port of lzo1x_1_compress from minilzo 2.09
// (the deterministic, 64-bit little-endian code path used by the x86-64
// builds of xcomsave), so it produces identical output for identical input.
package lzo

import (
	"encoding/binary"
	"errors"
)

const (
	m2MaxOffset = 0x0800
	m3MaxOffset = 0x4000
	m2MaxLen    = 8
	m3MaxLen    = 33
	m4MaxLen    = 9
	m3Marker    = 32
	m4Marker    = 16

	dBits = 14
	dMask = 1<<dBits - 1
)

// ErrCorrupt is returned when compressed input is malformed.
var ErrCorrupt = errors.New("lzo: corrupt compressed data")

// Compress compresses src using LZO1X-1.
func Compress(src []byte) []byte {
	out := make([]byte, 0, len(src)+len(src)/16+64+3)
	dict := make([]uint16, 1<<dBits)
	var (
		ip = 0 // position in src
		l  = len(src)
		t  = 0
	)
	for l > 20 {
		ll := min(l, 49152)
		// Mirrors minilzo's pointer-overflow guard, which in practice stops
		// when (t+ll)>>5 == 0.
		if (t+ll)>>5 == 0 {
			break
		}
		clear(dict)
		t, out = compressCore(src, ip, ll, out, t, dict)
		ip += ll
		l -= ll
	}
	t += l
	if t > 0 {
		ii := len(src) - t
		switch {
		case len(out) == 0 && t <= 238:
			out = append(out, byte(17+t))
		case t <= 3:
			out[len(out)-2] |= byte(t)
		case t <= 18:
			out = append(out, byte(t-3))
		default:
			tt := t - 18
			out = append(out, 0)
			for tt > 255 {
				tt -= 255
				out = append(out, 0)
			}
			out = append(out, byte(tt))
		}
		out = append(out, src[ii:ii+t]...)
	}
	return append(out, m4Marker|1, 0, 0)
}

func le32(b []byte, i int) uint32 { return binary.LittleEndian.Uint32(b[i:]) }
func le64(b []byte, i int) uint64 { return binary.LittleEndian.Uint64(b[i:]) }

// compressCore compresses src[in:in+inLen], with ti literal bytes pending
// from the previous block. It returns the number of trailing literals not
// yet emitted.
func compressCore(src []byte, in, inLen int, out []byte, ti int, dict []uint16) (int, []byte) {
	inEnd := in + inLen
	ipEnd := in + inLen - 20
	ip := in
	ii := ip
	if ti < 4 {
		ip += 4 - ti
	}
	var mPos int
	// minilzo enters its loop at the "literal" label, so the input position
	// advances once before the first lookup; after a match it does not.
	ip += 1 + (ip-ii)>>5
	for {
		// Find the next match (or run out of input).
		for {
			if ip >= ipEnd {
				goto done
			}
			dv := le32(src, ip)
			dindex := int((dv*0x1824429d)>>(32-dBits)) & dMask
			mPos = in + int(dict[dindex])
			dict[dindex] = uint16(ip - in)
			if dv == le32(src, mPos) {
				break
			}
			ip += 1 + (ip-ii)>>5
		}

		// Emit pending literals.
		ii -= ti
		ti = 0
		if t := ip - ii; t != 0 {
			if t <= 3 {
				out[len(out)-2] |= byte(t)
			} else if t <= 16 {
				out = append(out, byte(t-3))
			} else if t <= 18 {
				out = append(out, byte(t-3))
			} else {
				tt := t - 18
				out = append(out, 0)
				for tt > 255 {
					tt -= 255
					out = append(out, 0)
				}
				out = append(out, byte(tt))
			}
			out = append(out, src[ii:ii+t]...)
		}

		// Extend the match (64-bit little-endian variant).
		mLen := 4
		{
			v := le64(src, ip+mLen) ^ le64(src, mPos+mLen)
			if v == 0 {
				for {
					mLen += 8
					v = le64(src, ip+mLen) ^ le64(src, mPos+mLen)
					if ip+mLen >= ipEnd {
						goto mLenDone
					}
					if v != 0 {
						break
					}
				}
			}
			for v&0xff == 0 {
				v >>= 8
				mLen++
			}
		}
	mLenDone:
		mOff := ip - mPos
		ip += mLen
		ii = ip
		switch {
		case mLen <= m2MaxLen && mOff <= m2MaxOffset:
			mOff--
			out = append(out, byte((mLen-1)<<5|(mOff&7)<<2), byte(mOff>>3))
		case mOff <= m3MaxOffset:
			mOff--
			if mLen <= m3MaxLen {
				out = append(out, byte(m3Marker|(mLen-2)))
			} else {
				mLen -= m3MaxLen
				out = append(out, m3Marker)
				for mLen > 255 {
					mLen -= 255
					out = append(out, 0)
				}
				out = append(out, byte(mLen))
			}
			out = append(out, byte(mOff<<2), byte(mOff>>6))
		default:
			mOff -= 0x4000
			if mLen <= m4MaxLen {
				out = append(out, byte(m4Marker|(mOff>>11)&8|(mLen-2)))
			} else {
				mLen -= m4MaxLen
				out = append(out, byte(m4Marker|(mOff>>11)&8))
				for mLen > 255 {
					mLen -= 255
					out = append(out, 0)
				}
				out = append(out, byte(mLen))
			}
			out = append(out, byte(mOff<<2), byte(mOff>>6))
		}
	}
done:
	return inEnd - (ii - ti), out
}

// Decompress decompresses LZO1X data. outLen is the expected decompressed
// size; an error is returned if the data does not decompress to exactly that
// many bytes or is otherwise malformed.
func Decompress(src []byte, outLen int) (out []byte, err error) {
	// Malformed input can index out of range; report that as corruption
	// rather than crashing.
	defer func() {
		if r := recover(); r != nil {
			out, err = nil, ErrCorrupt
		}
	}()
	out = make([]byte, 0, outLen)
	ip := 0
	var t, mPos int

	copyMatch := func(n int) {
		if mPos < 0 || mPos >= len(out) {
			panic(ErrCorrupt)
		}
		for i := 0; i < n; i++ {
			out = append(out, out[mPos+i])
		}
	}
	copyLit := func(n int) {
		out = append(out, src[ip:ip+n]...)
		ip += n
	}
	runLen := func(base int) int {
		n := 0
		for src[ip] == 0 {
			n += 255
			ip++
		}
		n += base + int(src[ip])
		ip++
		return n
	}

	state := 0 // 0: expect literal run, 1: just copied a literal run
	if src[0] > 17 {
		t = int(src[0]) - 17
		ip++
		copyLit(t)
		if t < 4 {
			state = 2
		} else {
			state = 1
		}
	}

	for {
		switch state {
		case 0:
			t = int(src[ip])
			ip++
			if t < 16 {
				if t == 0 {
					t = runLen(15)
				}
				copyLit(t + 3)
				state = 1
				continue
			}
		case 1:
			// First instruction after a literal run.
			t = int(src[ip])
			ip++
			if t < 16 {
				mPos = len(out) - (1 + m2MaxOffset) - t>>2 - int(src[ip])<<2
				ip++
				copyMatch(3)
				goto matchDone
			}
		case 2:
			t = int(src[ip])
			ip++
		}

		// Match instruction in t.
		switch {
		case t >= 64:
			mPos = len(out) - 1 - (t>>2)&7 - int(src[ip])<<3
			ip++
			copyMatch(t>>5 - 1 + 2)
		case t >= 32:
			t &= 31
			if t == 0 {
				t = runLen(31)
			}
			mPos = len(out) - 1 - int(binary.LittleEndian.Uint16(src[ip:]))>>2
			ip += 2
			copyMatch(t + 2)
		case t >= 16:
			mPos = len(out) - (t&8)<<11
			t &= 7
			if t == 0 {
				t = runLen(7)
			}
			mPos -= int(binary.LittleEndian.Uint16(src[ip:])) >> 2
			ip += 2
			if mPos == len(out) {
				// End of stream marker.
				if ip != len(src) || len(out) != outLen {
					return nil, ErrCorrupt
				}
				return out, nil
			}
			mPos -= 0x4000
			copyMatch(t + 2)
		default:
			mPos = len(out) - 1 - t>>2 - int(src[ip])<<2
			ip++
			copyMatch(2)
		}
	matchDone:
		t = int(src[ip-2]) & 3
		if t == 0 {
			state = 0
			continue
		}
		copyLit(t)
		state = 2
		if len(out) > outLen {
			return nil, ErrCorrupt
		}
	}
}
