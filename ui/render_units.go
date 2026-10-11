package ui

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"hextactics/engine/data"
	"hextactics/engine/hex"
	"hextactics/engine/sim"
)

// drawUnits — юниты (враги — только видимые отряду).
func (b *Battle) drawUnits(dst *ebiten.Image) {
	s := b.size()
	// Сначала погибшие (блёкло), затем живые
	for _, u := range b.w.Units {
		if !u.Dead || !b.w.IsVisibleTo(b.player, u) && u.Side != b.player {
			continue
		}
		x, y := b.hexToScreen(u.Pos)
		circle(dst, x, y, s*0.42, rgba(40, 40, 40, 140))
		line(dst, x-s*0.2, y-s*0.2, x+s*0.2, y+s*0.2, 2, rgba(120, 120, 120, 180))
		line(dst, x+s*0.2, y-s*0.2, x-s*0.2, y+s*0.2, 2, rgba(120, 120, 120, 180))
	}
	for _, u := range b.w.Units {
		if u.Dead || !b.w.IsVisibleTo(b.player, u) {
			continue
		}
		b.drawUnit(dst, u)
	}
}

func (b *Battle) drawUnit(dst *ebiten.Image, u *sim.Unit) {
	s := b.size()
	z := b.zoom
	x, y := b.unitScreenPos(u)
	side := b.w.SideOf(u.Side)
	sc := side.Color.Color()
	sel := u.ID == b.sel
	ang := b.unitFacingAngle(u)

	// Фронтальный сектор (60°) — подсветка на клетке
	fx, fy := dirVec(ang)
	{
		var p vector.Path
		p.MoveTo(float32(x), float32(y))
		a0 := ang*math.Pi/3 - math.Pi/6
		a1 := ang*math.Pi/3 + math.Pi/6
		rr := s * 0.98
		for i := 0; i <= 8; i++ {
			a := a0 + (a1-a0)*float64(i)/8
			p.LineTo(float32(x+rr*math.Cos(a)), float32(y-rr*math.Sin(a)))
		}
		p.Close()
		fillPath(dst, &p, withAlpha(sc, 70))
	}
	// тень и корпус
	r := s * 0.52
	circle(dst, x+2*z, y+3*z, r, rgba(0, 0, 0, 90))
	body := sc
	if inc, _ := b.w.Incapacitated(u); inc {
		body = shade(sc, 0.6)
	}
	circle(dst, x, y, r, body)
	ring(dst, x, y, r, 2*z, shade(sc, 0.45))
	if sel {
		pulse := 0.5 + 0.5*math.Sin(b.time*5)
		ring(dst, x, y, r+4*z, (2+pulse)*z, colSelect)
	}
	if u.ClassDef().Important {
		ring(dst, x, y, r-3*z, 1*z, rgba(255, 230, 140, 200))
	}
	// направление взгляда — треугольник на краю
	{
		tip := r + 7*z
		bx, by := x+fx*(r-2*z), y+fy*(r-2*z)
		px, py := -fy, fx
		var p vector.Path
		p.MoveTo(float32(x+fx*tip), float32(y+fy*tip))
		p.LineTo(float32(bx+px*6*z), float32(by+py*6*z))
		p.LineTo(float32(bx-px*6*z), float32(by-py*6*z))
		p.Close()
		fillPath(dst, &p, colText)
		strokePath(dst, &p, 1, rgba(0, 0, 0, 160))
	}
	drawText(dst, u.ClassDef().Glyph, 15*z, true, x, y-9*z, rgb(20, 20, 26), alCenter)

	// Полосы HP и ОД
	bw := s * 1.1
	hpf := float64(u.HP) / float64(max(1, u.HPMax()))
	bar(dst, rect{x - bw/2, y - r - 10*z, bw, 4 * z}, hpf, hpColor(hpf), rgba(40, 10, 10, 220))
	if u.Side == b.player {
		bar(dst, rect{x - bw/2, y + r + 4*z, bw, 3 * z}, float64(u.AP)/float64(max(1, u.APMax())), colAP, rgba(10, 20, 40, 220))
	}
	// Подготовка/каналирование — кольцо прогресса
	if a := u.Action; a != nil && (a.Kind == sim.ActPrep || a.Kind == sim.ActChannel) {
		t := b.actionProgress(a)
		c := rgb(255, 160, 60)
		if a.Kind == sim.ActChannel {
			c = rgb(120, 200, 255)
		}
		var p vector.Path
		p.Arc(float32(x), float32(y), float32(r+8*z), float32(-math.Pi/2), float32(-math.Pi/2+2*math.Pi*t), vector.Clockwise)
		strokePath(dst, &p, 3*z, c)
	}
	// Значки эффектов
	ex := x + r*0.75
	ey := y - r - 2*z
	for i, e := range u.Effects {
		if i >= 4 {
			break
		}
		d := b.w.C.Effects[e.Def]
		if d == nil || d.Duration > 0 && d.Duration <= 1 && d.Aura == nil {
			// краткие эффекты аур показываем точкой
			if d != nil {
				circle(dst, ex+3*z, ey+3*z, 3*z, d.Color.Color())
				ex += 7 * z
			}
			continue
		}
		g := d.Glyph
		w := measure(g, 9*z, true) + 4*z
		fillRect(dst, rect{ex, ey, w, 12 * z}, withAlpha(shade(d.Color.Color(), 0.4), 230))
		drawText(dst, g, 9*z, true, ex+2*z, ey, d.Color.Color(), alLeft)
		ex += w + 2*z
	}
	// Имя при наведении/выборе
	if sel || b.hover == u.Pos {
		drawTextShadow(dst, u.Name, 11*z, false, x, y+r+8*z, colText, alCenter)
	}
	if st := b.w.State(u); st == sim.StateStunned || st == sim.StateAsleep {
		lbl := "оглушён"
		if st == sim.StateAsleep {
			lbl = "спит"
		}
		drawTextShadow(dst, lbl, 11*z, true, x, y-r-24*z, colWarn, alCenter)
	}
}

