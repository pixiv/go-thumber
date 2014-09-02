// Package jpeg implements reading and writing JPEG files as planar YUV data.
package jpeg

// Here be dragons.

/*
#cgo LDFLAGS: -ljpeg

#include <stdlib.h>
#include <stdio.h>
#include <jpeglib.h>

// jpeg_create_decompress is a macro that cgo doesn't know about; wrap it.
static void c_jpeg_create_decompress(j_decompress_ptr dinfo) {
	jpeg_create_decompress(dinfo);
}

void error_panic(j_common_ptr dinfo);

void sourceInit(struct jpeg_decompress_struct*);
void sourceSkip(struct jpeg_decompress_struct*, long);
boolean sourceFill(struct jpeg_decompress_struct*);
void sourceTerm(struct jpeg_decompress_struct*);

static int DCT_v_scaled_size(j_decompress_ptr dinfo, int component) {
#if JPEG_LIB_VERSION >= 70
	return dinfo->comp_info[component].DCT_v_scaled_size;
#else
	return dinfo->comp_info[component].DCT_scaled_size;
#endif
}

*/
import "C"

import (
	"fmt"
	"io"
	"unsafe"
)

const readBufferSize = 16384

type sourceManager struct {
	magic       uint32
	pub         C.struct_jpeg_source_mgr
	buffer      [readBufferSize]byte
	src         io.Reader
	startOfFile bool
	currentSize int
}

func getSourceManager(dinfo *C.struct_jpeg_decompress_struct) (ret *sourceManager) {
	// unsafe upcast magic to get the sourceManager associated with a dinfo
	ret = (*sourceManager)(unsafe.Pointer(uintptr(unsafe.Pointer(dinfo.src)) - unsafe.Offsetof(sourceManager{}.pub)))
	// just in case this ever breaks in a future release for some reason,
	// check the magic
	if ret.magic != magic {
		panic("Invalid sourceManager magic; upcast failed.")
	}
	return
}

//export sourceInit
func sourceInit(dinfo *C.struct_jpeg_decompress_struct) {
	mgr := getSourceManager(dinfo)
	mgr.startOfFile = true
}

//export sourceFill
func sourceFill(dinfo *C.struct_jpeg_decompress_struct) C.boolean {
	mgr := getSourceManager(dinfo)
	bytes, err := mgr.src.Read(mgr.buffer[:])
	mgr.pub.bytes_in_buffer = C.size_t(bytes)
	mgr.currentSize = bytes
	mgr.pub.next_input_byte = (*C.JOCTET)(&mgr.buffer[0])
	if err == io.EOF {
		if bytes == 0 {
			if mgr.startOfFile {
				panic("input is empty")
			}
			// EOF and need more data. Fill in a fake EOI to get a partial image.
			mgr.buffer[0] = 0xff
			mgr.buffer[1] = C.JPEG_EOI
			mgr.pub.bytes_in_buffer = 2
		}
	} else if err != nil {
		panic(err)
	}
	mgr.startOfFile = false

	return C.TRUE
}

//export sourceSkip
func sourceSkip(dinfo *C.struct_jpeg_decompress_struct, bytes C.long) {
	mgr := getSourceManager(dinfo)
	if bytes > 0 {
		for bytes >= C.long(mgr.pub.bytes_in_buffer) {
			bytes -= C.long(mgr.pub.bytes_in_buffer)
			sourceFill(dinfo)
		}
	}
	mgr.pub.bytes_in_buffer -= C.size_t(bytes)
	if mgr.pub.bytes_in_buffer != 0 {
		mgr.pub.next_input_byte = (*C.JOCTET)(&mgr.buffer[mgr.currentSize-int(mgr.pub.bytes_in_buffer)])
	}
}

//export sourceTerm
func sourceTerm(dinfo *C.struct_jpeg_decompress_struct) {
	// do nothing
}

