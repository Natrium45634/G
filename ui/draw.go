// Package ui — настольный интерфейс на Ebitengine. Интерфейс только читает
// состояние мира и передаёт намерения игрока в симуляцию через sim.Command.
package ui

import (
	"bytes"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

// ---------------------------------------------------------------------------
// Шрифты

var (
	srcRegular *text.GoTextFaceSource
	srcBold    *text.GoTextFaceSource
	faceCache  = map[[2]int]*text.GoTextFace{}
)

func initFonts() {
	var err error
	srcRegular, err = text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	if err != nil {
		panic(err)
	}
	srcBold, err = text.NewGoTextFaceSource(bytes.NewReader(gobold.TTF))
	if err != nil {
		panic(err)
	}
}

// face возвращает шрифт размера size (bold=1 — жирный).
func face(size float64, bold bool) *text.GoTextFace {
	key := [2]int{int(size * 10), 0}
	if bold {
		key[1] = 1
	}
	if f, ok := faceCache[key]; ok {
		return f
	}
	src := srcRegular
	if bold {
		src = srcBold
	}
	f := &text.GoTextFace{Source: src, Size: size}
	faceCache[key] = f
	return f
}

// ---------------------------------------------------------------------------
// Цвета

var (
	colBg        = rgb(22, 24, 30)
	colPanel     = rgba(30, 33, 42, 245)
	colPanel2    = rgba(40, 44, 56, 255)
	colPanelEdge = rgb(70, 76, 96)
	colText      = rgb(225, 228, 235)
	colTextDim   = rgb(150, 156, 170)
	colTextFaint = rgb(105, 110, 125)
	colAccent    = rgb(255, 210, 90)
	colGood      = rgb(110, 215, 120)
	colBad       = rgb(240, 90, 80)
	colWarn      = rgb(250, 190, 70)
	colAP        = rgb(90, 160, 255)
	colAPRes     = rgb(255, 210, 90)
	colHP        = rgb(90, 200, 100)
	colHPLow     = rgb(230, 80, 60)
	colSelect    = rgb(255, 230, 110)
	colPath      = rgb(255, 255, 255)
	colButton    = rgb(52, 58, 74)
	colButtonHi  = rgb(72, 80, 104)
	colButtonOn  = rgb(110, 92, 40)
	colButtonDis = rgb(38, 41, 50)
)

func rgb(r, g, b uint8) color.RGBA { return color.RGBA{r, g, b, 255} }

// rgba — цвет с прозрачностью (каналы премультиплицируются, как требует color.RGBA).
func rgba(r, g, b, a uint8) color.RGBA {
	f := float64(a) / 255
	return color.RGBA{uint8(float64(r) * f), uint8(float64(g) * f), uint8(float64(b) * f), a}
}

func withAlpha(c color.RGBA, a uint8) color.RGBA {
	// premultiplied: масштабируем каналы
	f := float64(a) / 255
	return color.RGBA{uint8(float64(c.R) * f), uint8(float64(c.G) * f), uint8(float64(c.B) * f), a}
}

func shade(c color.RGBA, k float64) color.RGBA {
	cl := func(v float64) uint8 {
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return uint8(v)
	}
	return color.RGBA{cl(float64(c.R) * k), cl(float64(c.G) * k), cl(float64(c.B) * k), c.A}
}

func mix(a, b color.RGBA, t float64) color.RGBA {
	l := func(x, y uint8) uint8 { return uint8(float64(x)*(1-t) + float64(y)*t) }
	return color.RGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), l(a.A, b.A)}
}

// ---------------------------------------------------------------------------
// Примитивы

type rect struct{ x, y, w, h float64 }

func (r rect) contains(x, y float64) bool {
	return x >= r.x && y >= r.y && x < r.x+r.w && y < r.y+r.h
}

func fillRect(dst *ebiten.Image, r rect, c color.Color) {
	vector.FillRect(dst, float32(r.x), float32(r.y), float32(r.w), float32(r.h), c, false)
}

func strokeRect(dst *ebiten.Image, r rect, w float64, c color.Color) {
	vector.StrokeRect(dst, float32(r.x), float32(r.y), float32(r.w), float32(r.h), float32(w), c, false)
}

func line(dst *ebiten.Image, x0, y0, x1, y1, w float64, c color.Color) {
	vector.StrokeLine(dst, float32(x0), float32(y0), float32(x1), float32(y1), float32(w), c, true)
}

func circle(dst *ebiten.Image, x, y, r float64, c color.Color) {
	vector.FillCircle(dst, float32(x), float32(y), float32(r), c, true)
}

func ring(dst *ebiten.Image, x, y, r, w float64, c color.Color) {
	vector.StrokeCircle(dst, float32(x), float32(y), float32(r), float32(w), c, true)
}

func fillPath(dst *ebiten.Image, p *vector.Path, c color.Color) {
	op := &vector.DrawPathOptions{AntiAlias: true}
	op.ColorScale.ScaleWithColor(c)
	vector.FillPath(dst, p, nil, op)
}