// drawPreparing — область умений, которые сейчас готовятся (видимые юниты).
func (b *Battle) drawPreparing(dst *ebiten.Image) {
	for _, u := range b.w.Units {
		a := u.Action
		if u.Dead || a == nil || a.Kind != sim.ActPrep || !b.w.IsVisibleTo(b.player, u) {
			continue
		}
		sk := b.w.C.Skills[a.Skill]
		if sk == nil || sk.Shape.Type == "self" {
			continue
		}
		area := b.w.SkillArea(sk, u, u.Pos, a.Target)
		c := rgb(255, 150, 60)
		if u.Side != b.player {
			c = rgb(255, 60, 50)
		}
		t := b.actionProgress(a)
		for _, h := range area {
			b.hexOverlay(dst, h, withAlpha(c, uint8(40+80*t)), 0.92)
			b.hexOutline(dst, h, withAlpha(c, 220), 2, 0.92)
		}
	}
}

// ---------------------------------------------------------------------------
// Планы

func (b *Battle) drawPlans(dst *ebiten.Image) {
	for _, u := range b.w.AliveUnits(b.player) {
		if u.ID != b.sel {
			b.drawPlan(dst, u, false)
		}
	}
	if u := b.selected(); u != nil {
		b.drawPlan(dst, u, true)
	}
}

