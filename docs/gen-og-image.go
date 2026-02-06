//go:build ignore

// gen-og-image.go generates og-image.png for social media sharing.
// Run with: go run docs/gen-og-image.go
package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

const (
	w = 1200
	h = 630
)

func main() {
	img := image.NewRGBA(image.Rect(0, 0, w, h))

	// Background: radial-ish gradient from center (#141926) to edges (#080B10)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx := float64(x-w/2) / float64(w/2)
			dy := float64(y-h/2) / float64(h/2)
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist > 1.0 {
				dist = 1.0
			}
			// Interpolate between center color and edge color
			r := lerp(20, 8, dist)
			g := lerp(25, 11, dist)
			b := lerp(38, 16, dist)
			img.Set(x, y, color.RGBA{r, g, b, 255})
		}
	}

	accent := color.RGBA{249, 115, 22, 255}       // #F97316
	accentGlow := color.RGBA{249, 115, 22, 40}     // subtle glow
	accentSoft := color.RGBA{255, 138, 76, 255}    // #FF8A4C

	// Draw accent bar at top
	for y := 0; y < 5; y++ {
		for x := 0; x < w; x++ {
			t := float64(x) / float64(w)
			r := lerp(249, 255, t)
			g := lerp(115, 138, t)
			b := lerp(22, 76, t)
			img.Set(x, y, color.RGBA{r, g, b, 255})
		}
	}

	// Paw icon - larger, centered
	cx, cy := w/2, h/2-40

	drawPaw(img, cx, cy, 1.0, accent, accentGlow, accentSoft)

	// Horizontal separator line below paw
	lineY := cy + 100
	lineColor := color.RGBA{45, 55, 72, 255}
	for x := cx - 200; x < cx+200; x++ {
		// Fade edges
		dist := math.Abs(float64(x-cx)) / 200.0
		alpha := uint8(255 * (1.0 - dist*dist))
		c := color.RGBA{lineColor.R, lineColor.G, lineColor.B, alpha}
		blendPixel(img, x, lineY, c)
	}

	// Text placeholder blocks
	textColor := color.RGBA{230, 237, 243, 255}
	subtextColor := color.RGBA{156, 163, 176, 200}

	// "paw-proxy" title block
	drawRoundedRect(img, cx-150, cy+120, cx+150, cy+142, 4, textColor)

	// "Zero-config HTTPS for local development" tagline
	drawRoundedRect(img, cx-240, cy+158, cx+240, cy+173, 3, subtextColor)

	// Bottom accent line (thinner)
	for x := 0; x < w; x++ {
		t := float64(x) / float64(w)
		r := lerp(249, 255, t)
		g := lerp(115, 138, t)
		b := lerp(22, 76, t)
		img.Set(x, h-2, color.RGBA{r, g, b, 180})
		img.Set(x, h-1, color.RGBA{r, g, b, 100})
	}

	writeImage("docs/og-image.png", img)

	// Generate 180x180 apple-touch-icon
	icon := image.NewRGBA(image.Rect(0, 0, 180, 180))
	// Dark background matching the OG image
	for y := 0; y < 180; y++ {
		for x := 0; x < 180; x++ {
			icon.Set(x, y, color.RGBA{14, 18, 27, 255})
		}
	}
	iconAccent := color.RGBA{249, 115, 22, 255}
	iconGlow := color.RGBA{249, 115, 22, 30}
	iconSoft := color.RGBA{255, 138, 76, 255}
	drawPaw(icon, 90, 85, 0.55, iconAccent, iconGlow, iconSoft)
	writeImage("docs/apple-touch-icon.png", icon)
}

func writeImage(path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		panic(err)
	}
	if err := f.Close(); err != nil {
		panic(err)
	}
}

