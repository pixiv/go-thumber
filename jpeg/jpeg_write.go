// Package jpeg implements reading and writing JPEG files as planar YUV data.
package jpeg

/*
Refer to jpeg_read.go for all the dragons. jpeg_write.go is just the summer
hangout of a few of them.
*/

/*
#cgo LDFLAGS: -ljpeg

#include <stdlib.h>
#include <stdio.h>
#include <jpeglib.h>

// jpeg_create_compress is a macro that cgo doesn't know about; wrap it.
static void c_jpeg_create_compress(j_compress_ptr cinfo) {
	jpeg_create_compress(cinfo);
}

void error_panic(j_common_ptr cinfo);

void destinationInit(struct jpeg_compress_struct*);
boolean destinationEmpty(struct jpeg_compress_struct*);
void destinationTerm(struct jpeg_compress_struct*);

*/
import "C"

import (
	"fmt"
	"io"
	"unsafe"
)

const writeBufferSize = 16384

type destinationManager struct {
	magic  uint32
	pub    C.struct_jpeg_destination_mgr
	buffer [writeBufferSize]byte
	dest   io.Writer
}

func getDestinationManager(cinfo *C.struct_jpeg_compress_struct) (ret *destinationManager) {
	// unsafe upcast magic to get the destinationManager associated with a cinfo
	ret = (*destinationManager)(unsafe.Pointer(uintptr(unsafe.Pointer(cinfo.dest)) - unsafe.Offsetof(destinationManager{}.pub)))
	// just in case this ever breaks in a future release for some reason,
	// check the magic
	if ret.magic != magic {
		panic("Invalid destinationManager magic; upcast failed.")
	}
	return
}

//export destinationInit
func destinationInit(cinfo *C.struct_jpeg_compress_struct) {
	// do nothing
}

func flushBuffer(mgr *destinationManager, inBuffer int) {
	wrote := 0
	for wrote != inBuffer {
		bytes, err := mgr.dest.Write(mgr.buffer[wrote:inBuffer])
		if err != nil {
			panic(err)
		}
		wrote += int(bytes)
	}
	mgr.pub.free_in_buffer = writeBufferSize
	mgr.pub.next_output_byte = (*C.JOCTET)(&mgr.buffer[0])
}

//export destinationEmpty
func destinationEmpty(cinfo *C.struct_jpeg_compress_struct) C.boolean {
	// need to write *entire* buffer, not subtracting free_in_buffer
	mgr := getDestinationManager(cinfo)
	flushBuffer(mgr, writeBufferSize)
	return C.TRUE
}

//export destinationTerm
func destinationTerm(cinfo *C.struct_jpeg_compress_struct) {
	// just empty buffer
	mgr := getDestinationManager(cinfo)
	inBuffer := int(writeBufferSize - mgr.pub.free_in_buffer)
	flushBuffer(mgr, inBuffer)
}

func makeDestinationManager(dest io.Writer, cinfo *C.struct_jpeg_compress_struct) (ret destinationManager) {
	ret.magic = magic
	ret.dest = dest
	ret.pub.init_destination = (*[0]byte)(C.destinationInit)
	ret.pub.empty_output_buffer = (*[0]byte)(C.destinationEmpty)
	ret.pub.term_destination = (*[0]byte)(C.destinationTerm)
	ret.pub.free_in_buffer = writeBufferSize
	ret.pub.next_output_byte = (*C.JOCTET)(&ret.buffer[0])
	cinfo.dest = &ret.pub
	return
}

// CompressionParameters specifies which settings to use during Compression.
type CompressionParameters struct {
	Quality  int  // Desired JPEG quality, 0-99
	Optimize bool // Whether to optimize the Huffman tables (slower)
	FastDCT  bool // Use a faster, less accurate DCT (note: do not use for Quality > 90)
}

