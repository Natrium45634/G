package ui

import (
	"image"
	"image/color"
	"math"
)

// WindowIcon рисует иконку окна (гексагон со стрелкой) программно.
func WindowIcon(size int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	c := float64(size) / 2
	r := c * 0.95
	inside := func(x, y float64) bool {
		dx, dy := math.Abs(x-c), math.Abs(y-c)
		// шестиугольник «остриём вверх»
		return dx <= r*math.Sqrt(3)/2 && dy <= r-dx/math.Sqrt(3)
	}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			if !inside(fx, fy) {
				continue
			}
			col := color.RGBA{70, 150, 255, 255}
			if !inside(c+(fx-c)*1.12, c+(fy-c)*1.12) {
				col = color.RGBA{255, 210, 90, 255}
			}
			// стрелка-направление взгляда
			ax, ay := fx-c, fy-c
			if ax > -r*0.35 && ax < r*0.55 && math.Abs(ay) < (r*0.55-ax)*0.6 && ax > -r*0.35 {
				if math.Abs(ay) < r*0.12 || ax > r*0.05 {
					col = color.RGBA{240, 240, 245, 255}
				}
			}
			img.Set(x, y, col)
		}
	}
	return img
}