func (b *Battle) drawPlan(dst *ebiten.Image, u *sim.Unit, selected bool) {
	plan := b.w.ProjectPlan(u)
	if len(plan.Steps) == 0 {
		return
	}
	s := b.size()
	z := b.zoom
	sc := b.w.SideOf(u.Side).Color.Color()
	lineCol := withAlpha(mix(sc, colPath, 0.6), 220)
	if !selected {
		lineCol = withAlpha(sc, 110)
	}
	// Линии маршрута
	ux, uy := b.unitScreenPos(u)
	px, py := ux, uy
	for _, st := range plan.Steps {
		col := lineCol
		if st.Error != nil {
			col = withAlpha(colBad, 200)
		}
		for i, h := range st.Path {
			x, y := b.hexToScreen(h)
			if i == len(st.Path)-1 {
				arrow(dst, px, py, x, y, 3*z, 9*z, col)
			} else {
				line(dst, px, py, x, y, 3*z, col)
			}
			px, py = x, y
		}
		if st.HasTarget && (st.Type == sim.OrderAttack || st.Type == sim.OrderSkill || st.Type == sim.OrderInteract) && selected {
			tx, ty := b.hexToScreen(st.Target)
			ax, ay := b.hexToScreen(st.To)
			dashed(dst, ax, ay, tx, ty, 1.5*z, withAlpha(orderColor(st.Type), 200))
		}
	}
	if !selected {
		return
	}
	// Области умений
	for _, st := range plan.Steps {
		if st.Type == sim.OrderSkill || st.Type == sim.OrderAttack {
			for _, h := range st.Area {
				b.hexOverlay(dst, h, rgba(255, 150, 40, 55), 0.9)
				b.hexOutline(dst, h, rgba(255, 170, 60, 170), 1.5, 0.9)
			}
		}
	}
	// Маркеры: номер на точке, действия без перемещения — отдельными значками
	stack := map[hex.Hex]int{}
	cursor := b.cursorIdx(u)
	if u.Current != nil {
		cursor++
	}
	for i, st := range plan.Steps {
		num := b.orderNumber(u, st.OrderID)
		x, y := b.hexToScreen(st.To)
		k := stack[st.To]
		stack[st.To]++
		col := orderColor(st.Type)
		if st.Type == sim.OrderMove && len(st.Path) > 0 {
			circle(dst, x, y, 10*z, rgba(15, 15, 20, 230))
			ring(dst, x, y, 10*z, 2*z, col)
			drawText(dst, fmt.Sprint(num), 11*z, true, x, y-7*z, colText, alCenter)
		} else {
			// маркер действия смещается вокруг точки, чтобы не перекрывать номер
			ang := -math.Pi/4 + float64(k)*0.9
			mx, my := x+math.Cos(ang)*s*0.62, y-math.Sin(ang)*s*0.62
			circle(dst, mx, my, 9*z, col)
			lbl := orderGlyph(st.Type)
			if st.Type == sim.OrderSkill {
				if sk := b.w.C.Skills[st.Skill]; sk != nil && sk.Icon != "" {
					lbl = sk.Icon
				}
			}
			drawText(dst, lbl, 9*z, true, mx, my-6*z, colBg, alCenter)
			drawTextShadow(dst, fmt.Sprint(num), 9*z, true, mx+10*z, my-14*z, colText, alLeft)
			if st.Type == sim.OrderTurn {
				dx, dy := dirVec(float64(st.FacingTo))
				arrow(dst, x, y, x+dx*s*0.8, y+dy*s*0.8, 2.5*z, 8*z, col)
			}
		}
		if st.Error != nil || len(nonEmptyIssues(st.Issues)) > 0 {
			c := colWarn
			if st.Error != nil {
				c = colBad
			}
			circle(dst, x+9*z, y+9*z, 6*z, c)
			drawText(dst, "!", 10*z, true, x+9*z, y+2*z, colBg, alCenter)
		}
		if i == cursor-1 && cursor < len(plan.Steps) {
			ring(dst, x, y, s*0.8, 2*z, colAccent)
		}
	}
	// Конечная позиция и ориентация (призрак)
	last := plan.Steps[len(plan.Steps)-1]
	ex, ey := b.hexToScreen(last.To)
	if last.To != u.Pos {
		circle(dst, ex, ey, s*0.35, withAlpha(sc, 90))
	}
	dx, dy := dirVec(float64(last.FacingTo))
	arrow(dst, ex, ey, ex+dx*s*0.75, ey+dy*s*0.75, 2*z, 7*z, withAlpha(colText, 200))
}

func nonEmptyIssues(is []sim.Issue) []sim.Issue {
	var out []sim.Issue
	for _, i := range is {
		if i.Code != "" {
			out = append(out, i)
		}
	}
	return out
}

