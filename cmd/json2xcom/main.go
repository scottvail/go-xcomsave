// SPDX-License-Identifier: GPL-2.0-or-later
// Copyright (C) 2026 Scott Vail

// Command json2xcom converts JSON produced by xcom2json back into an
// XCOM: EU/EW saved game.
//
//	json2xcom [-o <out_file>] <in_file>
//
// By default the output is <in_file> without its ".json" extension, or
// with ".out" appended if that file already exists or the input has no
// ".json" extension.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/scottvail/go-xcomsave"
)

func main() {
	out := flag.String("o", "", "output file name")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [-o <out_file>] <in_file>\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}
	in := flag.Arg(0)
	if *out == "" {
		if i := strings.LastIndex(in, ".json"); i >= 0 {
			*out = in[:i]
			if _, err := os.Stat(*out); err == nil {
				*out += ".out"
			}
		} else {
			*out = in + ".out"
		}
	}
	save, err := xcomsave.ReadJSONFile(in)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	if err := save.WriteFile(*out); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