func makeSourceManager(src io.Reader, dinfo *C.struct_jpeg_decompress_struct) (ret *sourceManager) {
	ret = (*sourceManager)(C.malloc(C.size_t(unsafe.Sizeof(sourceManager{}))))
	if ret == nil {
		panic("Failed to allocate sourceManager")
	}
	ret.magic = magic
	ret.src = src
	ret.pub.init_source = (*[0]byte)(C.sourceInit)
	ret.pub.fill_input_buffer = (*[0]byte)(C.sourceFill)
	ret.pub.skip_input_data = (*[0]byte)(C.sourceSkip)
	ret.pub.resync_to_restart = (*[0]byte)(C.jpeg_resync_to_restart) // default implementation
	ret.pub.term_source = (*[0]byte)(C.sourceTerm)
	ret.pub.bytes_in_buffer = 0
	ret.pub.next_input_byte = nil
	dinfo.src = &ret.pub
	return
}

// DecompressionParameters specifies which settings to use during decompression.
// TargetWidth, TargetHeight specify the minimum image dimensions required. The
// image will be downsampled if applicable.
type DecompressionParameters struct {
	TargetWidth  int  // Desired output width
	TargetHeight int  // Desired output height
	FastDCT      bool // Use a faster, less accurate DCT (note: do not use for Quality > 90)
}

