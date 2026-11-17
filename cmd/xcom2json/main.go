// SPDX-License-Identifier: GPL-2.0-or-later
// Copyright (C) 2026 Scott Vail

// Command xcom2json converts an XCOM: EU/EW saved game to JSON.
//
//	xcom2json [-o <out_file>] [-lf|-crlf] [-exact-floats] <in_file>
//
// The output file defaults to <in_file>.json.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/scottvail/go-xcomsave"
)

func main() {
	out := flag.String("o", "", "output file name (default <in_file>.json)")
	crlf := flag.Bool("crlf", runtime.GOOS == "windows", "use CRLF line endings (default on Windows)")
	lf := flag.Bool("lf", false, "use LF line endings")
	exact := flag.Bool("exact-floats", false, "write floats with full precision instead of 6 digits")
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
		*out = in + ".json"
	}
	save, err := xcomsave.ReadFile(in)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	js := save.MarshalXComJSON(xcomsave.JSONOptions{CRLF: *crlf && !*lf, ExactFloats: *exact})
	if err := os.WriteFile(*out, js, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
