// Package swscale provides an interface to the libswscale library to scale
// YUV images.
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

// Filter identifies which scaling (interpolation) filter to use
type Filter int

// Supported scaling filters
const (
	FastBilinear Filter = C.SWS_FAST_BILINEAR
	Bilinear     Filter = C.SWS_BILINEAR
	Bicubic      Filter = C.SWS_BICUBIC
	X            Filter = C.SWS_X
	Point        Filter = C.SWS_POINT
	Area         Filter = C.SWS_AREA
	Bicublin     Filter = C.SWS_BICUBLIN
	Gauss        Filter = C.SWS_GAUSS
	Sinc         Filter = C.SWS_SINC
	Lanczos      Filter = C.SWS_LANCZOS
	Spline       Filter = C.SWS_SPLINE
)

// ScaleOptions contains scaling parameters
type ScaleOptions struct {
	DstWidth, DstHeight int    // Target dimensions
	Filter              Filter // Filter type
}

func pad(a int, b int) int {
	return (a + (b - 1)) & (^(b - 1))
}

// Scale a YUVImage and return the new YUVImage
func Scale(src *jpeg.YUVImage, opts ScaleOptions) (*jpeg.YUVImage, error) {
	// Figure out what format we're dealing with
	var srcFmt, dstFmt int32
	var flags C.int
	flags = C.SWS_FULL_CHR_H_INT | C.int(opts.Filter) | C.SWS_ACCURATE_RND
	components := 3
	var dst jpeg.YUVImage
	dstFmt = C.AV_PIX_FMT_YUV444P
	dst.Format = jpeg.YUV444
	switch src.Format {
	case jpeg.YUV444:
		srcFmt = C.AV_PIX_FMT_YUV444P
		flags |= C.SWS_FULL_CHR_H_INP
	case jpeg.YUV422:
		srcFmt = C.AV_PIX_FMT_YUV422P
	case jpeg.YUV440:
		srcFmt = C.AV_PIX_FMT_YUV440P
	case jpeg.YUV420:
		srcFmt = C.AV_PIX_FMT_YUV420P
	case jpeg.Grayscale:
		srcFmt = C.AV_PIX_FMT_GRAY8
		dstFmt = C.AV_PIX_FMT_GRAY8
		components = 1
		dst.Format = jpeg.Grayscale
	}

	// swscale can't handle images smaller than this; pad them
	paddedDstWidth := opts.DstWidth
	paddedSrcWidth := src.Width
	padFactor := 1
	for paddedDstWidth < 8 || paddedSrcWidth < 4 {
		paddedDstWidth *= 2
		paddedSrcWidth *= 2
		padFactor *= 2
	}

	// Get the SWS context
	sws := C.sws_getContext(C.int(paddedSrcWidth), C.int(src.Height), srcFmt,
		C.int(paddedDstWidth), C.int(opts.DstHeight), dstFmt,
		flags, nil, nil, nil)

	if sws == nil {
		return nil, errors.New("sws_getContext failed")
	}

	defer C.sws_freeContext(sws)

	// We only need 3 planes, but libswscale is stupid and checks the alignment
	// of all 4 pointers... better give it a dummy one.
	var srcYUVPtr [4](*uint8)
	var dstYUVPtr [4](*uint8)
	var srcStrides [4](C.int)
	var dstStrides [4](C.int)

	dst.Width = opts.DstWidth
	dst.Height = opts.DstHeight
	dstStride := pad(paddedDstWidth, jpeg.AlignSize)
	dstFinalPaddedWidth := pad(opts.DstWidth, jpeg.AlignSize)
	dstPaddedHeight := pad(opts.DstHeight, jpeg.AlignSize)
	// Allocate image planes and pointers
	for i := 0; i < components; i++ {
		dst.Stride[i] = dstStride
		dst.Data[i] = make([]byte, dstStride*dstPaddedHeight)
		dstYUVPtr[i] = (*uint8)(unsafe.Pointer(&dst.Data[i][0]))
		dstStrides[i] = C.int(dstStride)
		// apply horizontal padding if image is too small
		if padFactor > 1 {
			planeWidth := src.PlaneWidth(i)
			paddedWidth := planeWidth * padFactor
			planeHeight := src.PlaneHeight(i)
			paddedStride := pad(paddedWidth, jpeg.AlignSize)
			newData := make([]uint8, paddedStride*planeHeight)
			for y := 0; y < planeHeight; y++ {
				copy(newData[y*paddedStride:], src.Data[i][y*src.Stride[i]:y*src.Stride[i]+planeWidth])
				pixel := src.Data[i][y*src.Stride[i]+planeWidth-1]
				for x := planeWidth; x < paddedWidth; x++ {
					newData[y*paddedStride+x] = pixel
				}
			}
			srcStrides[i] = C.int(paddedStride)
			srcYUVPtr[i] = &newData[0]
		} else {
			srcStrides[i] = C.int(src.Stride[i])
			srcYUVPtr[i] = (*uint8)(unsafe.Pointer(&src.Data[i][0]))
		}
	}

	C.sws_scale(sws, (**C.uint8_t)(unsafe.Pointer(&srcYUVPtr[0])), &srcStrides[0], 0, C.int(src.Height),
		(**C.uint8_t)(unsafe.Pointer(&dstYUVPtr[0])), &dstStrides[0])

	// Replicate the last column and row of pixels as padding, which is typical
	// behavior prior to JPEG compression
	for i := 0; i < components; i++ {
		for y := 0; y < dst.Height; y++ {
			pixel := dst.Data[i][y*dstStride+dst.Width-1]
			for x := dst.Width; x < dstFinalPaddedWidth; x++ {
				dst.Data[i][y*dstStride+x] = pixel
			}
		}
		lastRow := dst.Data[i][dstStride*(dst.Height-1) : dstStride*dst.Height]
		for y := dst.Height; y < dstPaddedHeight; y++ {
			copy(dst.Data[i][y*dstStride:], lastRow)
		}
	}

	return &dst, nil
}