func dashed(dst *ebiten.Image, x0, y0, x1, y1, w float64, c color.Color) {
	l := math.Hypot(x1-x0, y1-y0)
	if l < 1 {
		return
	}
	n := int(l / 8)
	for i := 0; i < n; i += 2 {
		t0 := float64(i) / float64(n)
		t1 := float64(i+1) / float64(n)
		line(dst, x0+(x1-x0)*t0, y0+(y1-y0)*t0, x0+(x1-x0)*t1, y0+(y1-y0)*t1, w, c)
	}
}

// ---------------------------------------------------------------------------
// Предпросмотр под курсором и подсказки

func (b *Battle) drawPreview(dst *ebiten.Image) []string {
	u := b.selected()
	if u == nil || !b.hoverIn || b.lp != nil && b.lp.orderID != 0 && b.lp.moved {
		if u != nil && b.lp != nil && b.lp.orderID != 0 && b.lp.moved && b.hoverIn {
			b.hexOutline(dst, b.hover, colAccent, 3, 0.9)
			return []string{"Перенести точку маршрута сюда"}
		}
		return nil
	}
	s := b.size()
	z := b.zoom
	pos, facing, plan := b.anchor(u)
	free := u.AP - plan.Total
	if b.cursorIdx(u) < len(u.Queue) {
		// при вставке в середину считаем ОД до точки вставки
		n := b.cursorIdx(u)
		if u.Current != nil {
			n++
		}
		if n > 0 && n <= len(plan.Steps) {
			free = u.AP - plan.Steps[n-1].Cum
		}
	}
	ax, ay := b.hexToScreen(pos)
	ring(dst, ax, ay, s*0.6, 2*z, withAlpha(colAccent, 180))

	switch b.mode {
	case ModeMove:
		if t := b.visibleUnitAt(b.hover); t != nil && b.w.Hostile(b.player, t.Side) {
			if sk := b.w.C.Skills[u.ClassDef().BasicAttack]; sk != nil {
				return b.attackPreview(dst, u, t, sk, pos)
			}
		}
		// Подсветка доступности
		field := b.reachField(u, pos, facing)
		for h, r := range field {
			if h == pos {
				continue
			}
			c := rgba(80, 220, 120, 38)
			if r.Cost > free {
				c = rgba(240, 190, 60, 30)
			}
			b.hexOverlay(dst, h, c, 0.94)
		}
		return b.movePreview(dst, u, pos, facing, free)
	case ModeAttack:
		if t := b.visibleUnitAt(b.hover); t != nil && b.w.Hostile(b.player, t.Side) {
			if sk := b.w.C.Skills[u.ClassDef().BasicAttack]; sk != nil {
				return b.attackPreview(dst, u, t, sk, pos)
			}
		}
		return []string{"Наведите на противника"}
	case ModeTurn:
		if d, ok := b.mouseDir(pos); ok {
			dx, dy := dirVec(float64(d))
			arrow(dst, ax, ay, ax+dx*s*1.4, ay+dy*s*1.4, 4*z, 12*z, colAccent)
			cost := b.w.TurnCost(u, facing, d)
			return []string{"Поворот → " + d.Name(),
				fmt.Sprintf("Стоимость: %.2f ОД (%d × 60°)", data.APf(cost), hex.Delta(facing, d)),
				"ЛКМ — подтвердить"}
		}
	case ModeSkill:
		return b.skillPreview(dst, u, pos, facing)
	case ModeInteract:
		c := b.w.Map.Cell(b.hover)
		if c != nil && c.Feature != nil {
			b.hexOutline(dst, b.hover, colGood, 3, 0.9)
			return []string{"Взаимодействие: " + featureName(c.Feature.Kind, c.Feature.Open),
				fmt.Sprintf("Стоимость: %.1f ОД (+ подход)", b.w.C.Rules.InteractCostAP)}
		}
		b.hexOutline(dst, b.hover, colBad, 2, 0.9)
		return []string{"Здесь нет двери или рычага"}
	}
	return nil
}

