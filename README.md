amterp/jsoncolor
=========

# Forked

This repo is a fork of [nwidger's jsoncolor](https://github.com/nwidger/jsoncolor) library.

This fork exists because the original is no longer maintained, and there's a bug where a space character is omitted after 
colons for non-collection values.

For example, the original library prints like this:

```json
{
  "foo":"bar", << notice missing space
  "quz": []    << notice *correct* space
}
```

This fork fixes that bug so the output becomes:

```json
{
  "foo": "bar",
  "quz": []
}
```

Additionally, this fork also updates Go to 1.24 and switches from
[fatih/color](https://github.com/fatih/color) to [amterp/color](https://github.com/amterp/color)
as the latter similarly contains bug fixes + improvements not yet merged upstream (though I'm hoping this [changes](https://github.com/fatih/color/pull/255)).

Thanks to the original contributors for their work! 🙏

# Original README

*Not quite original, contains updated references to correct libraries, etc.*

[![GoDoc](https://godoc.org/github.com/amterp/jsoncolor?status.svg)](https://godoc.org/github.com/amterp/jsoncolor)

`jsoncolor` is a drop-in replacement for `encoding/json`'s `Marshal`
and `MarshalIndent` functions and `Encoder` type which produce
colorized output using amterp's [color](https://github.com/amterp/color)
package.

## Installation

```
go get -u github.com/amterp/jsoncolor
```

## Usage

To use as a replacement for `encoding/json`, exchange

`import "encoding/json"` with `import json "github.com/amterp/jsoncolor"`.

`json.Marshal`, `json.MarshalIndent` and `json.NewEncoder` will now
produce colorized output.

## Custom Colors

The colors used for each type of token can be customized by creating a
custom `Formatter`, changing its `XXXColor` fields and then passing it
to `MarshalWithFormatter`, `MarshalIndentWithFormatter` or
`NewEncoderWithFormatter`.  If a `XXXColor` field of the custom
`Formatter` is not set, the corresponding `DefaultXXXColor` package
variable is used.  See
[color.New](https://godoc.org/github.com/amterp/color#New) for creating
custom color values and the
[GoDocs](https://godoc.org/github.com/amterp/jsoncolor#pkg-variables)
for the default colors.

``` go
import (
        "fmt"
        "log"

        "github.com/amterp/color"
        json "github.com/amterp/jsoncolor"
)

// create custom formatter
f := json.NewFormatter()

// set custom colors
f.StringColor = color.New(color.FgBlack, color.Bold)
f.TrueColor = color.New(color.FgWhite, color.Bold)
f.FalseColor = color.New(color.FgRed)
f.NumberColor = color.New(color.FgWhite)
f.NullColor = color.New(color.FgWhite, color.Bold)

// marshal v with custom formatter,
// dst contains colorized output
dst, err := json.MarshalWithFormatter(v, f)
if err != nil {
        log.Fatal(err)
}

// print colorized output to stdout
fmt.Println(string(dst))
```
