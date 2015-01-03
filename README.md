## go-thumber

go-thumber is a dynamic JPEG thumbnailing proxy designed for speed. It
implements JPEG -> JPEG thumbnailing only.

Features:
* Input: JPEG (YCbCr 4:4:4, 4:4:0, 4:2:2, 4:2:0, and greyscale modes)
* Output: JPEG (YCbCr 4:4:4 or greyscale)
* No color conversion: data is kept in direct planar YCbCr buffers for efficiency and quality
* Optimized JPEG decoding: decodes only as much data as necessary for a particular resolution
* Uses libswscale for very fast but high quality scaling (lanczos)

Unsupported:
* RGB or CMYK modes. The input images are assumed to have been transcoded to a sane format.
* Color-subsampled output. The assumption is that a low-res thumbnail can benefit more from full chroma, so this has not been implemented for simplicity.
* Progressive decode/buffering. While the JPEG encoded data is streamed to/from
  the network, currently the entire raw YCbCr image is buffered before and after
  scaling. This could be changed to work in slices, saving memory.
* Other image formats
* Cropping


### Dependencies

* Go 1.3 (needed for http.Client.Timeout and certain cgo features)
* libswscale (from ffmpeg or libav)
* libjpeg (preferably libjpeg-turbo)


### Build

    $ sudo apt-get install libswscale-dev libjpeg-dev
    $ mkdir -p "${GOPATH}/src/github.com/pixiv"
    $ cd "${GOPATH}/src/github.com/pixiv"
    $ git clone <repo URL>
    $ go install github.com/pixiv/go-thumber/thumberd

On hardened setups which default to PIC builds, the following flag is required:

    $ go install -ldflags '-extldflags=-fno-PIC' github.com/pixiv/go-thumber/thumberd

And the versioning is possible on build-time.

    $ go install -ldflags '-X main.version v1.3' github.com/pixiv/go-thumber/thumberd

### Usage

thumberd is a FastCGI server. You can also run it as a standalone HTTP server
like this:

    $ thumberd -local localhost:8080

And then access a URL of the form:

    http://localhost:8080/w=128,h=128,a=0,q=95/upstream-host.com/some-image.jpg

Parameters:

    w: thumbnail width (required)
    h: thumbnail height (required)
    q: JPEG quality (default 90)
    u: upscale if the source is smaller (default 1)
    a: force thumbnail aspect ratio. If 0, keep aspect (default 1)
    o: optimize JPEG (default 0)
    p: Factor to use when loading downsampled JPEGs. See below for explanation (default 2)

While uncompressing the source JPEG, the JPEG format allows direct loading of a
downscaled version (by partially decoding only enough data from the JPEG to
reconstruct a lower-resolution version). This built-in downscaling is pretty
decent, but not as good as the main lanczos rescaling algorithm used in
go-thumber. The factor parameter allows control over how this feature is used.

Setting the factor to 0 disables this feature and always loads the
full-resolution JPEG and then downscales it to the target size. Setting the
factor to 1 picks the same or higher resolution as the requested target size.
1.5 multiplies the requested size by 1.5, and so on, such that 2 will always
load at least twice the resolution required and then downsample it. Values
between 0 and 1 make no sense, since they will load an image *smaller* than
requested (but you can try them if you want to see what happens). The JPEG
format and libjpeg only allow resolutions of 1/8 through 8/8 (in units of 1/8)
of the original resolution, so this defines a lower bound; the actual resolution
loaded from the JPEG will usually be rounded up.

In practice, 0 provides the highest quality but is not very efficient, 1 is
fastest but noticeably lower quality, and 2 provides practically the same
quality as 0 while still being faster when the image is being scaled down to
~40% or less of its original dimensions. For comparison, ImageMagick seems to
behave as if p=1.