func featureName(kind string, open bool) string {
	if kind == "lever" {
		return "рычаг"
	}
	if open {
		return "дверь (открыта — закрыть)"
	}
	return "дверь (закрыта — открыть)"
}

func (b *Battle) movePreview(dst *ebiten.Image, u *sim.Unit, from hex.Hex, facing hex.Dir, free int) []string {
	keep := b.keepFacing || shift()
	h := b.hover
	if h == from {
		return []string{"Точка вставки приказов"}
	}
	if t := b.visibleUnitAt(h); t != nil && t.Side == b.player {
		return []string{t.Name + " — ЛКМ: выбрать"}
	}
	if c := b.w.Map.Cell(h); c != nil && c.Feature != nil && (c.Feature.Kind == "lever" || c.IsClosedDoor()) {
		b.hexOutline(dst, h, colGood, 3, 0.9)
		return []string{"ЛКМ: " + featureName(c.Feature.Kind, c.Feature.Open)}
	}
	prof := b.w.MoveProfile(u, keep, facing)
	res := b.w.Map.FindPath(from, h, prof, func(x hex.Hex) bool { return b.w.Blocked(u, x) })
	if !res.OK {
		b.hexOutline(dst, h, colBad, 3, 0.9)
		x, y := b.hexToScreen(h)
		line(dst, x-8, y-8, x+8, y+8, 3, colBad)
		line(dst, x+8, y-8, x-8, y+8, 3, colBad)
		why := sim.Message(sim.Code(res.Code))
		if res.Code == "NO_PATH" {
			why = "Нет пути (препятствия или занятые клетки)"
		}
		return []string{"Недоступно: " + why}
	}
	// линия пути
	s := b.size()
	z := b.zoom
	px, py := b.hexToScreen(from)
	for i, ph := range res.Path {
		x, y := b.hexToScreen(ph)
		if i == len(res.Path)-1 {
			arrow(dst, px, py, x, y, 3*z, 10*z, colAccent)
		} else {
			dashed(dst, px, py, x, y, 3*z, colAccent)
		}
		px, py = x, y
	}
	endF := facing
	if !keep && len(res.Path) > 0 {
		prev := from
		if len(res.Path) > 1 {
			prev = res.Path[len(res.Path)-2]
		}
		endF, _ = hex.DirectionTo(prev, h)
	}
	ex, ey := b.hexToScreen(h)
	circle(dst, ex, ey, s*0.33, withAlpha(colAccent, 90))
	dx, dy := dirVec(float64(endF))
	arrow(dst, ex, ey, ex+dx*s*0.8, ey+dy*s*0.8, 2.5*z, 8*z, colAccent)
	cost := res.Cost
	if !keep && !b.w.C.Rules.TurnFreeWhileMoving {
		cost += 0
	}
	left := free - cost
	lines := []string{
		fmt.Sprintf("Путь: %d кл. · %.2f ОД", len(res.Path), data.APf(cost)),
	}
	if left >= 0 {
		lines = append(lines, fmt.Sprintf("Останется: %.2f ОД", data.APf(left)))
	} else {
		regen := u.Stat("ap_regen")
		wait := 0.0
		if regen > 0 {
			wait = data.APf(-left) / regen
		}
		lines = append(lines, fmt.Sprintf("Не хватает %.2f ОД — ожидание ~%.1f с", data.APf(-left), wait))
	}
	lines = append(lines, "Конечная ориентация: "+endF.Name())
	if keep {
		lines = append(lines, "Без поворота: шаги не вперёд дороже")
	}
	if occ := b.pathWarnings(u, res.Path); occ != "" {
		lines = append(lines, "! "+occ)
	}
	return lines
}

// pathWarnings — предупреждения о маршруте (ловушки, заметные противники рядом).
func (b *Battle) pathWarnings(u *sim.Unit, path []hex.Hex) string {
	for _, h := range path {
		for _, e := range b.w.VisibleEnemies(b.player) {
			if hex.Distance(e.Pos, h) <= 1 {
				return "маршрут проходит рядом с противником (" + e.Name + ")"
			}
		}
	}
	return ""
}

