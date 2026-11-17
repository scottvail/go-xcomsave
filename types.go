// SPDX-License-Identifier: GPL-2.0-or-later
// Copyright (C) 2026 Scott Vail
// Ported from xcomsave, Copyright (C) 2015 tracktwo (https://github.com/tracktwo/xcomsave).

// Package xcomsave reads and writes XCOM: Enemy Unknown / Enemy Within saved
// games, and converts them to and from the JSON format used by the xcomsave
// tools (xcom2json / json2xcom).
//
// It is a Go port of xcomsave by tracktwo (https://github.com/tracktwo/xcomsave),
// which is licensed under the GNU GPL v2 or later.
//
// Typical use:
//
//	save, err := xcomsave.ReadFile("save62")      // binary save
//	js, err := save.MarshalXComJSON(xcomsave.JSONOptions{})
//	save2, err := xcomsave.UnmarshalXComJSON(js)
//	err = save2.WriteFile("save62")
package xcomsave

import "fmt"

// Version is a saved game format version.
type Version uint32

const (
	EnemyUnknown       Version = 0x0f
	EnemyWithin        Version = 0x10
	EnemyWithinAndroid Version = 0x13
)

// Supported reports whether this package can read and write the version.
func (v Version) Supported() bool {
	switch v {
	case EnemyUnknown, EnemyWithin, EnemyWithinAndroid:
		return true
	}
	return false
}

func (v Version) String() string {
	switch v {
	case EnemyUnknown:
		return "Enemy Unknown"
	case EnemyWithin:
		return "Enemy Within"
	case EnemyWithinAndroid:
		return "Enemy Within (Android)"
	}
	return fmt.Sprintf("unknown version 0x%x", uint32(v))
}

// String is a string that is stored either as Latin-1 (IsWide false) or as
// UTF-16 (IsWide true). Str always holds UTF-8.
type String struct {
	Str    string
	IsWide bool
}

// Header is the uncompressed save header.
type Header struct {
	Version          Version
	UncompressedSize int32
	GameNumber       int32
	SaveNumber       int32
	SaveDescription  String
	Time             String
	MapCommand       string
	TacticalSave     bool
	Ironman          bool
	Autosave         bool
	DLC              string
	Language         string

	// Android only.
	ProfileNumber int32
	ProfileDate   String
}

// Property is one serialized UnrealScript property. The concrete types are
// the *XxxProperty types in this package.
type Property interface {
	// PropertyName is the property's name, e.g. "iRank".
	PropertyName() string
	// Kind is the property kind as written in saves and JSON, e.g.
	// "IntProperty". All array variants report "ArrayProperty".
	Kind() string
}

type IntProperty struct {
	Name  string
	Value int32
}

type FloatProperty struct {
	Name  string
	Value float32
}

type BoolProperty struct {
	Name  string
	Value bool
}

// StrProperty is a string property.
type StrProperty struct {
	Name  string
	Value String
}

// NameProperty is an FName value.
type NameProperty struct {
	Name   string
	Value  string
	Number int32
}

// ObjectProperty references an actor by index into the actor table, or -1
// for none.
type ObjectProperty struct {
	Name  string
	Actor int32
}

// ByteProperty is an enum value. If Type is "None" it is a raw byte, stored
// in Number (and Value is "None").
type ByteProperty struct {
	Name   string
	Type   string
	Value  string
	Number int32
}

// StructProperty is a struct, either as nested properties or (for a few
// well-known native structs such as Vector) as raw bytes in NativeData.
type StructProperty struct {
	Name       string
	StructName string
	NativeData []byte
	Properties []Property
}

// ArrayProperty is a dynamic array whose element type could not be
// determined; its contents are kept as raw bytes.
type ArrayProperty struct {
	Name       string
	ArrayBound int32
	DataLength int32
	Data       []byte
}

// ObjectArrayProperty is a dynamic array of actor references.
type ObjectArrayProperty struct {
	Name   string
	Actors []int32
}

// NumberArrayProperty is a dynamic array of 4-byte numbers (ints or floats;
// the save does not say which).
type NumberArrayProperty struct {
	Name     string
	Elements []int32
}

// StructArrayProperty is a dynamic array of structs.
type StructArrayProperty struct {
	Name    string
	Structs [][]Property
}

// StringArrayProperty is a dynamic array of strings.
type StringArrayProperty struct {
	Name    string
	Strings []String
}

// EnumValue is one element of an EnumArrayProperty.
type EnumValue struct {
	Value  string
	Number int32
}

// EnumArrayProperty is a dynamic array of enum values.
type EnumArrayProperty struct {
	Name   string
	Values []EnumValue
}

// StaticArrayProperty is a fixed-size array. In the save it is stored as
// consecutive properties with the same name and increasing array indexes.
type StaticArrayProperty struct {
	Name       string
	Properties []Property
}

