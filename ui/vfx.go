package ui

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"hextactics/engine/hex"
	"hextactics/engine/sim"
)

// Визуальные эффекты — отдельный слой: только читают события симуляции,
// ничего в ней не меняют. Время эффектов — реальное (кадры).

type floatText struct {
	h      hex.Hex
	text   string
	col    color.RGBA
	t, dur float64
	big    bool
	dx     float64
}

type streak struct {
	from, to hex.Hex
	col      color.RGBA
	t, dur   float64
}

type flash struct {
	area   []hex.Hex
	col    color.RGBA
	t, dur float64
}

type vfxSystem struct {
	texts   []floatText
	streaks []streak
	flashes []flash
}

func newVFX() *vfxSystem { return &vfxSystem{} }

func (v *vfxSystem) update(dt float64) {
	keepT := v.texts[:0]
	for _, f := range v.texts {
		f.t += dt
		if f.t < f.dur {
			keepT = append(keepT, f)
		}
	}
	v.texts = keepT
	keepS := v.streaks[:0]
	for _, s := range v.streaks {
		s.t += dt
		if s.t < s.dur {
			keepS = append(keepS, s)
		}
	}
	v.streaks = keepS
	keepF := v.flashes[:0]
	for _, f := range v.flashes {
		f.t += dt
		if f.t < f.dur {
			keepF = append(keepF, f)
		}
	}
	v.flashes = keepF
}

func (v *vfxSystem) text(h hex.Hex, s string, c color.RGBA, big bool) {
	// смещение, чтобы одновременные надписи не накладывались
	n := 0
	for _, f := range v.texts {
		if f.h == h && f.t < 0.3 {
			n++
		}
	}
	v.texts = append(v.texts, floatText{h: h, text: s, col: c, dur: 1.4, big: big, dx: float64(n) * 14})
}

func (v *vfxSystem) onEvent(b *Battle, e *sim.Event) {
	unitHex := func(id int) (hex.Hex, bool) {
		if u := b.w.Unit(id); u != nil {
			return u.Pos, true
		}
		return hex.Hex{}, false
	}
	switch e.Type {
	case sim.EvAttack, sim.EvMissed, sim.EvEvaded:
		if e.Kind == "ranged" || e.Kind == "spell" {
			col := rgb(255, 240, 180)
			if e.Kind == "spell" {
				col = rgb(200, 140, 255)
			}
			v.streaks = append(v.streaks, streak{from: e.From, to: e.Hex, col: col, dur: 0.3})
		}
		switch e.Type {
		case sim.EvMissed:
			v.text(e.Hex, "промах", rgb(190, 200, 220), false)
		case sim.EvEvaded:
			v.text(e.Hex, "уклонение", rgb(170, 220, 255), false)
		}
	case sim.EvBlocked:
		v.text(e.Hex, "блок", rgb(150, 190, 255), false)
	case sim.EvDamage:
		if e.Amount > 0 {
			s := fmt.Sprintf("−%d", e.Amount)
			c := rgb(255, 90, 70)
			if e.Crit {
				s += "!"
				c = rgb(255, 220, 60)
			}
			if h, ok := unitHex(e.Unit); ok {
				v.text(h, s, c, e.Crit || e.Amount >= 20)
			}
		}
	case sim.EvHeal:
		if e.Amount > 0 {
			if h, ok := unitHex(e.Unit); ok {
				v.text(h, fmt.Sprintf("+%d", e.Amount), rgb(110, 230, 120), false)
			}
		}
	case sim.EvEffectApplied:
		if e.Text != "" {
			if d := b.w.C.Effects[e.Effect]; d != nil {
				if h, ok := unitHex(e.Unit); ok {
					v.text(h, d.Name, d.Color.Color(), false)
				}
			}
		}
	case sim.EvEffectResisted:
		v.text(e.Hex, "устоял", rgb(220, 220, 220), false)
	case sim.EvUnitDied:
		v.text(e.Hex, "погиб", rgb(255, 60, 60), true)
	case sim.EvSkillResolved:
		if len(e.Area) > 0 {
			v.flashes = append(v.flashes, flash{area: e.Area, col: rgb(255, 170, 70), dur: 0.45})
		}
	case sim.EvObjectTriggered:
		v.flashes = append(v.flashes, flash{area: e.Area, col: rgb(255, 80, 40), dur: 0.7})
	case sim.EvActionInterrupted:
		v.text(e.Hex, "прервано", rgb(255, 200, 90), false)
	case sim.EvEnemySpotted:
		v.text(e.Hex, "!", rgb(255, 80, 80), true)
	case sim.EvMapChanged:
		v.flashes = append(v.flashes, flash{area: []hex.Hex{e.Hex}, col: rgb(255, 230, 140), dur: 0.5})
	}
}

func (v *vfxSystem) draw(dst *ebiten.Image, b *Battle) {
	for _, f := range v.flashes {
		a := 1 - f.t/f.dur
		for _, h := range f.area {
			b.hexOverlay(dst, h, withAlpha(f.col, uint8(160*a)), 0.95)
		}
	}
	for _, s := range v.streaks {
		x0, y0 := b.hexToScreen(s.from)
		x1, y1 := b.hexToScreen(s.to)
		t := s.t / s.dur
		hx, hy := x0+(x1-x0)*t, y0+(y1-y0)*t
		tx, ty := x0+(x1-x0)*math.Max(0, t-0.35), y0+(y1-y0)*math.Max(0, t-0.35)
		line(dst, tx, ty, hx, hy, 3*b.zoom, s.col)
		circle(dst, hx, hy, 3*b.zoom, s.col)
	}
	for _, f := range v.texts {
		x, y := b.hexToScreen(f.h)
		p := f.t / f.dur
		a := uint8(255 * math.Min(1, (1-p)*2))
		size := 15.0
		if f.big {
			size = 20
		}
		size *= math.Max(0.8, b.zoom)
		drawText(dst, f.text, size, true, x+1+f.dx, y-b.size()*0.9-p*30+1, color.RGBA{0, 0, 0, a}, alCenter)
		drawText(dst, f.text, size, true, x+f.dx, y-b.size()*0.9-p*30, withAlpha(f.col, a), alCenter)
	}
}
