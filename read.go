// SPDX-License-Identifier: GPL-2.0-or-later
// Copyright (C) 2026 Scott Vail
// Ported from xcomsave, Copyright (C) 2015 tracktwo (https://github.com/tracktwo/xcomsave).

package xcomsave

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/scottvail/go-xcomsave/lzo"
)

const (
	compressedDataStart = 1024
	upkMagic            = 0x9e2a83c1
	chunkSize           = 0x20000
)

// ErrTacticalSave is returned for saves made during a tactical mission, which
// are not supported.
var ErrTacticalSave = errors.New("saved games in tactical missions are not supported; please use a geoscape save")

// ReadFile reads a binary save file.
func ReadFile(path string) (*Save, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Read(b)
}

// Read parses a binary save.
func Read(data []byte) (save *Save, err error) {
	defer recoverFormatError(&err)
	r := &reader{b: data}
	hdr := readHeader(r)
	if hdr.TacticalSave {
		return nil, ErrTacticalSave
	}
	raw, err := decompress(data, hdr.Version)
	if err != nil {
		return nil, err
	}
	return parseBody(hdr, raw), nil
}

// ReadHeader parses only the header of a binary save, verifying its
// checksums.
func ReadHeader(data []byte) (hdr Header, err error) {
	defer recoverFormatError(&err)
	return readHeader(&reader{b: data}), nil
}

// Decompress returns the decompressed body of a binary save (everything
// after the 1024-byte header). It does not verify checksums.
func Decompress(data []byte) ([]byte, error) {
	if len(data) < 4 {
		return nil, &FormatError{0, "file too short"}
	}
	return decompress(data, Version(le.Uint32(data)))
}

func recoverFormatError(err *error) {
	if r := recover(); r != nil {
		switch e := r.(type) {
		case *FormatError:
			*err = e
		case *CRCError:
			*err = e
		case *UnsupportedVersionError:
			*err = e
		default:
			panic(r)
		}
	}
}

func readHeader(r *reader) Header {
	var h Header
	h.Version = Version(r.int32())
	if !h.Version.Supported() {
		panic(&UnsupportedVersionError{h.Version})
	}
	h.UncompressedSize = r.int32()
	h.GameNumber = r.int32()
	h.SaveNumber = r.int32()
	h.SaveDescription, _ = r.unicodeString(true)
	h.Time, _ = r.unicodeString(true)
	h.MapCommand = r.string()
	h.TacticalSave = r.bool32()
	h.Ironman = r.bool32()
	h.Autosave = r.bool32()
	h.DLC = r.string()
	h.Language = r.string()
	compressedCRC := uint32(r.int32())

	if h.Version == EnemyWithinAndroid {
		r.pos += 12
		h.ProfileNumber = r.int32()
		h.ProfileDate, _ = r.unicodeString(true)
	} else {
		// The Android version has no header checksum.
		r.pos = 1016
		hdrSize := int(r.int32())
		hdrCRC := uint32(r.int32())
		if hdrSize < 0 || hdrSize > len(r.b) {
			r.fail("invalid header size %d", hdrSize)
		}
		if c := crc32b(r.b[:hdrSize]); c != hdrCRC {
			panic(&CRCError{Header: true, Expected: hdrCRC, Actual: c})
		}
	}
	if len(r.b) < compressedDataStart {
		r.fail("file too short")
	}
	if c := crc32b(r.b[compressedDataStart:]); c != compressedCRC {
		panic(&CRCError{Header: false, Expected: compressedCRC, Actual: c})
	}
	return h
}

