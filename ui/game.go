package ui

import (
	"errors"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"hextactics/engine/data"
)

// ErrQuit — выход из игры.
var ErrQuit = errors.New("quit")

// scene — экран игры.
type scene interface {
	Update(dt float64) error
	Draw(dst *ebiten.Image)
}

// Game — реализация ebiten.Game.
type Game struct {
	Content    *data.Content
	DataSource string
	SavePath   string
	W, H       int
	cur        scene
	last       time.Time
	Screenshot func(img *ebiten.Image) // для отладочного режима
	Frame      int
	quit       bool
}

// NewGame создаёт игру с главным меню.
func NewGame(c *data.Content, source, savePath string) *Game {
	initFonts()
	g := &Game{Content: c, DataSource: source, SavePath: savePath, W: 1600, H: 900}
	g.cur = newMenu(g)
	return g
}

// StartBattle запускает сценарий.
func (g *Game) StartBattle(id string) error {
	b, err := newBattle(g, id, nil)
	if err != nil {
		return err
	}
	g.cur = b
	return nil
}

func (g *Game) toMenu() { g.cur = newMenu(g) }

// Update — шаг логики (вызывается Ebitengine 60 раз в секунду).
func (g *Game) Update() error {
	now := time.Now()
	dt := 1.0 / 60
	if !g.last.IsZero() {
		dt = now.Sub(g.last).Seconds()
		if dt > 0.25 {
			dt = 0.25 // после сворачивания окна не «догоняем» больше четверти секунды
		}
		if dt < 0 {
			dt = 0
		}
	}
	g.last = now
	if g.quit {
		return ErrQuit
	}
	return g.cur.Update(dt)
}

// Draw — отрисовка кадра.
func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(colBg)
	g.cur.Draw(screen)
	g.Frame++
	if g.Screenshot != nil {
		g.Screenshot(screen)
	}
}

// Layout — логический размер экрана равен размеру окна.
func (g *Game) Layout(outW, outH int) (int, int) {
	if outW < 1100 {
		outW = 1100
	}
	if outH < 680 {
		outH = 680
	}
	g.W, g.H = outW, outH
	return outW, outH
}
