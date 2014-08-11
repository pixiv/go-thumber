package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/pixiv/go-thumber/thumbnail"
)

var params thumbnail.ThumbnailParameters

func main() {
	flag.IntVar(&params.Width, "w", 128, "target width")
	flag.IntVar(&params.Height, "h", 128, "target width")
	flag.BoolVar(&params.ForceAspect, "a", false, "force aspect")
	flag.BoolVar(&params.Upscale, "u", false, "also upscale if needed")
	flag.IntVar(&params.Quality, "q", 95, "JPEG quality")
	flag.BoolVar(&params.Optimize, "o", false, "optimize JPEG")
	flag.Float64Var(&params.PrescaleFactor, "p", 1.0, "prescale factor")
	flag.Parse()

	if flag.NArg() != 2 {
		fmt.Printf("Two non-flag args are required\n")
		os.Exit(1)
	}

	inputFilename := flag.Arg(0)
	outputFilename := flag.Arg(1)

	fmt.Printf("Thumbnailing %s\n", inputFilename)
	ifd, err := os.Open(inputFilename)
	if err != nil {
		panic(err)
	}
	ofd, err := os.Create(outputFilename)
	if err != nil {
		panic(err)
	}

	err = thumbnail.MakeThumbnail(ifd, ofd, params)
	if err != nil {
		panic(err)
	}
}
