package swscale

/*
#cgo LDFLAGS: -lswscale

#include <stdlib.h>
#include <stdio.h>
#include <libswscale/swscale.h>
*/
import "C"

import (
	"errors"
	"unsafe"

	"github.com/pixiv/go-thumber/jpeg"
)

type Filter int

const (
	FAST_BILINEAR Filter = C.SWS_FAST_BILINEAR
	BILINEAR      Filter = C.SWS_BILINEAR
	BICUBIC       Filter = C.SWS_BICUBIC
	X             Filter = C.SWS_X
	POINT         Filter = C.SWS_POINT
	AREA          Filter = C.SWS_AREA
	BICUBLIN      Filter = C.SWS_BICUBLIN
	GAUSS         Filter = C.SWS_GAUSS
	SINC          Filter = C.SWS_SINC
	LANCZOS       Filter = C.SWS_LANCZOS
	SPLINE        Filter = C.SWS_SPLINE
)

type ScaleOptions struct {
	DstWidth, DstHeight int
	Filter              Filter
}

func pad(a int, b int) int {
	return (a + (b - 1)) & (^(b - 1))
}

func Scale(src *jpeg.YUVImage, opts ScaleOptions) (*jpeg.YUVImage, error) {
	// Figure out what format we're dealing with
	var srcFmt, dstFmt int32
	var flags C.int
	flags = C.SWS_FULL_CHR_H_INT | C.int(opts.Filter)
	components := 3
	var dst jpeg.YUVImage
	dstFmt = C.PIX_FMT_YUV444P
	dst.Format = jpeg.YUV444
	switch src.Format {
	case jpeg.YUV444:
		srcFmt = C.PIX_FMT_YUV444P
		flags |= C.SWS_FULL_CHR_H_INP
	case jpeg.YUV422:
		srcFmt = C.PIX_FMT_YUV422P
	case jpeg.YUV440:
		srcFmt = C.PIX_FMT_YUV440P
	case jpeg.YUV420:
		srcFmt = C.PIX_FMT_YUV420P
	case jpeg.Grayscale:
		srcFmt = C.PIX_FMT_GRAY8
		dstFmt = C.PIX_FMT_GRAY8
		components = 1
		dst.Format = jpeg.Grayscale
	}

	// Get the SWS context
	sws := C.sws_getContext(C.int(src.Width), C.int(src.Height), srcFmt,
		C.int(opts.DstWidth), C.int(opts.DstHeight), dstFmt,
		flags, nil, nil, nil)

	if sws == nil {
		return nil, errors.New("sws_getContext failed")
	}

	// We only need 3 planes, but libswscale is stupid and checks the alignment
	// of all 4 pointers... better give it a dummy one.
	var srcYUVPtr [4](*uint8)
	var dstYUVPtr [4](*uint8)
	var srcStride [4](C.int)
	var dstStride [4](C.int)
	paddedWidth := pad(opts.DstWidth, jpeg.AlignSize)
	paddedHeight := pad(opts.DstHeight, jpeg.AlignSize)
	// Allocate image planes and pointers
	for i := 0; i < components; i++ {
		dst.Width = opts.DstWidth
		dst.Height = opts.DstHeight
		dst.Stride[i] = paddedWidth
		dst.Data[i] = make([]byte, dst.Stride[i]*paddedHeight)
		srcYUVPtr[i] = (*uint8)(unsafe.Pointer(&src.Data[i][0]))
		dstYUVPtr[i] = (*uint8)(unsafe.Pointer(&dst.Data[i][0]))
		srcStride[i] = C.int(src.Stride[i])
		dstStride[i] = C.int(dst.Stride[i])
	}

	C.sws_scale(sws, (**C.uint8_t)(unsafe.Pointer(&srcYUVPtr[0])), &srcStride[0], 0, C.int(src.Height),
		(**C.uint8_t)(unsafe.Pointer(&dstYUVPtr[0])), &dstStride[0])

	C.sws_freeContext(sws)

	// Replicate the last column and row of pixels as padding, which is typical
	// behavior prior to JPEG compression
	for i := 0; i < components; i++ {
		for y := 0; y < dst.Height; y++ {
			pixel := dst.Data[i][y*paddedWidth+dst.Width-1]
			for x := dst.Width; x < paddedWidth; x++ {
				dst.Data[i][y*paddedWidth+x] = pixel
			}
		}
		lastRow := dst.Data[i][paddedWidth*(dst.Height-1) : paddedWidth*dst.Height]
		for y := dst.Height; y < paddedHeight; y++ {
			copy(dst.Data[i][y*paddedWidth:], lastRow)
		}
	}

	return &dst, nil
}
