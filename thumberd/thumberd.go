package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/fcgi"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pixiv/go-thumber/thumbnail"
)

var local = flag.String("local", "", "serve as webserver, example: 0.0.0.0:8000")
var timeout = flag.Int("timeout", 3, "timeout for upstream HTTP requests, in seconds")
var show_version = flag.Bool("version", false, "show version and exit")

var client http.Client

var version string

const maxDimension = 65000
const maxPixels = 10000000

var http_stats struct {
	received int64
	inflight int64
	ok int64
	thumb_error int64
	upstream_error int64
	arg_error int64
	total_time_us int64
}

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

func statusServer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "version %s\n", version)
	fmt.Fprintf(w, "received %d\n", atomic.LoadInt64(&http_stats.received))
	fmt.Fprintf(w, "inflight %d\n", atomic.LoadInt64(&http_stats.inflight))
	fmt.Fprintf(w, "ok %d\n", atomic.LoadInt64(&http_stats.ok))
	fmt.Fprintf(w, "thumb_error %d\n", atomic.LoadInt64(&http_stats.thumb_error))
	fmt.Fprintf(w, "upstream_error %d\n", atomic.LoadInt64(&http_stats.upstream_error))
	fmt.Fprintf(w, "arg_error %d\n", atomic.LoadInt64(&http_stats.arg_error))
	fmt.Fprintf(w, "total_time_us %d\n", atomic.LoadInt64(&http_stats.total_time_us))
}

func thumbServer(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	defer func() {
		elapsed := int64(time.Now().Sub(startTime) / 1000)
		atomic.AddInt64(&http_stats.total_time_us, elapsed)
	}()

	atomic.AddInt64(&http_stats.received, 1)
	atomic.AddInt64(&http_stats.inflight, 1)
	defer atomic.AddInt64(&http_stats.inflight, -1)

	path := r.RequestURI

	// Defaults
	var params = thumbnail.ThumbnailParameters{
		Upscale:        true,
		ForceAspect:    true,
		Quality:        90,
		Optimize:       false,
		PrescaleFactor: 2.0,
	}

	if path[0] != '/' {
		http.Error(w, "Path should start with /", http.StatusBadRequest)
		atomic.AddInt64(&http_stats.arg_error, 1)
		return
	}
	parts := strings.SplitN(path[1:], "/", 2)
	if len(parts) < 2 {
		http.Error(w, "Path needs to have at least two components", http.StatusBadRequest)
		atomic.AddInt64(&http_stats.arg_error, 1)
		return
	}
	for _, arg := range strings.Split(parts[0], ",") {
		tup := strings.SplitN(arg, "=", 2)
		if len(tup) != 2 {
			http.Error(w, "Arguments must have the form name=value", http.StatusBadRequest)
			atomic.AddInt64(&http_stats.arg_error, 1)
			return
		}
		switch tup[0] {
		case "w", "h", "q", "u", "a", "o":
			val, err := strconv.Atoi(tup[1])
			if err != nil {
				http.Error(w, "Invalid integer value for "+tup[0], http.StatusBadRequest)
				atomic.AddInt64(&http_stats.arg_error, 1)
				return
			}
			switch tup[0] {
			case "w":
				params.Width = val
			case "h":
				params.Height = val
			case "q":
				params.Quality = val
			case "u":
				params.Upscale = val != 0
			case "a":
				params.ForceAspect = val != 0
			case "o":
				params.Optimize = val != 0
			}
		case "p":
			val, err := strconv.ParseFloat(tup[1], 64)
			if err != nil {
				http.Error(w, "Invalid float value for "+tup[0], http.StatusBadRequest)
				atomic.AddInt64(&http_stats.arg_error, 1)
				return
			}
			params.PrescaleFactor = val
		}
	}
	if params.Width <= 0 || params.Width > maxDimension {
		http.Error(w, "Width (w) not specified or invalid", http.StatusBadRequest)
		atomic.AddInt64(&http_stats.arg_error, 1)
		return
	}
	if params.Height <= 0 || params.Height > maxDimension {
		http.Error(w, "Height (h) not specified or invalid", http.StatusBadRequest)
		atomic.AddInt64(&http_stats.arg_error, 1)
		return
	}
	if params.Width*params.Height > maxPixels {
		http.Error(w, "Image dimensions are insane", http.StatusBadRequest)
		atomic.AddInt64(&http_stats.arg_error, 1)
		return
	}
	if params.Quality > 100 || params.Quality < 0 {
		http.Error(w, "Quality must be between 0 and 100", http.StatusBadRequest)
		atomic.AddInt64(&http_stats.arg_error, 1)
		return
	}

	srcReader, err := client.Get("http://" + parts[1])
	if err != nil {
		http.Error(w, "Upstream failed: "+err.Error(), http.StatusBadGateway)
		atomic.AddInt64(&http_stats.upstream_error, 1)
		return
	}
	if srcReader.StatusCode != http.StatusOK {
		http.Error(w, "Upstream failed: "+srcReader.Status, srcReader.StatusCode)
		atomic.AddInt64(&http_stats.upstream_error, 1)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	err = thumbnail.MakeThumbnail(srcReader.Body, w, params)
	if err != nil {
		switch err := err.(type) {
		case *url.Error:
			http.Error(w, "Upstream failed: "+err.Error(), http.StatusBadGateway)
			atomic.AddInt64(&http_stats.upstream_error, 1)
			return
		default:
			http.Error(w, "Thumbnailing failed: "+err.Error(), http.StatusInternalServerError)
			atomic.AddInt64(&http_stats.thumb_error, 1)
			return
		}
	}
	srcReader.Body.Close()
	atomic.AddInt64(&http_stats.ok, 1)
}

func main() {
	flag.Parse()
	if *show_version {
		fmt.Printf("thumberd %s\n", version)
		return
	}

	client.Timeout = time.Duration(*timeout) * time.Second

	var err error

	http.HandleFunc("/server-status", statusServer)

	http.HandleFunc("/", thumbServer)

	if *local != "" { // Run as a local web server
		err = http.ListenAndServe(*local, nil)
	} else { // Run as FCGI via standard I/O
		err = fcgi.Serve(nil, nil)
	}
	if err != nil {
		log.Fatal(err)
	}
}