func drawPaw(img *image.RGBA, cx, cy int, scale float64, accent, glow, soft color.RGBA) {
	s := func(v int) int { return int(float64(v) * scale) }

	// Glow behind the paw
	drawFilledCircleAlpha(img, cx, cy-s(10), s(120), glow)

	// Paw toe pads (4 on top)
	drawFilledCircle(img, cx-s(60), cy-s(60), s(28), accent)
	drawFilledCircle(img, cx-s(20), cy-s(85), s(25), soft)
	drawFilledCircle(img, cx+s(20), cy-s(85), s(25), soft)
	drawFilledCircle(img, cx+s(60), cy-s(60), s(28), accent)

	// Main pad (large ellipse)
	drawFilledEllipse(img, cx, cy+s(15), s(52), s(42), accent)
}

func lerp(a, b uint8, t float64) uint8 {
	return uint8(float64(a)*(1.0-t) + float64(b)*t)
}

func blendPixel(img *image.RGBA, x, y int, c color.RGBA) {
	bounds := img.Bounds()
	if x < bounds.Min.X || x >= bounds.Max.X || y < bounds.Min.Y || y >= bounds.Max.Y {
		return
	}
	existing := img.RGBAAt(x, y)
	alpha := float64(c.A) / 255.0
	r := uint8(float64(c.R)*alpha + float64(existing.R)*(1-alpha))
	g := uint8(float64(c.G)*alpha + float64(existing.G)*(1-alpha))
	b := uint8(float64(c.B)*alpha + float64(existing.B)*(1-alpha))
	img.Set(x, y, color.RGBA{r, g, b, 255})
}

func drawFilledCircle(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	for y := cy - r - 1; y <= cy+r+1; y++ {
		for x := cx - r - 1; x <= cx+r+1; x++ {
			dx := float64(x - cx)
			dy := float64(y - cy)
			dist := math.Sqrt(dx*dx+dy*dy) - float64(r)
			if dist < 0 {
				blendPixel(img, x, y, c)
			} else if dist < 1.5 {
				// Anti-alias edge
				alpha := uint8(float64(c.A) * (1.0 - dist/1.5))
				blendPixel(img, x, y, color.RGBA{c.R, c.G, c.B, alpha})
			}
		}
	}
}

func drawFilledCircleAlpha(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			dx := float64(x - cx)
			dy := float64(y - cy)
			dist := math.Sqrt(dx*dx+dy*dy) / float64(r)
			if dist <= 1.0 {
				// Fade from center
				alpha := uint8(float64(c.A) * (1.0 - dist*dist))
				blendPixel(img, x, y, color.RGBA{c.R, c.G, c.B, alpha})
			}
		}
	}
}

func drawFilledEllipse(img *image.RGBA, cx, cy, rx, ry int, c color.RGBA) {
	for y := cy - ry - 1; y <= cy+ry+1; y++ {
		for x := cx - rx - 1; x <= cx+rx+1; x++ {
			dx := float64(x-cx) / float64(rx)
			dy := float64(y-cy) / float64(ry)
			dist := math.Sqrt(dx*dx + dy*dy)
			if dist < 1.0 {
				blendPixel(img, x, y, c)
			} else if dist < 1.05 {
				alpha := uint8(float64(c.A) * (1.0 - (dist-1.0)/0.05))
				blendPixel(img, x, y, color.RGBA{c.R, c.G, c.B, alpha})
			}
		}
	}
}

func drawRoundedRect(img *image.RGBA, x1, y1, x2, y2, radius int, c color.RGBA) {
	r := float64(radius)
	for y := y1; y < y2; y++ {
		for x := x1; x < x2; x++ {
			// Distance to nearest rounded corner
			dx := math.Max(float64(x1+radius-x), math.Max(float64(x-x2+radius+1), 0))
			dy := math.Max(float64(y1+radius-y), math.Max(float64(y-y2+radius+1), 0))
			if dx > 0 && dy > 0 {
				dist := math.Sqrt(dx*dx+dy*dy) - r
				if dist < 0 {
					blendPixel(img, x, y, c)
				} else if dist < 1.0 {
					alpha := uint8(float64(c.A) * (1.0 - dist))
					blendPixel(img, x, y, color.RGBA{c.R, c.G, c.B, alpha})
				}
			} else {
				blendPixel(img, x, y, c)
			}
		}
	}
}
