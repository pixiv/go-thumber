package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestThumbServer(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(thumbServer))

	res, err := http.Get(ts.URL)
	if err != nil {
		t.Error("unexpected")
		return
	}

	if res.StatusCode != 400 {
		t.Error("StatusCode should be 400")
	}
}

func originImageHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "../test-image/test001.jpg")
}

func TestThumbServerWithSuccessCase(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(thumbServer))
	defer ts.Close()

	origin := httptest.NewServer(http.HandlerFunc(originImageHandler))

	defer origin.Close()

	originHost := fmt.Sprintf(strings.Replace(origin.URL, "http://", "", 1))
	res, err := http.Get(ts.URL + "/w=128,h=128,a=0,q=95/" + originHost + "/")
	if err != nil {
		t.Error("unexpected")
		return
	}
	if res.StatusCode != 200 {
		t.Error("Status code should be 200, but got ", res.StatusCode)
		return
	}
}

func BenchmarkThumbServer(b *testing.B) {
	ts := httptest.NewServer(http.HandlerFunc(thumbServer))
	defer ts.Close()

	origin := httptest.NewServer(http.HandlerFunc(originImageHandler))
	defer origin.Close()
	originHost := fmt.Sprintf(strings.Replace(origin.URL, "http://", "", 1))

	for i := 0; i < b.N; i++ {
		res, err := http.Get(ts.URL + "/w=128,h=128,a=0,q=95/" + originHost + "/")
		if err != nil {
			b.Error("unexpected", i)
			return
		}
		if res.StatusCode != 200 {
			b.Error("Status code should be 200, but got ", res.StatusCode, i)
			return
		}
	}
}

func TestThumbServerWithInvalidParam(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(thumbServer))
	defer ts.Close()

	res, err := http.Get(ts.URL + "/w=abc,h=100,q=0.9/")
	if err != nil {
		t.Error("unexpected")
		return
	}
	if res.StatusCode != 400 {
		t.Error("Status code should be 400")
		return
	}
}

func TestStatusServer(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(statusServer))
	defer ts.Close()

	res, err := http.Get(ts.URL)
	if err != nil {
		t.Error("unexpected")
		return
	}
	if res.StatusCode != 200 {
		t.Error("Status code should be 200")
		return
	}
}

func TestNotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(errorServer))
	defer ts.Close()

	res, err := http.Get(ts.URL)
	if err != nil {
		t.Error("unexpected")
		return
	}
	if res.StatusCode != 404 {
		t.Error("Status code should be 404")
		return
	}
}