// decompress decompresses the chunks following the header. Each chunk is a
// UE3 compressed-chunk: magic, block size, total compressed size, total
// uncompressed size, then (compressed, uncompressed) size pairs for each
// block, then the block data.
func decompress(data []byte, v Version) ([]byte, error) {
	if !v.Supported() {
		return nil, &UnsupportedVersionError{v}
	}
	var out []byte
	pos := compressedDataStart
	for pos < len(data) {
		if pos+16 > len(data) {
			return nil, &FormatError{pos, "truncated compressed chunk header"}
		}
		if le.Uint32(data[pos:]) != upkMagic {
			return nil, &FormatError{pos, "failed to find compressed chunk header"}
		}
		totalUncompressed := int(le.Uint32(data[pos+12:]))
		pos += 16
		type block struct{ c, u int }
		var blocks []block
		for sum := 0; sum < totalUncompressed; {
			if pos+8 > len(data) {
				return nil, &FormatError{pos, "truncated compressed block table"}
			}
			b := block{int(le.Uint32(data[pos:])), int(le.Uint32(data[pos+4:]))}
			if b.u <= 0 || b.c < 0 {
				return nil, &FormatError{pos, "invalid compressed block size"}
			}
			blocks = append(blocks, b)
			sum += b.u
			pos += 8
		}
		for _, b := range blocks {
			if pos+b.c > len(data) {
				return nil, &FormatError{pos, "truncated compressed block"}
			}
			d, err := decompressBlock(v, data[pos:pos+b.c], b.u)
			if err != nil {
				return nil, &FormatError{pos, "failed to decompress chunk: " + err.Error()}
			}
			out = append(out, d...)
			pos += b.c
		}
	}
	if len(out) == 0 {
		return nil, &FormatError{pos, "found no compressed data in save"}
	}
	return out, nil
}

func decompressBlock(v Version, src []byte, size int) ([]byte, error) {
	if v == EnemyWithinAndroid {
		zr, err := zlib.NewReader(bytes.NewReader(src))
		if err != nil {
			return nil, err
		}
		d, err := io.ReadAll(zr)
		if err != nil {
			return nil, err
		}
		if len(d) != size {
			return nil, fmt.Errorf("expected %d bytes, got %d", size, len(d))
		}
		return d, nil
	}
	return lzo.Decompress(src, size)
}

func parseBody(hdr Header, raw []byte) *Save {
	r := &reader{b: raw}
	s := &Save{Header: hdr}
	s.Actors = readActorTable(r, hdr.Version)
	for {
		s.Checkpoints = append(s.Checkpoints, readCheckpointChunk(r, hdr.Version))
		if r.eof() {
			break
		}
	}
	return s
}

func readActorTable(r *reader, v Version) []string {
	count := int(r.int32())
	var actors []string
	if v == EnemyUnknown {
		for i := 0; i < count; i++ {
			name := r.string()
			instance := r.int32()
			actors = append(actors, name+"_"+strconv.Itoa(int(instance)-1))
		}
		return actors
	}
	if count%2 != 0 {
		r.fail("actor table has an odd number of entries (%d)", count)
	}
	for i := 0; i < count; i += 2 {
		name := r.string()
		instance := r.int32()
		if instance == 0 {
			r.fail("malformed actor table entry: expected a non-zero instance")
		}
		pkg := r.string()
		if r.int32() != 0 {
			r.fail("malformed actor table entry: missing 0 instance")
		}
		actors = append(actors, pkg+"."+name+"_"+strconv.Itoa(int(instance)-1))
	}
	return actors
}

func readCheckpointChunk(r *reader, v Version) CheckpointChunk {
	var c CheckpointChunk
	c.UnknownInt1 = r.int32()
	c.GameType = r.string()
	if r.string() != "None" {
		r.fail("failed to locate 'None' after actor table")
	}
	c.UnknownInt2 = r.int32()
	c.Checkpoints = readCheckpointTable(r, v)

	nameCount := int(r.int32())
	for i := 0; i < nameCount; i++ {
		var e NameEntry
		e.Name = r.string()
		if !bytes.Equal(r.raw(8), make([]byte, 8)) {
			r.fail("expected all zeros in name table entry")
		}
		e.Data = r.raw(int(r.int32()))
		c.NameTable = append(c.NameTable, e)
	}

	c.ClassName = r.string()
	c.Actors = readActorTable(r, v)
	c.UnknownInt3 = r.int32()

	templateCount := int(r.int32())
	for i := 0; i < templateCount; i++ {
		var t ActorTemplate
		t.ActorClassPath = r.string()
		copy(t.LoadParams[:], r.raw(64))
		t.ArchetypePath = r.string()
		c.ActorTemplates = append(c.ActorTemplates, t)
	}

	c.DisplayName = r.string()
	c.MapName = r.string()
	c.UnknownInt4 = r.int32()
	return c
}