func (p *IntProperty) PropertyName() string         { return p.Name }
func (p *FloatProperty) PropertyName() string       { return p.Name }
func (p *BoolProperty) PropertyName() string        { return p.Name }
func (p *StrProperty) PropertyName() string         { return p.Name }
func (p *NameProperty) PropertyName() string        { return p.Name }
func (p *ObjectProperty) PropertyName() string      { return p.Name }
func (p *ByteProperty) PropertyName() string        { return p.Name }
func (p *StructProperty) PropertyName() string      { return p.Name }
func (p *ArrayProperty) PropertyName() string       { return p.Name }
func (p *ObjectArrayProperty) PropertyName() string { return p.Name }
func (p *NumberArrayProperty) PropertyName() string { return p.Name }
func (p *StructArrayProperty) PropertyName() string { return p.Name }
func (p *StringArrayProperty) PropertyName() string { return p.Name }
func (p *EnumArrayProperty) PropertyName() string   { return p.Name }
func (p *StaticArrayProperty) PropertyName() string { return p.Name }

func (*IntProperty) Kind() string         { return "IntProperty" }
func (*FloatProperty) Kind() string       { return "FloatProperty" }
func (*BoolProperty) Kind() string        { return "BoolProperty" }
func (*StrProperty) Kind() string         { return "StrProperty" }
func (*NameProperty) Kind() string        { return "NameProperty" }
func (*ObjectProperty) Kind() string      { return "ObjectProperty" }
func (*ByteProperty) Kind() string        { return "ByteProperty" }
func (*StructProperty) Kind() string      { return "StructProperty" }
func (*ArrayProperty) Kind() string       { return "ArrayProperty" }
func (*ObjectArrayProperty) Kind() string { return "ArrayProperty" }
func (*NumberArrayProperty) Kind() string { return "ArrayProperty" }
func (*StructArrayProperty) Kind() string { return "ArrayProperty" }
func (*StringArrayProperty) Kind() string { return "ArrayProperty" }
func (*EnumArrayProperty) Kind() string   { return "ArrayProperty" }
func (*StaticArrayProperty) Kind() string { return "StaticArrayProperty" }

// propertyKinds lists every kind string that can appear in a save.
var propertyKinds = []string{
	"IntProperty", "FloatProperty", "BoolProperty", "StrProperty",
	"ObjectProperty", "NameProperty", "ByteProperty", "StructProperty",
	"ArrayProperty", "StaticArrayProperty",
}

// Find returns the first property in props with the given name, or nil.
func Find(props []Property, name string) Property {
	for _, p := range props {
		if p.PropertyName() == name {
			return p
		}
	}
	return nil
}

// Checkpoint is one serialized actor.
type Checkpoint struct {
	Name          string
	InstanceName  string
	Vector        [3]float32
	Rotator       [3]int32
	ClassName     string
	Properties    []Property
	TemplateIndex int32
	PadSize       uint32
}

// NameEntry is an entry of a checkpoint chunk's name table.
type NameEntry struct {
	Name string
	Data []byte
}

// ActorTemplate is an entry of a checkpoint chunk's actor template table.
type ActorTemplate struct {
	ActorClassPath string
	LoadParams     [64]byte
	ArchetypePath  string
}

// CheckpointChunk is one checkpoint chunk (a map/level's worth of actors).
type CheckpointChunk struct {
	UnknownInt1 int32
	GameType    string
	Checkpoints []Checkpoint
	UnknownInt2 int32
	ClassName   string
	Actors      []string
	UnknownInt3 int32
	DisplayName string
	MapName     string
	UnknownInt4 int32

	// NameTable and ActorTemplates are normally empty for strategy saves.
	// They are preserved when reading and writing binary saves but, as with
	// xcom2json, are not included in the JSON format.
	NameTable      []NameEntry
	ActorTemplates []ActorTemplate
}

// Save is a complete saved game.
type Save struct {
	Header      Header
	Actors      []string
	Checkpoints []CheckpointChunk
}

// FormatError reports malformed save data.
type FormatError struct {
	Offset int
	Msg    string
}

func (e *FormatError) Error() string {
	return fmt.Sprintf("invalid save format at 0x%x: %s", e.Offset, e.Msg)
}

// CRCError reports a checksum mismatch.
type CRCError struct {
	Header           bool // header CRC (true) or compressed payload CRC (false)
	Expected, Actual uint32
}

func (e *CRCError) Error() string {
	what := "compressed data"
	if e.Header {
		what = "header"
	}
	return fmt.Sprintf("CRC mismatch in %s: expected 0x%08x, computed 0x%08x", what, e.Expected, e.Actual)
}

// UnsupportedVersionError reports a save version this package cannot handle.
type UnsupportedVersionError struct{ Version Version }

func (e *UnsupportedVersionError) Error() string {
	return fmt.Sprintf("unsupported save version 0x%x (supported: 0x0f Enemy Unknown, 0x10 Enemy Within, 0x13 Enemy Within Android)", uint32(e.Version))
}
