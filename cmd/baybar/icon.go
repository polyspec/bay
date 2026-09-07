package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"sync"
)

// Status icons are drawn rather than shipped as assets: they are two circles,
// and generating them keeps the binary self-contained. They are template
// images - black with an alpha channel - so macOS tints them for the menu bar
// theme and for the highlighted state.
const (
	iconPx     = 36 // 2x for an 18pt menu bar item
	iconRadius = 13.0
	iconStroke = 3.0
)

type fill int

const (
	fillNone fill = iota // nothing running
	fillHalf             // some services running
	fillFull             // everything running
)

var (
	iconOnce  sync.Once
	iconCache map[fill][]byte
)

func statusIcon(f fill) []byte {
	iconOnce.Do(func() {
		iconCache = map[fill][]byte{
			fillNone: renderIcon(fillNone),
			fillHalf: renderIcon(fillHalf),
			fillFull: renderIcon(fillFull),
		}
	})
	return iconCache[f]
}

func renderIcon(f fill) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, iconPx, iconPx))
	center := float64(iconPx-1) / 2

	for y := 0; y < iconPx; y++ {
		for x := 0; x < iconPx; x++ {
			dx, dy := float64(x)-center, float64(y)-center
			d := math.Hypot(dx, dy)

			// The ring is always present; coverage falls off over one pixel so
			// the edges are anti-aliased rather than stair-stepped.
			a := coverage(iconRadius-d) * coverage(d-(iconRadius-iconStroke))

			// Adding the coverages rather than taking the maximum keeps the
			// seam where the fill meets the ring from showing as a faint line:
			// both are 0.5 there, and they should sum to a solid pixel.
			switch f {
			case fillFull:
				a += coverage(iconRadius - iconStroke - d)
			case fillHalf:
				a += coverage(iconRadius*0.42 - d)
			}
			if a <= 0 {
				continue
			}
			img.SetNRGBA(x, y, color.NRGBA{A: uint8(math.Round(math.Min(a, 1) * 255))})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	return buf.Bytes()
}

// coverage maps a signed distance in pixels to an alpha in [0,1], giving one
// pixel of feathering at the edge.
func coverage(d float64) float64 {
	switch {
	case d <= -0.5:
		return 0
	case d >= 0.5:
		return 1
	default:
		return d + 0.5
	}
}
