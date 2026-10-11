package ui

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// Draw — кадр боя: карта → подсветки → планы → юниты → эффекты → панели → подсказки.
func (b *Battle) Draw(dst *ebiten.Image) {
	mapImg := dst.SubImage(toImageRect(b.view)).(*ebiten.Image)
	b.drawMap(mapImg)
	b.drawPreparing(mapImg)
	preview := b.drawPreview(mapImg)
	b.drawPlans(mapImg)
	b.drawUnits(mapImg)
	b.vfx.draw(mapImg, b)
	b.drawPartyCards(dst)

	b.drawTopBar(dst)
	for _, bt := range b.buttons {
		if bt.r.y < topH {
			b.drawButton(dst, bt)
		}
	}
	b.drawLog(dst)
	b.drawRightPanel(dst)
	b.drawToasts(dst)
	if b.showDiag {
		b.drawDiag(dst)
	}
	b.drawOverlays(dst)

	// Подсказки: кнопка → эффект → предпросмотр/клетка
	if b.overlayActive() {
		return
	}
	if b.hoverBtn != nil && b.hoverBtn.tip != "" {
		b.drawTooltipBox(dst, splitLines(b.hoverBtn.tip), nil)
		return
	}
	for _, e := range b.effectHits {
		if e.r.contains(b.mx, b.my) {
			b.drawTooltipBox(dst, wrap(e.tip, 13, false, 360), nil)
			return
		}
	}
	if b.passiveHit.contains(b.mx, b.my) {
		if u := b.selected(); u != nil {
			var lines []string
			for _, p := range u.Passives() {
				lines = append(lines, p.Name+": "+p.Desc)
			}
			b.drawTooltipBox(dst, lines, nil)
			return
		}
	}
	if b.hoverIn && b.lp == nil && b.rp == nil {
		lines, cols := b.cellTooltip()
		if len(preview) > 0 {
			lines = append(preview, append([]string{"—"}, lines...)...)
			c2 := make([]color.RGBA, len(preview)+1)
			for i := range c2 {
				c2[i] = colAccent
			}
			c2[len(c2)-1] = colTextFaint
			for i := 1; i < len(preview); i++ {
				c2[i] = colText
			}
			cols = append(c2, cols...)
		}
		b.drawTooltipBox(dst, lines, cols)
	}
}

func splitLines(s string) []string {
	var out []string
	for _, l := range splitN(s) {
		out = append(out, wrap(l, 13, false, 420)...)
	}
	return out
}

func splitN(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	return append(out, cur)
}

// drawDiag — диагностика (F3): частота кадров и тиков, состояние симуляции.
func (b *Battle) drawDiag(dst *ebiten.Image) {
	lines := []string{
		fmt.Sprintf("FPS %.0f · TPS %.0f", ebiten.ActualFPS(), ebiten.ActualTPS()),
		fmt.Sprintf("Тик %d · игровое время %.2f с · реальное %.1f с", b.w.Tick, b.w.Seconds(), b.clock.RealElapsed),
		fmt.Sprintf("Скорость %d× · последняя %d× · событий %d", b.clock.Speed, b.clock.LastSpeed, b.w.LastSeq()),
		fmt.Sprintf("Юнитов %d · объектов %d · версия карты %d · занятость %d", len(b.w.Units), len(b.w.Objects), b.w.Map.Version, b.w.OccVersion),
		fmt.Sprintf("ГСЧ: вызовов %d · хэш состояния %s", b.w.RNG.Calls, b.w.StateHash()),
	}
	x, y := b.view.x+8, b.view.y+b.view.h-float64(len(lines))*17-12
	fillRect(dst, rect{x - 4, y - 4, 520, float64(len(lines))*17 + 8}, rgba(0, 0, 0, 190))
	for i, l := range lines {
		drawText(dst, l, 12, false, x, y+float64(i)*17, colGood, alLeft)
	}
}
