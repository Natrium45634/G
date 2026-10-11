package ui

import (
	"fmt"
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"hextactics/engine/hex"
)

func toImageRect(r rect) image.Rectangle {
	return image.Rect(int(r.x), int(r.y), int(r.x+r.w), int(r.y+r.h))
}

// menu — главное меню: выбор сценария.
type menu struct {
	g       *Game
	sel     int
	err     string
	t       float64
	buttons []rect
}

func newMenu(g *Game) *menu { return &menu{g: g} }

func (m *menu) Update(dt float64) error {
	m.t += dt
	if m.g.quit {
		return ErrQuit
	}
	ids := m.g.Content.ScenarioOrder
	mx, my := ebiten.CursorPosition()
	W, H := float64(m.g.W), float64(m.g.H)
	m.buttons = m.buttons[:0]
	y := H/2 - 60
	for range ids {
		m.buttons = append(m.buttons, rect{W/2 - 300, y, 600, 74})
		y += 84
	}
	quit := rect{W/2 - 300, y + 10, 600, 40}
	for i, r := range m.buttons {
		if r.contains(float64(mx), float64(my)) {
			m.sel = i
		}
	}
	start := -1
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		for i, r := range m.buttons {
			if r.contains(float64(mx), float64(my)) {
				start = i
			}
		}
		if quit.contains(float64(mx), float64(my)) {
			return ErrQuit
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
		m.sel = (m.sel + 1) % len(ids)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
		m.sel = (m.sel - 1 + len(ids)) % len(ids)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		start = m.sel
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return ErrQuit
	}
	if start >= 0 && start < len(ids) {
		if err := m.g.StartBattle(ids[start]); err != nil {
			m.err = err.Error()
		}
	}
	return nil
}

func (m *menu) Draw(dst *ebiten.Image) {
	W, H := float64(m.g.W), float64(m.g.H)
	// декоративный фон из гексов
	for q := -14; q <= 30; q++ {
		for r := -2; r <= 22; r++ {
			x, y := hex.ToPixel(hex.Hex{Q: q, R: r}, 34)
			if x < -40 || x > W+40 || y < -40 || y > H+40 {
				continue
			}
			v := 0.5 + 0.5*math.Sin(float64(q)*0.4+float64(r)*0.3+m.t*0.6)
			fillPath(dst, hexPath(x, y, 33), rgba(30, uint8(34+v*14), uint8(44+v*18), 255))
		}
	}
	fillRect(dst, rect{W/2 - 340, 60, 680, H - 120}, rgba(18, 20, 26, 235))
	strokeRect(dst, rect{W/2 - 340, 60, 680, H - 120}, 1, colPanelEdge)
	drawText(dst, "HEX TACTICS", 44, true, W/2, 90, colAccent, alCenter)
	drawText(dst, "Тактика в реальном времени с паузой на гексагональной карте", 16, false, W/2, 150, colText, alCenter)
	drawText(dst, "Очереди приказов · направленный бой · умения · характеристики из данных", 13, false, W/2, 176, colTextDim, alCenter)
	drawText(dst, "Выберите сценарий:", 15, true, W/2-300, H/2-90, colText, alLeft)
	for i, id := range m.g.Content.ScenarioOrder {
		sc := m.g.Content.Scenarios[id]
		r := m.buttons[i]
		bg := colButton
		if i == m.sel {
			bg = colButtonHi
		}
		fillRect(dst, r, bg)
		strokeRect(dst, r, 1, colPanelEdge)
		if i == m.sel {
			strokeRect(dst, r, 2, colAccent)
		}
		drawText(dst, sc.Name, 18, true, r.x+16, r.y+8, colText, alLeft)
		lines := wrap(sc.Desc, 12, false, r.w-32)
		for j, l := range lines {
			if j >= 2 {
				break
			}
			drawText(dst, l, 12, false, r.x+16, r.y+34+float64(j)*15, colTextDim, alLeft)
		}
	}
	y := m.buttons[len(m.buttons)-1].y + 94
	q := rect{W/2 - 300, y, 600, 40}
	fillRect(dst, q, colButton)
	strokeRect(dst, q, 1, colPanelEdge)
	drawText(dst, "Выход", 16, false, W/2, y+10, colText, alCenter)
	drawText(dst, fmt.Sprintf("Данные: %s · Enter — начать, ↑↓ — выбор, F1 в бою — справка", m.g.DataSource), 12, false, W/2, H-90, colTextFaint, alCenter)
	if m.err != "" {
		drawText(dst, m.err, 13, false, W/2, H-110, colBad, alCenter)
	}
}
