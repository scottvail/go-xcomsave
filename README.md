# go-xcomsave

A pure-Go library and command-line tools for reading and writing
**XCOM: Enemy Unknown / Enemy Within** saved games. It is a Go port of
[tracktwo/xcomsave](https://github.com/tracktwo/xcomsave), the C++ library
behind `xcom2json` / `json2xcom`, and is compatible with it: it reads and
writes the same binary saves and the same JSON format.

- Enemy Unknown (0x0f), Enemy Within (0x10) and Enemy Within Android (0x13)
  saves.
- No cgo or external tools.
- Reading a save and writing it back produces a byte-identical file, and the
  JSON output is byte-identical to `xcom2json.exe`'s.

```sh
go get github.com/scottvail/go-xcomsave
```

## Library

```go
import "github.com/scottvail/go-xcomsave"

save, err := xcomsave.ReadFile("save62")                     // binary save -> model
js := save.MarshalXComJSON(xcomsave.JSONOptions{CRLF: true}) // model -> xcom2json JSON
save, err = xcomsave.ReadJSONFile("save62.json")             // JSON -> model
err = save.WriteFile("save62")                               // model -> binary save
```

The model (`Save`, `Checkpoint`, `IntProperty`, `StrProperty`, ...) can be
edited directly; `xcomsave.Find(props, "iRank")` looks up a property by name.

Notes:

- PC saves are LZO1X-compressed in 128 KB chunks (Android uses zlib). The
  [`lzo`](lzo/) subpackage is a port of minilzo's LZO1X-1 and reproduces the
  game's compressed output exactly.
- `xcom2json.exe` writes CRLF line endings, as `JSONOptions{CRLF: true}` does.
  The reference copy in `testdata/` is stored with LF.
- xcom2json writes floats with 6 significant digits, which can change the
  last bit of a float on the way back. `JSONOptions{ExactFloats: true}` writes
  them losslessly.
- Strings that are not wide (`is_wide: false`) are stored as Latin-1; writing
  characters outside Latin-1 to one returns an error rather than corrupting it.
- Tactical (mid-mission) saves are rejected, as in the original tools.

## Command-line tools

Go equivalents of the original tools:

```sh
go install github.com/scottvail/go-xcomsave/cmd/xcom2json@latest
go install github.com/scottvail/go-xcomsave/cmd/json2xcom@latest

xcom2json [-o out.json] [-crlf|-lf] [-exact-floats] save62
json2xcom [-o save62] save62.json
```

## Building

Requires Go 1.24 or later. Everything is pure Go (no cgo), so no C compiler
is needed and every platform can be cross-compiled from any host.

```sh
git clone https://github.com/scottvail/go-xcomsave
cd go-xcomsave

go build ./...    # build the library and tools (checks that everything compiles)
go test ./...     # run the tests
go vet ./...      # static checks, as run in CI
```

Build the command-line tools into `dist/`:

```sh
go build -trimpath -ldflags "-s -w" -o dist/ ./cmd/...
```

Cross-compile them for another platform by setting `GOOS` and `GOARCH`, for
example:

```sh
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o dist/windows-amd64/ ./cmd/...
GOOS=linux   GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o dist/linux-arm64/   ./cmd/...
GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o dist/darwin-arm64/  ./cmd/...
```

On Windows PowerShell, set the variables first instead:
`$env:GOOS="linux"; $env:GOARCH="arm64"; go build ...`.

To work on go-xcomsave together with a program that uses it, add both to a Go
workspace from the program's directory, and the program builds against your
local copy:

```sh
go work init . ../go-xcomsave
```

## Test data

`testdata/save62` and `testdata/x-eu-save62.json` are an Enemy Unknown save
and its `xcom2json.exe` output, the reference for the file format. They are
stored byte for byte (see `.gitattributes`), so make sure git does not
convert their line endings.

## License

Copyright (C) 2026 Scott Vail.

This program is free software; you can redistribute it and/or modify it under
the terms of the GNU General Public License as published by the Free Software
Foundation; either version 2 of the License, or (at your option) any later
version. See [LICENSE](LICENSE) for the full text.

This program is distributed in the hope that it will be useful, but WITHOUT ANY
WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A
PARTICULAR PURPOSE.

The `xcomsave` package is a port of
[xcomsave](https://github.com/tracktwo/xcomsave) (Copyright (C) 2015 tracktwo,
GPL v2 or later), and the `lzo` package is a port of
[minilzo](http://www.oberhumer.com/opensource/lzo/) (Copyright (C) 1996-2015
Markus F.X.J. Oberhumer, GPL v2 or later). This project is not affiliated with
either.
