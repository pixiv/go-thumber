/* Here be dragons. */

package thumbnail

import (
	"github.com/pixiv/go-thumber/jpeg"
	"github.com/pixiv/go-thumber/swscale"
	"io"
	"math"
)

type ThumbnailParameters struct {
	Width          int
	Height         int
	Upscale        bool
	ForceAspect    bool
	Quality        int
	Optimize       bool
	PrescaleFactor float64
}

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
		opts.Filter = swscale.LANCZOS
		// swscale can't handle images smaller than this; punt and don't scale
		// them.
		if opts.DstWidth >= 8 && img.Width >= 4 {
			img, err = swscale.Scale(img, opts)
			if err != nil {
				return err
			}
		}
	}

	//fmt.Printf("%dx%d\n", img.Width, img.Height);

	var cparams jpeg.CompressionParameters

	cparams.Optimize = params.Optimize
	cparams.Quality = params.Quality

	return jpeg.WriteJPEG(img, dst, cparams)
}
