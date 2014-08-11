/* Here be dragons. */
package jpeg

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
    "math"
    "fmt"
    "io"
    "unsafe"
)

const read_buffer_size = 16384

type sourceManager struct {
    magic uint32
    pub C.struct_jpeg_source_mgr
    buffer [read_buffer_size]byte
    src io.Reader
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
func sourceInit (dinfo *C.struct_jpeg_decompress_struct) {
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
        mgr.pub.next_input_byte = (*C.JOCTET)(&mgr.buffer[mgr.currentSize - int(mgr.pub.bytes_in_buffer)])
    }
}

//export sourceTerm
func sourceTerm(dinfo *C.struct_jpeg_decompress_struct) {
    // do nothing
}

func makeSourceManager(src io.Reader, dinfo *C.struct_jpeg_decompress_struct) (ret sourceManager) {
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

type DecompressionParameters struct {
    TargetWidth int
    TargetHeight int
    FastDCT bool // note: do not use for Quality > 90
}

// Calculate the smallest scale factor (in eigths) that will, after rounding
// down during downscaling, meet or exceed the specified target size.
func calcScale(target float64, size float64) int {
    fmt.Printf("%f\n", (target * 8) * (1 - 1 / (size)) / size)
    return int(math.Ceil((target * 8) * (1 - 1 / (size)) / size))
}

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

    var dinfo C.struct_jpeg_decompress_struct
    var jerr C.struct_jpeg_error_mgr

    img = new(YUVImage)

    // Setup error handling
    C.jpeg_std_error(&jerr)
    jerr.error_exit = (*[0]byte)(C.error_panic)
    dinfo.err = &jerr

    // Initialize decompression
    C.c_jpeg_create_decompress(&dinfo)
    makeSourceManager(src, &dinfo)
    C.jpeg_read_header(&dinfo, C.TRUE)

    // Configure pre-scaling and request calculation of component info
    if params.TargetWidth > 0 && params.TargetHeight > 0 {
        var scaleFactor int
        for scaleFactor = 1; scaleFactor <= 8; scaleFactor++ {
            if ((scaleFactor * int(dinfo.image_width) + 7) / 8) >= params.TargetWidth &&
               ((scaleFactor * int(dinfo.image_height) + 7) / 8) >= params.TargetHeight {
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
    C.jpeg_calc_output_dimensions(&dinfo)

    // Figure out what color format we're dealing with after scaling
    comp_info := (*[3]C.jpeg_component_info)(unsafe.Pointer(dinfo.comp_info))
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
            dw_y := comp_info[Y].downsampled_width
            dh_y := comp_info[Y].downsampled_height
            dw_c := comp_info[U].downsampled_width
            dh_c := comp_info[U].downsampled_height
            //fmt.Printf("%d %d %d %d\n", dw_y, dh_y, dw_c, dh_c)
            if dw_c != comp_info[V].downsampled_width || dh_c != comp_info[V].downsampled_height {
                panic("Unsupported color subsampling (Cb and Cr differ)")
            }
            // Since the decisions about which DCT size and subsampling mode
            // to use, if any, are complex, instead just check the calculated
            // output plane sizes and infer the subsampling mode from that.
            if dw_y == dw_c {
                if dh_y == dh_c {
                    img.Format = YUV444
                } else if (dh_y + 1) / 2 == dh_c {
                    img.Format = YUV440
                    colorVDiv = 2
                } else {
                    panic("Unsupported color subsampling (vertical is not 1 or 2)")
                }
            } else if (dw_y + 1) / 2 == dw_c {
                if dh_y == dh_c {
                    img.Format = YUV422
                } else if (dh_y + 1) / 2 == dh_c {
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

    img.Width = int(comp_info[Y].downsampled_width)
    img.Height = int(comp_info[Y].downsampled_height)
    //fmt.Printf("%dx%d (format: %d)\n", img.Width, img.Height, img.Format)
    //fmt.Printf("Output: %dx%d\n", dinfo.output_width, dinfo.output_height)

    // libjpeg raw data out is in planar format, which avoids unnecessary
    // planar->packed->planar conversions.
    dinfo.raw_data_out = C.TRUE

    // Allocate image planes
    for i := 0; i < int(dinfo.num_components); i++ {
        /*fmt.Printf("%d: %dx%d (DCT %dx%d, %dx%d blocks sf %dx%d)\n", i,
                   comp_info[i].downsampled_width, comp_info[i].downsampled_height,
                   comp_info[i].DCT_scaled_size, comp_info[i].DCT_scaled_size,
                   comp_info[i].width_in_blocks, comp_info[i].height_in_blocks,
                   comp_info[i].h_samp_factor, comp_info[i].v_samp_factor)*/
        // When downsampling, odd modes like 14x14 may be used. Pad to AlignSize
        // (16) and then add an extra AlignSize padding, to cover overflow from
        // any such modes.
        img.Stride[i] = pad(int(comp_info[i].downsampled_width), AlignSize) + AlignSize
        height := pad(int(comp_info[i].downsampled_height), AlignSize) + AlignSize
        img.Data[i] = make([]byte, img.Stride[i] * height)
    }

    // Start decompression
    C.jpeg_start_decompress(&dinfo)

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
        compRows := int(C.DCT_v_scaled_size(&dinfo, C.int(i)) * comp_info[i].v_samp_factor)
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
                    compRow = (int(row) / colorVDiv + j)
                }
                yuvPtrInt[i][j] = C.JSAMPROW(unsafe.Pointer(&img.Data[i][img.Stride[i] * compRow]))
            }
        }
        // Get the data
        row += C.jpeg_read_raw_data(&dinfo, C.JSAMPIMAGE(unsafe.Pointer(&yuvPtr[0])), C.JDIMENSION(2*iMCURows))
    }

    // Clean up
    C.jpeg_finish_decompress(&dinfo)
    C.jpeg_destroy_decompress(&dinfo)

    return
}
