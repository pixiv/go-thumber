package main

import (
    "flag"
    "log"
    "net/http"
    "net/http/fcgi"
    "github.com/pixiv/go-thumber/thumbnail"
    "runtime"
    "time"
    "net/url"
    "strconv"
    "strings"
)

var local = flag.String("local", "", "serve as webserver, example: 0.0.0.0:8000")
var timeout = flag.Int("timeout", 3, "timeout for upstream HTTP requests, in seconds")

var client http.Client

const maxDimension = 65000
const maxPixels = 10000000

func init() {
    runtime.GOMAXPROCS(runtime.NumCPU())
}

func thumbServer(w http.ResponseWriter, r *http.Request) {
    path := r.RequestURI

    // Defaults
    var params = thumbnail.ThumbnailParameters{
        Upscale: true,
        ForceAspect: true,
        Quality: 90,
        Optimize: false,
        PrescaleFactor: 2.0,
    }

    if path[0] != '/' {
        http.Error(w, "Path should start with /", http.StatusBadRequest)
        return
    }
    parts := strings.SplitN(path[1:], "/", 2)
    if len(parts) < 2 {
        http.Error(w, "Path needs to have at least two components", http.StatusBadRequest)
        return
    }
    for _, arg := range strings.Split(parts[0], ",") {
        tup := strings.SplitN(arg, "=", 2)
        if len(tup) != 2 {
            http.Error(w, "Arguments must have the form name=value", http.StatusBadRequest)
            return
        }
        switch tup[0] {
            case "w", "h", "q", "u", "a", "o":
                val, err := strconv.Atoi(tup[1])
                if err != nil {
                    http.Error(w, "Invalid integer value for " + tup[0], http.StatusBadRequest)
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
                    http.Error(w, "Invalid float value for " + tup[0], http.StatusBadRequest)
                    return
                }
                params.PrescaleFactor = val
        }
    }
    if params.Width <= 0 || params.Width > maxDimension {
        http.Error(w, "Width (w) not specified or invalid", http.StatusBadRequest)
        return
    }
    if params.Height <= 0 || params.Height > maxDimension {
        http.Error(w, "Height (h) not specified or invalid", http.StatusBadRequest)
        return
    }
    if params.Width * params.Height > maxPixels {
        http.Error(w, "Image dimensions are insane", http.StatusBadRequest)
        return
    }
    if params.Quality > 100 || params.Quality < 0 {
        http.Error(w, "Quality must be between 0 and 100", http.StatusBadRequest)
        return
    }

    srcReader, err := client.Get("http://" + parts[1])
    if err != nil {
        http.Error(w, "Upstream failed: " + err.Error(), http.StatusBadGateway)
        return
    }
    if srcReader.StatusCode != http.StatusOK {
        http.Error(w, "Upstream failed: " + srcReader.Status, srcReader.StatusCode)
        return
    }

    w.Header().Set("Content-Type", "image/jpeg")
    err = thumbnail.MakeThumbnail(srcReader.Body, w, params)
    if err != nil {
        switch err := err.(type) {
            case *url.Error:
                http.Error(w, "Upstream failed: " + err.Error(), http.StatusBadGateway)
                return
            default:
                http.Error(w, "Thumbnailing failed: " + err.Error(), http.StatusInternalServerError)
                return
        }
    }
    srcReader.Body.Close()
}

func main() {
    flag.Parse()

    client.Timeout = time.Duration(*timeout) * time.Second

    var err error

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