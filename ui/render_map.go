package ui

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"hextactics/engine/gamemap"
	"hextactics/engine/hex"
)

// hash32 — детерминированный «шум» для вариаций текстуры клеток.
func hash32(q, r, k int) uint32 {
	h := uint32(q*73856093) ^ uint32(r*19349663) ^ uint32(k*83492791)
	h ^= h >> 13
	h *= 0x5bd1e995
	h ^= h >> 15
	return h
}

func hrand(q, r, k int) float64 { return float64(hash32(q, r, k)%1000) / 1000 }

// ensureTerrain перерисовывает слой местности при изменении масштаба или карты.
func (b *Battle) ensureTerrain() {
	key := [2]uint64{uint64(b.zoom * 1000), b.w.Map.Version}
	if b.terrain != nil && key == b.terrainKey {
		return
	}
	minX, minY, maxX, maxY := b.mapBounds()
	s := b.size()
	pad := s * 1.2
	wpx := int(maxX-minX+2*pad) + 2
	hpx := int(maxY-minY+2*pad) + 2
	if b.terrain != nil {
		b.terrain.Deallocate()
	}
	img := ebiten.NewImage(wpx, hpx)
	ox, oy := minX-pad, minY-pad
	for _, c := range b.w.Map.Cells {
		x, y := hex.ToPixel(c.Hex, s)
		b.drawCell(img, c, x-ox, y-oy, s)
	}
	// Перепады высот: тёмная кромка со стороны более низкой клетки
	for _, c := range b.w.Map.Cells {
		x, y := hex.ToPixel(c.Hex, s)
		x, y = x-ox, y-oy
		for d := 0; d < 6; d++ {
			n := b.w.Map.Cell(c.Hex.Neighbor(hex.Dir(d)))
			if n == nil || n.Height >= c.Height || c.Terrain.BlocksLOS {
				continue
			}
			i0, i1 := edgeCorners(d)
			x0, y0 := hexCorner(x, y, s, i0)
			x1, y1 := hexCorner(x, y, s, i1)
			w := 2.0 + float64(c.Height-n.Height)*1.5
			line(img, x0, y0, x1, y1, w*b.zoom, rgba(30, 24, 16, 210))
		}
	}
	b.terrain, b.terrainKey, b.terrainOX, b.terrainOY = img, key, ox, oy
}

func (b *Battle) drawCell(dst *ebiten.Image, c *gamemap.Cell, x, y, s float64) {
	t := c.Terrain
	base := t.Color.Color()
	v := 0.94 + hrand(c.Hex.Q, c.Hex.R, 1)*0.12
	base = shade(base, v*(1+0.13*float64(c.Height)))
	fillPath(dst, hexPath(x, y, s), base)
	z := b.zoom
	q, r := c.Hex.Q, c.Hex.R
	switch t.Pattern {
	case "grass":
		for k := 0; k < 4; k++ {
			px := x + (hrand(q, r, 10+k)-0.5)*s*1.2
			py := y + (hrand(q, r, 20+k)-0.5)*s*1.1
			line(dst, px, py, px+1.5*z, py-4*z, 1.2*z, shade(base, 1.25))
		}
	case "road":
		fillPath(dst, hexPath(x, y, s*0.6), shade(base, 1.08))
		for k := 0; k < 3; k++ {
			circle(dst, x+(hrand(q, r, 30+k)-0.5)*s, y+(hrand(q, r, 40+k)-0.5)*s, 1.5*z, shade(base, 0.8))
		}
	case "trees":
		for k := 0; k < 3; k++ {
			px := x + (hrand(q, r, 50+k)-0.5)*s*0.9
			py := y + (hrand(q, r, 60+k)-0.5)*s*0.8
			rr := s * (0.26 + hrand(q, r, 70+k)*0.1)
			circle(dst, px+2*z, py+3*z, rr, rgba(10, 30, 10, 120))
			circle(dst, px, py, rr, shade(base, 0.75))
			circle(dst, px-rr*0.3, py-rr*0.3, rr*0.5, shade(base, 1.05))
		}
	case "bush":
		for k := 0; k < 4; k++ {
			px := x + (hrand(q, r, 80+k)-0.5)*s
			py := y + (hrand(q, r, 90+k)-0.5)*s
			circle(dst, px, py, s*0.16, shade(base, 0.7))
		}
	case "swamp":
		for k := 0; k < 5; k++ {
			px := x + (hrand(q, r, 100+k)-0.5)*s*1.2
			py := y + (hrand(q, r, 110+k)-0.5)*s
			circle(dst, px, py, 2*z, rgba(70, 110, 120, 255))
		}
		line(dst, x-s*0.3, y+s*0.2, x-s*0.3, y-s*0.2, 1.5*z, shade(base, 1.4))
	case "water", "ford":
		for k := -1; k <= 1; k++ {
			yy := y + float64(k)*s*0.4
			for i := 0; i < 2; i++ {
				xx := x - s*0.4 + float64(i)*s*0.45
				line(dst, xx, yy, xx+s*0.12, yy-s*0.08, 1.4*z, shade(base, 1.35))
				line(dst, xx+s*0.12, yy-s*0.08, xx+s*0.24, yy, 1.4*z, shade(base, 1.35))
			}
		}
		if t.Pattern == "ford" {
			for k := 0; k < 3; k++ {
				circle(dst, x+(hrand(q, r, 120+k)-0.5)*s, y+(hrand(q, r, 130+k)-0.5)*s, 2.5*z, rgb(150, 140, 120))
			}
		}
	case "bridge":
		for k := -2; k <= 2; k++ {
			xx := x + float64(k)*s*0.3
			line(dst, xx, y-s*0.75, xx, y+s*0.75, 2*z, shade(base, 0.7))
		}
	case "rock":
		fillPath(dst, hexPath(x, y-s*0.1, s*0.7), shade(base, 1.2))
		fillPath(dst, hexPath(x+s*0.15, y+s*0.15, s*0.4), shade(base, 0.85))
	case "wall":
		fillPath(dst, hexPath(x, y, s*0.92), shade(base, 1.1))
		for k := -1; k <= 1; k++ {
			yy := y + float64(k)*s*0.38
			line(dst, x-s*0.75, yy, x+s*0.75, yy, 1.2*z, shade(base, 0.7))
		}
	case "floor":
		strokePath(dst, hexPath(x, y, s*0.55), 1*z, shade(base, 0.85))
	case "rubble":
		for k := 0; k < 4; k++ {
			px := x + (hrand(q, r, 140+k)-0.5)*s
			py := y + (hrand(q, r, 150+k)-0.5)*s
			fillRect(dst, rect{px, py, 5 * z, 4 * z}, shade(base, 0.7))
		}
	case "sand":
		for k := 0; k < 6; k++ {
			circle(dst, x+(hrand(q, r, 160+k)-0.5)*s*1.2, y+(hrand(q, r, 170+k)-0.5)*s, 1*z, shade(base, 0.8))
		}
	}
	strokePath(dst, hexPath(x, y, s), 1, rgba(0, 0, 0, 55))
}