func readCheckpointTable(r *reader, v Version) []Checkpoint {
	count := int(r.int32())
	var table []Checkpoint
	for i := 0; i < count; i++ {
		var c Checkpoint
		c.Name = r.string()
		c.InstanceName = r.string()
		for j := range c.Vector {
			c.Vector[j] = r.float32()
		}
		for j := range c.Rotator {
			c.Rotator[j] = r.int32()
		}
		c.ClassName = r.string()
		propLength := int(r.int32())
		if propLength < 0 {
			r.fail("found negative property length")
		}
		start := r.pos
		c.Properties = readProperties(r, v)
		if used := r.pos - start; used < propLength {
			c.PadSize = uint32(propLength - used)
			for _, b := range r.raw(int(c.PadSize)) {
				if b != 0 {
					r.fail("found non-zero padding byte")
				}
			}
		}
		c.TemplateIndex = r.int32()
		table = append(table, c)
	}
	return table
}

func readProperties(r *reader, v Version) []Property {
	var props []Property
	for {
		name := r.string()
		if u := r.int32(); u != 0 {
			r.fail("read non-zero property unknown value: %x", u)
		}
		if name == "None" {
			return props
		}
		kind := r.string()
		if u := r.int32(); u != 0 {
			r.fail("read non-zero property unknown2 value: %x", u)
		}
		size := int(r.int32())
		arrayIndex := r.int32()

		var p Property
		switch kind {
		case "ObjectProperty":
			if v == EnemyUnknown {
				p = &ObjectProperty{name, r.int32()}
			} else {
				a1, a2 := r.int32(), r.int32()
				if a1 != -1 && a1 != a2+1 {
					r.fail("actor references in object property not related")
				}
				if a1 != -1 {
					a1 /= 2
				}
				p = &ObjectProperty{name, a1}
			}
		case "IntProperty":
			p = &IntProperty{name, r.int32()}
		case "ByteProperty":
			typ := r.string()
			if u := r.int32(); u != 0 {
				r.fail("read non-zero enum property unknown value: %x", u)
			}
			if typ == "None" {
				// A raw byte rather than an enum value.
				p = &ByteProperty{name, typ, "None", int32(r.byte())}
			} else {
				val := r.string()
				p = &ByteProperty{name, typ, val, r.int32()}
			}
		case "BoolProperty":
			p = &BoolProperty{name, r.byte() != 0}
		case "ArrayProperty":
			p = readArrayProperty(r, name, size, v)
		case "FloatProperty":
			p = &FloatProperty{name, r.float32()}
		case "StructProperty":
			p = readStructProperty(r, name, v)
		case "StrProperty":
			s, _ := r.unicodeString(true)
			p = &StrProperty{name, s}
		case "NameProperty":
			s := r.string()
			p = &NameProperty{name, s, r.int32()}
		default:
			r.fail("unknown property type %s", kind)
		}

		if arrayIndex == 0 {
			props = append(props, p)
			continue
		}
		// A non-zero array index means this is an element of a static array.
		if len(props) == 0 {
			r.fail("static array element %s[%d] without a preceding element", name, arrayIndex)
		}
		last := props[len(props)-1]
		if sa, ok := last.(*StaticArrayProperty); ok {
			sa.Properties = append(sa.Properties, p)
		} else {
			props[len(props)-1] = &StaticArrayProperty{Name: name, Properties: []Property{last, p}}
		}
	}
}

// nativeStructSizes lists structs that are serialized as raw bytes.
var nativeStructSizes = map[string]int{
	"Vector2D": 8,
	"Vector":   12,
	"Rotator":  12,
	"Box":      25,
	"Color":    4,
}