func strokePath(dst *ebiten.Image, p *vector.Path, w float64, c color.Color) {
	op := &vector.DrawPathOptions{AntiAlias: true}
	op.ColorScale.ScaleWithColor(c)
	vector.StrokePath(dst, p, &vector.StrokeOptions{Width: float32(w), LineJoin: vector.LineJoinRound, LineCap: vector.LineCapRound}, op)
}

// hexPath — шестиугольник с центром (x, y) и радиусом r.
func hexPath(x, y, r float64) *vector.Path {
	var p vector.Path
	for i := 0; i < 6; i++ {
		a := math.Pi / 180 * (60*float64(i) - 30)
		px, py := x+r*math.Cos(a), y+r*math.Sin(a)
		if i == 0 {
			p.MoveTo(float32(px), float32(py))
		} else {
			p.LineTo(float32(px), float32(py))
		}
	}
	p.Close()
	return &p
}

func hexCorner(x, y, r float64, i int) (float64, float64) {
	a := math.Pi / 180 * (60*float64(i) - 30)
	return x + r*math.Cos(a), y + r*math.Sin(a)
}

// edgeCorners — вершины ребра шестиугольника, обращённого в направлении d (0=восток, против часовой на экране).
func edgeCorners(d int) (int, int) {
	// Направление d смотрит на угол экрана -60·d градусов; ребро между вершинами.
	// Вершина i имеет угол 60i-30 (по часовой на экране, y вниз).
	// Восток (d=0): вершины 0 (−30°) и 1 (30°) → ребро справа.
	switch d {
	case 0:
		return 0, 1
	case 1:
		return 5, 0
	case 2:
		return 4, 5
	case 3:
		return 3, 4
	case 4:
		return 2, 3
	default:
		return 1, 2
	}
}

// arrow — стрелка от (x0,y0) к (x1,y1).
func arrow(dst *ebiten.Image, x0, y0, x1, y1, w float64, head float64, c color.Color) {
	line(dst, x0, y0, x1, y1, w, c)
	ang := math.Atan2(y1-y0, x1-x0)
	var p vector.Path
	p.MoveTo(float32(x1), float32(y1))
	p.LineTo(float32(x1-head*math.Cos(ang-0.45)), float32(y1-head*math.Sin(ang-0.45)))
	p.LineTo(float32(x1-head*math.Cos(ang+0.45)), float32(y1-head*math.Sin(ang+0.45)))
	p.Close()
	fillPath(dst, &p, c)
}

// dirVec — экранный единичный вектор направления d (0=В, против часовой на экране).
func dirVec(d float64) (float64, float64) {
	a := d * math.Pi / 3
	return math.Cos(a), -math.Sin(a)
}

// ---------------------------------------------------------------------------
// Текст

type align int

const (
	alLeft align = iota
	alCenter
	alRight
)

// drawText рисует строку; (x, y) — верхний край строки.
func drawText(dst *ebiten.Image, s string, size float64, bold bool, x, y float64, c color.Color, al align) float64 {
	f := face(size, bold)
	op := &text.DrawOptions{}
	switch al {
	case alCenter:
		op.PrimaryAlign = text.AlignCenter
	case alRight:
		op.PrimaryAlign = text.AlignEnd
	}
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(c)
	text.Draw(dst, s, f, op)
	w, _ := text.Measure(s, f, 0)
	return w
}

// drawTextShadow — текст с тенью (для подписей на карте).
func drawTextShadow(dst *ebiten.Image, s string, size float64, bold bool, x, y float64, c color.Color, al align) {
	drawText(dst, s, size, bold, x+1, y+1, color.RGBA{0, 0, 0, 200}, al)
	drawText(dst, s, size, bold, x, y, c, al)
}

func measure(s string, size float64, bold bool) float64 {
	w, _ := text.Measure(s, face(size, bold), 0)
	return w
}

// wrap разбивает текст на строки шириной не более width.
func wrap(s string, size float64, bold bool, width float64) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		cur := ""
		for _, wd := range words {
			try := wd
			if cur != "" {
				try = cur + " " + wd
			}
			if measure(try, size, bold) > width && cur != "" {
				out = append(out, cur)
				cur = wd
			} else {
				cur = try
			}
		}
		out = append(out, cur)
	}
	return out
}

// bar — полоса прогресса.
func bar(dst *ebiten.Image, r rect, frac float64, fg, bg color.Color) {
	fillRect(dst, r, bg)
	if frac > 1 {
		frac = 1
	}
	if frac > 0 {
		fillRect(dst, rect{r.x, r.y, r.w * frac, r.h}, fg)
	}
}

// panel — фон панели с рамкой.
func panel(dst *ebiten.Image, r rect) {
	fillRect(dst, r, colPanel)
	strokeRect(dst, r, 1, colPanelEdge)
}