func (b *Battle) attackPreview(dst *ebiten.Image, u, t *sim.Unit, sk *data.SkillDef, from hex.Hex) []string {
	b.hexOutline(dst, t.Pos, colBad, 3, 0.95)
	// Если цель вне дальности, прогноз считается из клетки, куда юнит подойдёт.
	approach := ""
	if is := b.w.RangeIssue(from, t.Pos, sk); is != nil && !sk.NoApproach {
		prof := b.w.MoveProfile(u, false, u.Facing)
		goal := func(h hex.Hex) bool {
			if h != u.Pos {
				if ok, _ := b.w.Map.CanStand(h, prof); !ok || b.w.Blocked(u, h) {
					return false
				}
			}
			return b.w.RangeIssue(h, t.Pos, sk) == nil
		}
		res := b.w.Map.FindPathToAny(from, goal, prof, func(h hex.Hex) bool { return b.w.Blocked(u, h) }, -1)
		if res.OK && len(res.Path) > 0 {
			px, py := b.hexToScreen(from)
			for _, h := range res.Path {
				x, y := b.hexToScreen(h)
				dashed(dst, px, py, x, y, 2.5*b.zoom, colAccent)
				px, py = x, y
			}
			from = res.Path[len(res.Path)-1]
			ring(dst, px, py, b.size()*0.4, 2*b.zoom, colAccent)
			approach = fmt.Sprintf("Подход: %d кл., %.1f ОД — прогноз из конечной клетки", len(res.Path), data.APf(res.Cost))
		} else {
			approach = "! " + is.Text + " — подойти не удаётся"
		}
	}
	p := b.w.AttackPreview(u, t, sk, from)
	lines := []string{fmt.Sprintf("%s — %s", t.Name, t.ClassDef().Name)}
	lines = append(lines, fmt.Sprintf("HP %d/%d · броня %.0f · уклонение %.0f%% · блок %.0f%%", t.HP, t.HPMax(), t.Stat("armor"), t.Stat("evasion"), t.Stat("block")))
	d := hex.Distance(from, t.Pos)
	rng := fmt.Sprintf("дистанция %d (дальность %d–%d)", d, sk.RangeMin, sk.Range)
	lines = append(lines, "«"+sk.Name+"»: "+rng)
	if approach != "" {
		lines = append(lines, approach)
	}
	lines = append(lines, fmt.Sprintf("Сектор цели: %s · неожиданность %d%%", p.Sector.Name(), p.SurpriseChance))
	lines = append(lines, fmt.Sprintf("Попадание %d%% · уклонение %d%% · блок %d%%", p.HitChance, p.EvadeChance, p.BlockChance))
	lines = append(lines, fmt.Sprintf("Шанс нанести урон ≈ %d%%", p.LandChance))
	if res := b.knownResists(t); res != "" {
		lines = append(lines, "Известные сопротивления: "+res)
	} else {
		lines = append(lines, "Сопротивления неизвестны (раскрываются после удара)")
	}
	var effs []string
	for _, e := range t.Effects {
		if d := b.w.C.Effects[e.Def]; d != nil {
			effs = append(effs, d.Name)
		}
	}
	if len(effs) > 0 {
		lines = append(lines, "Эффекты: "+strings.Join(effs, ", "))
	}
	lines = append(lines, "ЛКМ — дойти и атаковать")
	return lines
}

func (b *Battle) knownResists(t *sim.Unit) string {
	var parts []string
	for _, dt := range []string{"physical", "fire", "poison", "arcane"} {
		d := b.w.C.DamageTypes[dt]
		if d == nil {
			continue
		}
		if b.w.KnownStat(b.player, t, d.ResistStat) {
			if v := t.Stat(d.ResistStat); v != 0 {
				parts = append(parts, fmt.Sprintf("%s %.0f%%", d.Name, v))
			} else {
				parts = append(parts, d.Name+" 0%")
			}
		}
		if b.w.KnownStat(b.player, t, d.VulnStat) {
			if v := t.Stat(d.VulnStat); v != 0 {
				parts = append(parts, fmt.Sprintf("уязв. %s +%.0f%%", d.Name, v))
			}
		}
	}
	return strings.Join(parts, ", ")
}