// WriteJPEG writes a YUVImage as a JPEG into dest.
func WriteJPEG(img *YUVImage, dest io.Writer, params CompressionParameters) (err error) {
	defer func() {
		if r := recover(); r != nil {
			img = nil
			var ok bool
			err, ok = r.(error)
			if !ok {
				err = fmt.Errorf("JPEG error: %v", r)
			}
		}
	}()

	var cinfo C.struct_jpeg_compress_struct
	var jerr C.struct_jpeg_error_mgr

	// No subsampling suport for now
	if img.Format != YUV444 && img.Format != Grayscale {
		panic("Unsupported colorspace")
	}

	// Setup error handling
	C.jpeg_std_error(&jerr)
	jerr.error_exit = (*[0]byte)(C.error_panic)
	cinfo.err = &jerr

	// Initialize compression object
	C.c_jpeg_create_compress(&cinfo)
	makeDestinationManager(dest, &cinfo)

	// Set up compression parameters
	cinfo.image_width = C.JDIMENSION(img.Width)
	cinfo.image_height = C.JDIMENSION(img.Height)
	cinfo.input_components = 3
	cinfo.in_color_space = C.JCS_YCbCr
	if img.Format == Grayscale {
		cinfo.input_components = 1
		cinfo.in_color_space = C.JCS_GRAYSCALE
	}

	C.jpeg_set_defaults(&cinfo)
	C.jpeg_set_quality(&cinfo, C.int(params.Quality), C.TRUE)
	if params.Optimize {
		cinfo.optimize_coding = C.TRUE
	} else {
		cinfo.optimize_coding = C.FALSE
	}
	C.jpeg_set_colorspace(&cinfo, cinfo.in_color_space)
	if params.FastDCT {
		cinfo.dct_method = C.JDCT_IFAST
	} else {
		cinfo.dct_method = C.JDCT_ISLOW
	}
	compInfo := (*[3]C.jpeg_component_info)(unsafe.Pointer(cinfo.comp_info))

	for i := 0; i < int(cinfo.input_components); i++ {
		compInfo[i].h_samp_factor = 1
		compInfo[i].v_samp_factor = 1
	}

	// libjpeg raw data in is in planar format, which avoids unnecessary
	// planar->packed->planar conversions.
	cinfo.raw_data_in = C.TRUE

	// Start compression
	C.jpeg_start_compress(&cinfo, C.TRUE)

	// Allocate JSAMPIMAGE to hold pointers to one iMCU worth of image data
	// this is a safe overestimate; we use the return value from
	// jpeg_read_raw_data to figure out what is the actual iMCU row count.
	var yuvPtrInt [3][AlignSize]C.JSAMPROW
	yuvPtr := [3]C.JSAMPARRAY{
		C.JSAMPARRAY(unsafe.Pointer(&yuvPtrInt[0][0])),
		C.JSAMPARRAY(unsafe.Pointer(&yuvPtrInt[1][0])),
		C.JSAMPARRAY(unsafe.Pointer(&yuvPtrInt[2][0])),
	}

	// Encode the image.
	var row C.JDIMENSION
	for row = 0; row < cinfo.image_height; {
		// First fill in the pointers into the plane data buffers
		for i := 0; i < int(cinfo.num_components); i++ {
			for j := 0; j < int(C.DCTSIZE*compInfo[i].v_samp_factor); j++ {
				compRow := (int(row) + j)
				yuvPtrInt[i][j] = C.JSAMPROW(unsafe.Pointer(&img.Data[i][img.Stride[i]*compRow]))
			}
		}
		// Get the data
		row += C.jpeg_write_raw_data(&cinfo, C.JSAMPIMAGE(unsafe.Pointer(&yuvPtr[0])), C.JDIMENSION(C.DCTSIZE*compInfo[0].v_samp_factor))
	}

	// Clean up
	C.jpeg_finish_compress(&cinfo)
	C.jpeg_destroy_compress(&cinfo)

	return
}