// ReadJPEG reads a JPEG file and returns a planar YUV image.
func ReadJPEG(src io.Reader, params DecompressionParameters) (img *YUVImage, err error) {
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

	dinfo := (*C.struct_jpeg_decompress_struct)(C.malloc(C.size_t(unsafe.Sizeof(C.struct_jpeg_decompress_struct{}))))
	if dinfo == nil {
		panic("Failed to allocate dinfo")
	}
	defer C.free(unsafe.Pointer(dinfo))
	dinfo.err = (*C.struct_jpeg_error_mgr)(C.malloc(C.size_t(unsafe.Sizeof(C.struct_jpeg_error_mgr{}))))
	if dinfo.err == nil {
		panic("Failed to allocate dinfo.err")
	}
	defer C.free(unsafe.Pointer(dinfo.err))

	img = new(YUVImage)

	// Setup error handling
	C.jpeg_std_error(dinfo.err)
	dinfo.err.error_exit = (*[0]byte)(C.error_panic)

	// Initialize decompression
	C.c_jpeg_create_decompress(dinfo)
	defer C.jpeg_destroy_decompress(dinfo)

	srcManager := makeSourceManager(src, dinfo)
	defer C.free(unsafe.Pointer(srcManager))

	C.jpeg_read_header(dinfo, C.TRUE)

	// Configure pre-scaling and request calculation of component info
	if params.TargetWidth > 0 && params.TargetHeight > 0 {
		var scaleFactor int
		for scaleFactor = 1; scaleFactor <= 8; scaleFactor++ {
			if ((scaleFactor*int(dinfo.image_width)+7)/8) >= params.TargetWidth &&
				((scaleFactor*int(dinfo.image_height)+7)/8) >= params.TargetHeight {
				break
			}
		}
		if scaleFactor < 8 {
			dinfo.scale_num = C.uint(scaleFactor)
			dinfo.scale_denom = 8
		}
	}

	// More settings
	if params.FastDCT {
		dinfo.dct_method = C.JDCT_IFAST
	} else {
		dinfo.dct_method = C.JDCT_ISLOW
	}
	C.jpeg_calc_output_dimensions(dinfo)

	// Figure out what color format we're dealing with after scaling
	compInfo := (*[3]C.jpeg_component_info)(unsafe.Pointer(dinfo.comp_info))
	colorVDiv := 1
	switch dinfo.num_components {
	case 1:
		if dinfo.jpeg_color_space != C.JCS_GRAYSCALE {
			panic("Unsupported colorspace")
		}
		img.Format = Grayscale
	case 3:
		// No support for RGB and CMYK (both rare)
		if dinfo.jpeg_color_space != C.JCS_YCbCr {
			panic("Unsupported colorspace")
		}
		dwY := compInfo[Y].downsampled_width
		dhY := compInfo[Y].downsampled_height
		dwC := compInfo[U].downsampled_width
		dhC := compInfo[U].downsampled_height
		//fmt.Printf("%d %d %d %d\n", dwY, dhY, dwC, dhC)
		if dwC != compInfo[V].downsampled_width || dhC != compInfo[V].downsampled_height {
			panic("Unsupported color subsampling (Cb and Cr differ)")
		}
		// Since the decisions about which DCT size and subsampling mode
		// to use, if any, are complex, instead just check the calculated
		// output plane sizes and infer the subsampling mode from that.
		if dwY == dwC {
			if dhY == dhC {
				img.Format = YUV444
			} else if (dhY+1)/2 == dhC {
				img.Format = YUV440
				colorVDiv = 2
			} else {
				panic("Unsupported color subsampling (vertical is not 1 or 2)")
			}
		} else if (dwY+1)/2 == dwC {
			if dhY == dhC {
				img.Format = YUV422
			} else if (dhY+1)/2 == dhC {
				img.Format = YUV420
				colorVDiv = 2
			} else {
				panic("Unsupported color subsampling (vertical is not 1 or 2)")
			}
		} else {
			panic("Unsupported color subsampling (horizontal is not 1 or 2)")
		}
	default:
		panic("Unsupported number of components")
	}

	img.Width = int(compInfo[Y].downsampled_width)
	img.Height = int(compInfo[Y].downsampled_height)
	//fmt.Printf("%dx%d (format: %d)\n", img.Width, img.Height, img.Format)
	//fmt.Printf("Output: %dx%d\n", dinfo.output_width, dinfo.output_height)

	// libjpeg raw data out is in planar format, which avoids unnecessary
	// planar->packed->planar conversions.
	dinfo.raw_data_out = C.TRUE

	// Allocate image planes
	for i := 0; i < int(dinfo.num_components); i++ {
		/*fmt.Printf("%d: %dx%d (DCT %dx%d, %dx%d blocks sf %dx%d)\n", i,
		  compInfo[i].downsampled_width, compInfo[i].downsampled_height,
		  compInfo[i].DCT_scaled_size, compInfo[i].DCT_scaled_size,
		  compInfo[i].width_in_blocks, compInfo[i].height_in_blocks,
		  compInfo[i].h_samp_factor, compInfo[i].v_samp_factor)*/
		// When downsampling, odd modes like 14x14 may be used. Pad to AlignSize
		// (16) and then add an extra AlignSize padding, to cover overflow from
		// any such modes.
		img.Stride[i] = pad(int(compInfo[i].downsampled_width), AlignSize) + AlignSize
		height := pad(int(compInfo[i].downsampled_height), AlignSize) + AlignSize
		img.Data[i] = make([]byte, img.Stride[i]*height)
	}

	// Start decompression
	C.jpeg_start_decompress(dinfo)

	// Allocate JSAMPIMAGE to hold pointers to one iMCU worth of image data
	// this is a safe overestimate; we use the return value from
	// jpeg_read_raw_data to figure out what is the actual iMCU row count.
	var yuvPtrInt [3][AlignSize]C.JSAMPROW
	yuvPtr := [3]C.JSAMPARRAY{
		C.JSAMPARRAY(unsafe.Pointer(&yuvPtrInt[0][0])),
		C.JSAMPARRAY(unsafe.Pointer(&yuvPtrInt[1][0])),
		C.JSAMPARRAY(unsafe.Pointer(&yuvPtrInt[2][0])),
	}

	// Decode the image.
	var row C.JDIMENSION

	var iMCURows int
	for i := 0; i < int(dinfo.num_components); i++ {
		compRows := int(C.DCT_v_scaled_size(dinfo, C.int(i)) * compInfo[i].v_samp_factor)
		if compRows > iMCURows {
			iMCURows = compRows
		}
	}
	//fmt.Printf("iMCU_rows: %d (div: %d)\n", iMCURows, colorVDiv)
	for row = 0; row < dinfo.output_height; {
		// First fill in the pointers into the plane data buffers
		for i := 0; i < int(dinfo.num_components); i++ {
			for j := 0; j < iMCURows; j++ {
				compRow := (int(row) + j)
				if i > 0 {
					compRow = (int(row)/colorVDiv + j)
				}
				yuvPtrInt[i][j] = C.JSAMPROW(unsafe.Pointer(&img.Data[i][img.Stride[i]*compRow]))
			}
		}
		// Get the data
		row += C.jpeg_read_raw_data(dinfo, C.JSAMPIMAGE(unsafe.Pointer(&yuvPtr[0])), C.JDIMENSION(2*iMCURows))
	}

	// Clean up
	C.jpeg_finish_decompress(dinfo)

	return
}
