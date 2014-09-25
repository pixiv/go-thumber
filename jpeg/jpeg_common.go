package jpeg

/*
#cgo LDFLAGS: -ljpeg

#include <stdlib.h>
#include <stdio.h>
#include <jpeglib.h>

void goPanic(char *);
*/
import "C"

//export goPanic
func goPanic(msg *C.char) {
	panic(C.GoString(msg))
}


// The dimension multiple to which data buffers should be aligned.
const AlignSize int = 16

// PixelFormat represents a JPEG pixel format (either YUVxxx or Grayscale).
type PixelFormat int

// Valid PixelFormats
const (
	Grayscale PixelFormat = iota
	YUV444                // 1x1 subsampling
	YUV422                // 2x2 subsampling
	YUV440                // 1x2 subsampling
	YUV420                // 2x2 subsampling
)

// Planes
const (
	Y = 0
	U = 1
	V = 2
)

// YUVImage represents a planar image. Data is stored in a raw array of bytes
// for each plane, with an explicit stride (instead of a multidimensional
// array). This should probably be replaced with image.YCbCr (which I didn't
// know existed...)
type YUVImage struct {
	Width, Height int
	Format        PixelFormat
	Data          [3][]byte
	Stride        [3]int
}

// Used to ensure that the unsafe upcast magic actually works as intended
const magic uint32 = 0xdeadbeef

func pad(a int, b int) int {
	return (a + (b - 1)) & (^(b - 1))
}

func (i *YUVImage) PlaneWidth(plane int) int {
	if plane != 0 && (i.Format == YUV422 || i.Format == YUV420) {
		return (i.Width + 1) / 2
	} else {
		return i.Width
	}
}

func (i *YUVImage) PlaneHeight(plane int) int {
	if plane != 0 && (i.Format == YUV440 || i.Format == YUV420) {
		return (i.Height + 1) / 2
	} else {
		return i.Height
	}
}