func (b *Battle) skillPreview(dst *ebiten.Image, u *sim.Unit, pos hex.Hex, facing hex.Dir) []string {
	sk := b.w.C.Skills[b.skill]
	if sk == nil {
		return nil
	}
	s := b.size()
	z := b.zoom
	// Зона дальности
	if sk.Range > 0 && sk.Targeting != "direction" {
		for _, h := range hex.Range(pos, sk.Range) {
			if hex.Distance(pos, h) < sk.RangeMin || !b.w.Map.InBounds(h) {
				continue
			}
			b.hexOverlay(dst, h, rgba(120, 170, 255, 22), 0.94)
		}
	}
	// Подходящие цели
	if sk.Targeting == "unit" {
		for _, t := range b.w.AliveUnits(-1) {
			if !b.w.IsVisibleTo(b.player, t) {
				continue
			}
			ok := (sk.TargetFilter == "enemy" && b.w.Hostile(u.Side, t.Side)) ||
				(sk.TargetFilter == "ally" && t.Side == u.Side && t.ID != u.ID) ||
				(sk.TargetFilter == "ally_or_self" && t.Side == u.Side) || sk.TargetFilter == "any"
			if ok {
				b.hexOutline(dst, t.Pos, colGood, 2, 0.98)
			}
		}
	}
	var tgt sim.Target
	var lines []string
	head := fmt.Sprintf("«%s» — %.1f ОД", sk.Name, sk.APCost)
	for r, v := range sk.Costs {
		if rd := b.w.C.Resources[r]; rd != nil {
			head += fmt.Sprintf(", %s %.0f", rd.Name, v)
		}
	}
	lines = append(lines, head)
	switch sk.Targeting {
	case "direction":
		d, ok := b.mouseDir(pos)
		if !ok {
			return append(lines, "Наведите мышь на сторону")
		}
		tgt = sim.Target{Hex: pos.Neighbor(d), Dir: d}
		ax, ay := b.hexToScreen(pos)
		dx, dy := dirVec(float64(d))
		arrow(dst, ax, ay, ax+dx*s*1.2, ay+dy*s*1.2, 3*z, 10*z, colAccent)
		lines = append(lines, "Направление: "+d.Name())
		if hex.Delta(facing, d) > 0 && sk.RequiresFacing {
			lines = append(lines, fmt.Sprintf("Разворот: +%.2f ОД", data.APf(b.w.TurnCost(u, facing, d))))
		}
	case "unit":
		t := b.visibleUnitAt(b.hover)
		if t == nil {
			return append(lines, "Выберите юнита")
		}
		tgt = sim.Target{Unit: t.ID, Hex: t.Pos}
		tgt.Dir, _ = hex.DirectionTo(pos, t.Pos)
		if sk.Attack && b.w.Hostile(u.Side, t.Side) {
			pv := b.attackPreview(dst, u, t, sk, pos)
			lines = append(lines, pv[1:len(pv)-1]...)
		} else {
			lines = append(lines, "Цель: "+t.Name)
		}
		if is := b.w.RangeIssue(pos, t.Pos, sk); is != nil {
			lines = append(lines, "! "+is.Text+" — юнит подойдёт")
		}
	case "hex":
		tgt = sim.Target{Hex: b.hover}
		tgt.Dir, _ = hex.DirectionTo(pos, b.hover)
		if is := b.w.RangeIssue(pos, b.hover, sk); is != nil {
			lines = append(lines, "! "+is.Text+" — юнит подойдёт")
		}
	default:
		return lines
	}
	area := b.w.SkillArea(sk, u, b.dashEnd(u, pos, sk, tgt), tgt)
	var hit []string
	for _, h := range area {
		b.hexOverlay(dst, h, rgba(255, 120, 40, 90), 0.9)
		b.hexOutline(dst, h, rgba(255, 170, 60, 220), 2, 0.9)
		if t := b.visibleUnitAt(h); t != nil {
			mark := t.Name
			if t.Side == b.player && sk.Affects == "all" && t.ID != u.ID {
				mark += " (СОЮЗНИК!)"
			}
			hit = append(hit, mark)
		}
	}
	if len(hit) > 0 {
		lines = append(lines, "В области: "+strings.Join(hit, ", "))
	} else if sk.Shape.Type != "single" {
		lines = append(lines, "В области сейчас никого")
	}
	if ready := u.Cooldowns[sk.ID]; ready > b.w.Tick {
		lines = append(lines, fmt.Sprintf("! Перезарядка ещё %.1f с — приказ подождёт", b.w.C.Seconds(ready-b.w.Tick)))
	}
	lines = append(lines, "ЛКМ — добавить в очередь (в точку вставки)")
	return lines
}