func readStructProperty(r *reader, name string, v Version) Property {
	structName := r.string()
	if u := r.int32(); u != 0 {
		r.fail("read non-zero prop unknown value in struct property: %x", u)
	}
	if n, ok := nativeStructSizes[structName]; ok {
		return &StructProperty{Name: name, StructName: structName, NativeData: r.raw(n)}
	}
	return &StructProperty{Name: name, StructName: structName, Properties: readProperties(r, v)}
}

type arrayKind int

const (
	arrayUnknown arrayKind = iota
	arrayStruct
	arrayEnum
	arrayString
)

// sniffArrayKind inspects (without consuming) the start of an array's data
// to guess its element type.
func sniffArrayKind(r *reader) arrayKind {
	saved := r.pos
	defer func() { r.pos = saved }()
	return sniffArrayKindAt(r)
}

func sniffArrayKindAt(r *reader) arrayKind {
	s, ok := r.unicodeString(false)
	if !ok || s.Str == "" {
		return arrayUnknown
	}
	// A leading "None" is ambiguous (an all-default struct, or an enum's
	// zero value); skip the element and look at the next one.
	if s.Str == "None" {
		if !r.has(4) {
			return arrayUnknown
		}
		r.int32()
		return sniffArrayKindAt(r)
	}
	// Two strings in a row: an array of strings.
	saved := r.pos
	if s2, ok := r.unicodeString(false); ok && s2.Str != "" {
		return arrayString
	}
	r.pos = saved
	// Otherwise an int (0 for structs, the enum number for enums) followed
	// by a property kind (struct) or the next enum value (enum).
	if !r.has(4) {
		return arrayUnknown
	}
	r.int32()
	s, ok = r.unicodeString(false)
	if !ok || s.Str == "" {
		return arrayUnknown
	}
	for _, k := range propertyKinds {
		if k == s.Str {
			return arrayStruct
		}
	}
	return arrayEnum
}

func readArrayProperty(r *reader, name string, size int, v Version) Property {
	bound := r.int32()
	dataSize := size - 4
	if dataSize > 0 {
		r.need(dataSize, "array property "+name)
		if bound < 0 {
			r.fail("array property %s has negative length %d", name, bound)
		}
		// Every element takes at least one byte, so this bounds allocations.
		capHint := min(int(bound), dataSize)
		switch {
		case int(bound)*8 == dataSize:
			// Actor references.
			actors := make([]int32, 0, capHint)
			for i := int32(0); i < bound; i++ {
				a1, a2 := r.int32(), r.int32()
				switch {
				case a1 == -1 && a2 == -1:
					actors = append(actors, -1)
				case a1 != a2+1:
					r.fail("expected related actor numbers in object array")
				default:
					actors = append(actors, a1/2)
				}
			}
			return &ObjectArrayProperty{name, actors}
		case int(bound)*4 == dataSize:
			elems := make([]int32, 0, capHint)
			for i := int32(0); i < bound; i++ {
				elems = append(elems, r.int32())
			}
			return &NumberArrayProperty{name, elems}
		}
		switch sniffArrayKind(r) {
		case arrayStruct:
			structs := make([][]Property, 0, capHint)
			for i := int32(0); i < bound; i++ {
				structs = append(structs, readProperties(r, v))
			}
			return &StructArrayProperty{name, structs}
		case arrayEnum:
			vals := make([]EnumValue, 0, capHint)
			for i := int32(0); i < bound; i++ {
				s := r.string()
				vals = append(vals, EnumValue{s, r.int32()})
			}
			return &EnumArrayProperty{name, vals}
		case arrayString:
			strs := make([]String, 0, capHint)
			for i := int32(0); i < bound; i++ {
				s, _ := r.unicodeString(true)
				strs = append(strs, s)
			}
			return &StringArrayProperty{name, strs}
		}
		return &ArrayProperty{Name: name, ArrayBound: bound, DataLength: int32(dataSize), Data: r.raw(dataSize)}
	}
	return &ArrayProperty{Name: name, ArrayBound: bound, DataLength: int32(dataSize)}
}
