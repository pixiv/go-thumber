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

// Pixel formats
type PixelFormat int

const AlignSize int = 16

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

/* Represents a planar image. Data is stored in a raw array of bytes for each
 * plane, with an explicit stride (instead of a multidimensional array). */
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