func (b *Battle) dashEnd(u *sim.Unit, pos hex.Hex, sk *data.SkillDef, tgt sim.Target) hex.Hex {
	if sk.Dash <= 0 {
		return pos
	}
	cur := pos
	prof := b.w.MoveProfile(u, false, tgt.Dir)
	for i := 0; i < sk.Dash; i++ {
		n := cur.Neighbor(tgt.Dir)
		if _, code := b.w.Map.StepCost(cur, n, prof); code != "" || b.w.Blocked(u, n) {
			break
		}
		cur = n
	}
	return cur
}

// cellTooltip — подсказка по клетке под курсором.
func (b *Battle) cellTooltip() ([]string, []color.RGBA) {
	h := b.hover
	c := b.w.Map.Cell(h)
	if c == nil {
		return nil, nil
	}
	lines := []string{fmt.Sprintf("%s  [%d:%d]  (q=%d, r=%d)", c.Terrain.Name, c.Col, c.Row, h.Q, h.R)}
	cols := []color.RGBA{colText}
	add := func(s string, col color.RGBA) { lines = append(lines, s); cols = append(cols, col) }
	if c.Height > 0 {
		add(fmt.Sprintf("Высота %d: +%d%% к точности по низинам", c.Height, b.w.C.Rules.HeightHitBonus*c.Height), colTextDim)
	}
	if c.Terrain.Cover > 0 {
		add(fmt.Sprintf("Укрытие: −%d%% к попаданию дальних атак", c.Terrain.Cover), colGood)
	}
	if c.Terrain.LOSCover > 0 {
		add(fmt.Sprintf("Частичное препятствие обзору: −%d%% к выстрелам сквозь", c.Terrain.LOSCover), colTextDim)
	}
	if c.Terrain.BlocksLOS {
		add("Перекрывает линию видимости", colTextDim)
	}
	if c.Feature != nil {
		add("Элемент: "+featureName(c.Feature.Kind, c.Feature.Open), colAccent)
	}
	if t := b.visibleUnitAt(h); t != nil {
		add("Занято: "+t.Name, colWarn)
	}
	if !b.visibleCells()[h] {
		add("Вне обзора отряда", colTextDim)
	}
	if u := b.selected(); u != nil {
		pos, _, _ := b.anchor(u)
		prof := b.w.MoveProfile(u, false, 0)
		pct, ok := prof.TerrainPct(c.Terrain)
		d := hex.Distance(pos, h)
		if ok && !c.IsClosedDoor() {
			add(fmt.Sprintf("Вход для %s: %d%% → %.2f ОД; расстояние %d", u.Name, pct, data.APf(prof.StepCost*pct/100), d), colTextDim)
		} else {
			why := "непроходимо для этого юнита"
			if c.IsClosedDoor() {
				why = "закрытая дверь"
			}
			add(fmt.Sprintf("Вход для %s: %s; расстояние %d", u.Name, why, d), colBad)
		}
	}
	return lines, cols
}