// drawMap — местность, элементы карты, затемнение и подсветки.
func (b *Battle) drawMap(dst *ebiten.Image) {
	b.ensureTerrain()
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(b.terrainOX-b.camX+b.view.x, b.terrainOY-b.camY+b.view.y)
	dst.DrawImage(b.terrain, op)
	s := b.size()

	// Элементы: двери и рычаги
	for _, c := range b.w.Map.FeatureCells() {
		x, y := b.hexToScreen(c.Hex)
		f := c.Feature
		switch f.Kind {
		case "door":
			if f.Open {
				strokeRect(dst, rect{x - s*0.55, y - s*0.15, s * 1.1, s * 0.3}, 2, rgb(150, 110, 60))
				drawTextShadow(dst, "дверь", 10*b.zoom, false, x, y+s*0.25, colTextDim, alCenter)
			} else {
				fillRect(dst, rect{x - s*0.7, y - s*0.22, s * 1.4, s * 0.44}, rgb(120, 82, 40))
				strokeRect(dst, rect{x - s*0.7, y - s*0.22, s * 1.4, s * 0.44}, 2, rgb(60, 40, 20))
				circle(dst, x+s*0.45, y, 2.5*b.zoom, rgb(220, 200, 120))
			}
		case "lever":
			ang := -0.9
			if f.Open {
				ang = 0.9
			}
			fillRect(dst, rect{x - s*0.3, y + s*0.15, s * 0.6, s * 0.18}, rgb(80, 70, 60))
			tx, ty := x+math.Sin(ang)*s*0.5, y+s*0.15-math.Cos(ang)*s*0.5
			line(dst, x, y+s*0.2, tx, ty, 3*b.zoom, rgb(70, 60, 50))
			kc := rgb(220, 70, 60)
			if f.Open {
				kc = rgb(90, 220, 90)
			}
			circle(dst, tx, ty, 4*b.zoom, kc)
		}
	}

	// Затемнение клеток вне обзора отряда
	vis := b.visibleCells()
	fog := rgba(0, 0, 10, 105)
	for _, c := range b.w.Map.Cells {
		if vis[c.Hex] {
			continue
		}
		x, y := b.hexToScreen(c.Hex)
		if x < b.view.x-s || x > b.view.x+b.view.w+s || y < b.view.y-s || y > b.view.y+b.view.h+s {
			continue
		}
		fillPath(dst, hexPath(x, y, s+0.5), fog)
	}

	// Объекты (ловушки, метки отложенных ударов)
	for _, o := range b.w.Objects {
		if !b.w.ObjectVisibleTo(o, b.player) {
			continue
		}
		d := b.w.ObjectDef(o)
		if d == nil {
			continue
		}
		x, y := b.hexToScreen(o.Hex)
		c := d.Color.Color()
		if d.Trigger == "timer" {
			left := b.w.C.Seconds(o.Trigger - b.w.Tick)
			pulse := 0.5 + 0.5*math.Sin(b.time*8)
			for _, h := range hex.Range(o.Hex, d.Radius) {
				hx, hy := b.hexToScreen(h)
				fillPath(dst, hexPath(hx, hy, s*0.95), withAlpha(c, uint8(50+60*pulse)))
				strokePath(dst, hexPath(hx, hy, s*0.95), 2, withAlpha(c, 200))
			}
			drawTextShadow(dst, formatSec(left), 14*b.zoom, true, x, y-8*b.zoom, colAccent, alCenter)
		} else {
			ring(dst, x, y, s*0.45, 2, c)
			drawTextShadow(dst, d.Glyph, 11*b.zoom, true, x, y-6*b.zoom, c, alCenter)
		}
	}
}

func formatSec(s float64) string {
	if s < 0 {
		s = 0
	}
	return fmt.Sprintf("%.1fс", s)
}

// hexOverlay — заливка клетки полупрозрачным цветом.
func (b *Battle) hexOverlay(dst *ebiten.Image, h hex.Hex, c color.RGBA, scale float64) {
	x, y := b.hexToScreen(h)
	fillPath(dst, hexPath(x, y, b.size()*scale), c)
}

func (b *Battle) hexOutline(dst *ebiten.Image, h hex.Hex, c color.RGBA, w, scale float64) {
	x, y := b.hexToScreen(h)
	strokePath(dst, hexPath(x, y, b.size()*scale), w, c)
}
