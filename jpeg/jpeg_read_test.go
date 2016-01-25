package jpeg

import (
	"os"
	"testing"
)

type testImage struct {
	Path          string
	Width, Height int
}

var testImages = []testImage{
	testImage{
		"../test-image/test001.jpg",
		1000,
		750,
	},
}

func TestRead(t *testing.T) {
	for _, testImage := range testImages {
		src, err := os.Open("../test-image/test001.jpg")
		if err != nil {
			t.Errorf("open: %v", err)
			continue
		}
		img, err := ReadJPEG(src, DecompressionParameters{})
		if err != nil {
			t.Errorf("ReadJPEG: %v", err)
			continue
		}

		if testImage.Width != img.Width {
			t.Errorf("width: %d, expected: %d", img.Width, testImage.Width)
		}
		if testImage.Height != img.Height {
			t.Errorf("height: %d, expected: %d", img.Height, testImage.Height)
		}
	}
}
