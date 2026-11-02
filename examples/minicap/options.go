package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
)

// options defines capture projection, output, and client behavior.
//
// Example: choose a JPEG socket or raw stdout.
type options struct {
	projection                        projection
	quality                           int
	socketName                        string
	displayID                         int
	screenshot, test, info, skip, raw bool
	rate                              float64
}

// parseOptions validates the supported minicap-compatible command line.
//
// Example: parse -P 1080x2400@360x800/0 -Q 80.
func parseOptions(args []string) (options, error) {
	var o options
	fs := flag.NewFlagSet("minicap", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var spec string
	fs.StringVar(&spec, "P", "", "display projection")
	fs.IntVar(&o.quality, "Q", 80, "JPEG quality")
	fs.IntVar(&o.quality, "q", 80, "JPEG quality")
	fs.StringVar(&o.socketName, "n", "minicap", "abstract socket name")
	fs.IntVar(&o.displayID, "d", 0, "display ID")
	fs.BoolVar(&o.screenshot, "s", false, "write one JPEG")
	fs.BoolVar(&o.test, "t", false, "test capture")
	fs.BoolVar(&o.info, "i", false, "show display information")
	fs.BoolVar(&o.skip, "S", false, "skip queued frames")
	fs.BoolVar(&o.raw, "raw", false, "stream raw RGBA to stdout")
	fs.Float64Var(&o.rate, "r", 0, "maximum frame rate")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if fs.NArg() != 0 {
		return o, errors.New("unexpected arguments")
	}
	rateSpecified := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "r" {
			rateSpecified = true
		}
	})
	if o.quality < 0 || o.quality > 100 {
		return o, errors.New("quality must be between 0 and 100")
	}
	if o.displayID < 0 {
		return o, errors.New("display ID must be nonnegative")
	}
	if math.IsNaN(o.rate) || math.IsInf(o.rate, 0) || o.rate < 0 || (rateSpecified && o.rate == 0) || (o.rate > 0 && o.rate < 1e-9) {
		return o, errors.New("invalid frame rate")
	}
	modes := 0
	for _, mode := range []bool{o.screenshot, o.test, o.info, o.raw} {
		if mode {
			modes++
		}
	}
	if modes > 1 {
		return o, errors.New("choose one output mode")
	}
	if o.rate > 0 {
		o.skip = true
	}
	if o.info {
		return o, nil
	}
	if spec == "" {
		return o, errors.New("missing -P projection")
	}
	p, err := parseProjection(spec)
	if err != nil {
		return o, fmt.Errorf("-P: %w", err)
	}
	o.projection = p
	return o, nil
}
