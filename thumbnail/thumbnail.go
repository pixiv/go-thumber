// Package thumbnail provides a simple interface to thumbnail a JPEG stream and
// return the thumbnailed version.
package thumbnail

import (
	"io"
	"math"

	"github.com/pixiv/go-thumber/jpeg"
	"github.com/pixiv/go-thumber/swscale"
)

// ThumbnailParameters configures the thumbnailing process
type ThumbnailParameters struct {
	Width          int     // Target width
	Height         int     // Target height
	Upscale        bool    // Whether to upscale images that are smaller than the target
	ForceAspect    bool    // Whether the source aspect ratio should be preserved
	Quality        int     // JPEG quality (0-99)
	Optimize       bool    // Whether to optimize the JPEG huffman tables
	PrescaleFactor float64 // Controls whether optimized JPEG prescaling is used and how much.
}

// MakeThumbnail makes a thumbnail of a JPEG stream at src and writes it to dst.
func MakeThumbnail(src io.Reader, dst io.Writer, params ThumbnailParameters) error {
	var dparams jpeg.DecompressionParameters
	if params.PrescaleFactor > 0 {
		dparams.TargetWidth = int(math.Ceil(float64(params.Width) * params.PrescaleFactor))
		dparams.TargetHeight = int(math.Ceil(float64(params.Height) * params.PrescaleFactor))
	}
	img, err := jpeg.ReadJPEG(src, dparams)
	if err != nil {
		return err
	}
	//fmt.Printf("%dx%d\n", img.Width, img.Height);

	if !params.Upscale && !params.ForceAspect &&
		img.Width < params.Width && img.Height < params.Height {
		params.Width = img.Width
		params.Height = img.Height
	}

	if img.Width != params.Width || img.Height != params.Height ||
		(img.Format != jpeg.YUV444 && img.Format != jpeg.Grayscale) {

		var opts swscale.ScaleOptions
		opts.DstWidth = params.Width
		opts.DstHeight = params.Height
		if !params.ForceAspect {
			if opts.DstWidth > params.Height*img.Width/img.Height {
				opts.DstWidth = int(float64(params.Height*img.Width)/float64(img.Height) + 0.5)
			} else if opts.DstHeight > params.Width*img.Height/img.Width {
				opts.DstHeight = int(float64(params.Width*img.Height)/float64(img.Width) + 0.5)
			}
		}
		opts.Filter = swscale.Lanczos
		img, err = swscale.Scale(img, opts)
		if err != nil {
			return err
		}
	}

	//fmt.Printf("%dx%d\n", img.Width, img.Height);

	var cparams jpeg.CompressionParameters

	cparams.Optimize = params.Optimize
	cparams.Quality = params.Quality

	return jpeg.WriteJPEG(img, dst, cparams)
}
