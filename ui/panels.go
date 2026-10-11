package ui

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"hextactics/engine/data"
	"hextactics/engine/sim"
)

// button — элемент интерфейса, на который можно нажать.
type button struct {
	r       rect
	label   string
	key     string
	enabled bool
	active  bool
	tip     string
	act     func()
	style   int // 0 обычная, 1 вкладка/переключатель, 2 строка очереди, 3 крупная
	col     color.RGBA
}

func (b *Battle) addButton(bt *button) *button {
	b.buttons = append(b.buttons, bt)
	return bt
}

// ---------------------------------------------------------------------------
// Геометрия правой панели (общая для кнопок и отрисовки)

type rightGeom struct {
	x, w      float64
	header    rect
	bars      rect
	state     rect
	effects   rect
	effLines  [][]*sim.ActiveEffect
	passives  rect
	actions   []rect
	skillHead rect
	skills    []rect
	queueHead rect
	rows      []rect
	rowStep   []int
	hint      rect
}

func (b *Battle) rightLayout(u *sim.Unit) *rightGeom {
	W, H := float64(b.g.W), float64(b.g.H)
	g := &rightGeom{x: W - rightW + 10, w: rightW - 20}
	y := topH + 10
	g.header = rect{g.x, y, g.w, 50}
	y += 54
	nres := len(u.ClassDef().Resources)
	g.bars = rect{g.x, y, g.w, float64(44 + nres*18)}
	y += g.bars.h + 4
	g.state = rect{g.x, y, g.w, 58}
	y += 60
	// эффекты: «чипы» переносятся по строкам
	cx := 0.0
	var lineEff []*sim.ActiveEffect
	for _, e := range u.Effects {
		cw := b.effectChipWidth(e)
		if cx+cw > g.w && len(lineEff) > 0 {
			g.effLines = append(g.effLines, lineEff)
			lineEff, cx = nil, 0
		}
		lineEff = append(lineEff, e)
		cx += cw + 4
	}
	if len(lineEff) > 0 {
		g.effLines = append(g.effLines, lineEff)
	}
	effH := float64(max(1, len(g.effLines))) * 22
	g.effects = rect{g.x, y, g.w, effH}
	y += effH + 2
	g.passives = rect{g.x, y, g.w, 18}
	y += 24
	// быстрые действия: 2 ряда по 4
	bw := (g.w - 3*4) / 4
	for i := 0; i < 8; i++ {
		r := i / 4
		c := i % 4
		g.actions = append(g.actions, rect{g.x + float64(c)*(bw+4), y + float64(r)*30, bw, 26})
	}
	y += 64
	g.skillHead = rect{g.x, y, g.w, 18}
	y += 20
	for range skillSlots(u) {
		g.skills = append(g.skills, rect{g.x, y, g.w, 26})
		y += 29
	}
	y += 4
	g.queueHead = rect{g.x, y, g.w, 40}
	y += 44
	g.hint = rect{g.x, H - 46, g.w, 40}
	// строки очереди
	plan := b.w.ProjectPlan(u)
	rowH := 44.0
	avail := int((g.hint.y - 4 - y) / rowH)
	if avail < 1 {
		avail = 1
	}
	if b.queueScroll > max(0, len(plan.Steps)-avail) {
		b.queueScroll = max(0, len(plan.Steps)-avail)
	}
	for i := b.queueScroll; i < len(plan.Steps) && len(g.rows) < avail; i++ {
		g.rows = append(g.rows, rect{g.x, y, g.w, rowH - 4})
		g.rowStep = append(g.rowStep, i)
		y += rowH
	}
	return g
}

func (b *Battle) effectChipWidth(e *sim.ActiveEffect) float64 {
	return measure(b.effectChipText(e), 12, false) + 12
}

func (b *Battle) effectChipText(e *sim.ActiveEffect) string {
	d := b.w.C.Effects[e.Def]
	name := e.Def
	if d != nil {
		name = d.Name
	}
	s := name
	if e.Stacks > 1 {
		s += fmt.Sprintf(" ×%d", e.Stacks)
	}
	if e.Remaining > 0 {
		s += fmt.Sprintf(" %.1fс", b.w.C.Seconds(e.Remaining))
	}
	if e.Absorb > 0 {
		s += fmt.Sprintf(" (%d)", e.Absorb)
	}
	return s
}

// ---------------------------------------------------------------------------
// Кнопки (пересоздаются каждый кадр)

func (b *Battle) buildButtons() {
	b.buttons = b.buttons[:0]
	W, H := float64(b.g.W), float64(b.g.H)

	// Верхняя панель: скорость
	x := 10.0
	speeds := []struct {
		label string
		sp    int
		key   string
	}{{"II Пауза", 0, "Пробел"}, {"1×", 1, "="}, {"2×", 2, "="}, {"4×", 4, "="}}
	for _, s := range speeds {
		sp := s.sp
		w := 46.0
		if sp == 0 {
			w = 84
		}
		active := b.clock.Speed == sp
		tip := fmt.Sprintf("Скорость %d× (клавиши = и −)", sp)
		if sp == 0 {
			tip = "Пауза: время остановлено, приказы можно редактировать (Пробел)"
		}
		b.addButton(&button{r: rect{x, 8, w, 30}, label: s.label, enabled: true, active: active, style: 1, tip: tip,
			act: func() {
				if sp == 0 {
					b.clock.SetSpeed(0)
				} else {
					b.clock.SetSpeed(sp)
					b.showMenu = false
				}
			}})
		x += w + 4
	}
	right := W - 10
	addRight := func(label, tip string, active bool, act func()) {
		w := measure(label, 14, false) + 20
		right -= w
		b.addButton(&button{r: rect{right, 8, w, 30}, label: label, enabled: true, active: active, tip: tip, act: act, style: 1})
		right -= 4
	}
	addRight("Меню", "Меню (Esc)", b.showMenu, func() { b.showMenu = true; b.clock.SetSpeed(0) })
	addRight("? Справка", "Управление и правила (F1)", b.showHelp, func() { b.showHelp = true })
	addRight("Загрузить", "Загрузить быстрое сохранение (F9)", false, b.load)
	addRight("Сохранить", "Быстрое сохранение (F5)", false, b.save)
	addRight("Автопауза ▼", "Настройка событий, при которых игра встаёт на паузу", b.showAutoPause, func() { b.showAutoPause = !b.showAutoPause })
	qm := "ОД: гибко"
	if b.w.QueueMode == "strict" {
		qm = "ОД: строго"
	}
	defer func() { b.topRight = right }()
	addRight(qm, "Резервирование ОД (P): строгое — очередь не может превышать текущие ОД; гибкое — очередь ждёт восстановления", b.w.QueueMode == "strict", func() {
		mode := "strict"
		if b.w.QueueMode == "strict" {
			mode = "flexible"
		}
		b.command(sim.Command{Kind: sim.CmdSetQueueMode, Mode: mode})
	})

	// Выпадающее меню автопаузы
	if b.showAutoPause {
		ap := &b.clock.AutoPause
		items := []struct {
			label string
			v     *bool
		}{
			{"Замечен противник", &ap.EnemySpotted},
			{"Гибель своего юнита", &ap.UnitDied},
			{"Гибель важного юнита", &ap.ImportantDied},
			{"Опасная атака / опасное умение по отряду", &ap.DangerousAttack},
			{"Прерывание действия", &ap.Interrupted},
			{"Приказ не выполнен", &ap.OrderFailed},
			{"Юнит выполнил очередь", &ap.QueueFinished},
		}
		px := right - 200
		for i, it := range items {
			v := it.v
			mark := "□ "
			if *v {
				mark = "■ "
			}
			b.addButton(&button{r: rect{px, topH + 4 + float64(i)*30, 330, 28}, label: mark + it.label, enabled: true, active: *v, style: 2,
				act: func() { *v = !*v }})
		}
	}

	// Карточки отряда поверх карты
	cx := b.view.x + 8
	for _, u := range sortedUnitsByID(b.w.AliveUnits(b.player)) {
		id := u.ID
		b.addButton(&button{r: rect{cx, b.view.y + 8, 150, 50}, enabled: true, active: id == b.sel, style: 9, label: fmt.Sprint(id),
			tip: "", act: func() {
				b.trySelect(id)
			}})
		cx += 156
	}

	// Правая панель
	if u := b.selected(); u != nil {
		g := b.rightLayout(u)
		acts := []struct {
			label, key, tip string
			active          bool
			act             func()
		}{
			{"Движение", "M", "Режим движения: ЛКМ по клетке добавляет точку маршрута", b.mode == ModeMove, func() { b.cancelMode() }},
			{"Атака", "A", "Обычная атака: выберите противника (дойти и атаковать)", b.mode == ModeAttack, func() { b.setMode(ModeAttack, "") }},
			{"Поворот", "R", "Повернуться: выбрать направление мышью; стоит ОД за каждые 60°", b.mode == ModeTurn, func() { b.setMode(ModeTurn, "") }},
			{"Защита", "G", "Защитная стойка (+блок, +уклонение, +бдительность) и атака по врагу, вошедшему в зону", false, func() {
				b.issue(sim.OrderSpec{Type: sim.OrderGuard, Duration: b.w.C.Rules.GuardSeconds, React: true})
			}},
			{"Ждать", "Z", "Ожидание 1 с (Shift+Z — 3 с)", false, func() { b.issue(sim.OrderSpec{Type: sim.OrderWait, Duration: 1}) }},
			{"Взаимод.", "F", "Дверь или рычаг", b.mode == ModeInteract, func() { b.setMode(ModeInteract, "") }},
			{"Стоп", "X", "Завершить очередь (после «Стоп» нельзя добавлять приказы в конец)", false, func() { b.issue(sim.OrderSpec{Type: sim.OrderStop}) }},
			{"Без пов.", "K", "Отключить автоматический поворот при движении (пятиться, двигаться боком)", b.keepFacing, func() { b.keepFacing = !b.keepFacing }},
		}
		for i, a := range acts {
			b.addButton(&button{r: g.actions[i], label: a.label, key: a.key, enabled: true, active: a.active, tip: a.tip, act: a.act})
		}
		for i, id := range skillSlots(u) {
			sk := b.w.C.Skills[id]
			sid := id
			enabled, tip := b.skillAvailability(u, sk)
			b.addButton(&button{r: g.skills[i], label: sk.Name, key: fmt.Sprint(i + 1), enabled: true, active: b.mode == ModeSkill && b.skill == id,
				tip: tip, style: 4, col: sk.Color.Color(), act: func() { b.activateSkill(u, sid) }})
			_ = enabled
		}
		// Строки очереди
		plan := b.w.ProjectPlan(u)
		for ri, r := range g.rows {
			si := g.rowStep[ri]
			st := plan.Steps[si]
			oid := st.OrderID
			qi := si
			if u.Current != nil {
				qi = si - 1
			}
			row := r
			// кнопки справа: ↑ ↓ ✕
			bx := row.x + row.w - 3*24
			if !st.Active {
				up, down := qi, qi
				b.addButton(&button{r: rect{bx, row.y + 2, 22, 18}, label: "↑", enabled: qi > 0, tip: "Переставить выше (PageUp)", style: 5,
					act: func() { b.command(sim.Command{Kind: sim.CmdReorder, Unit: u.ID, OrderID: oid, NewIndex: up - 1}) }})
				b.addButton(&button{r: rect{bx + 24, row.y + 2, 22, 18}, label: "↓", enabled: qi < len(u.Queue)-1, tip: "Переставить ниже (PageDown)", style: 5,
					act: func() { b.command(sim.Command{Kind: sim.CmdReorder, Unit: u.ID, OrderID: oid, NewIndex: down + 1}) }})
			}
			b.addButton(&button{r: rect{bx + 48, row.y + 2, 22, 18}, label: "×", enabled: true, tip: "Удалить приказ (Backspace); активный — прервать", style: 5,
				act: func() {
					b.command(sim.Command{Kind: sim.CmdRemove, Unit: u.ID, OrderID: oid})
				}})
			// политика при ошибке
			pol := st.Policy
			plabel := "ошибка: " + sim.PolicyName(pol)
			pw := measure(plabel, 12, false) + 10
			b.addButton(&button{r: rect{row.x + row.w - pw - 2, row.y + 21, pw, 16}, label: plabel, enabled: true, style: 6,
				tip: "Политика обработки ошибки при исполнении (нажмите, чтобы сменить)",
				act: func() {
					next := sim.Policies[0]
					for i, p := range sim.Policies {
						if p == pol {
							next = sim.Policies[(i+1)%len(sim.Policies)]
						}
					}
					b.command(sim.Command{Kind: sim.CmdSetPolicy, Unit: u.ID, OrderID: oid, Policy: next})
				}})
			// сама строка — выбор точки вставки
			b.addButton(&button{r: row, enabled: true, style: 7, act: func() {
				if st.Active {
					b.cursor[u.ID] = 0
				} else if qi+1 >= len(u.Queue) {
					delete(b.cursor, u.ID)
				} else {
					b.cursor[u.ID] = qi + 1
				}
			}, tip: "Сделать точкой вставки: новые действия добавятся после этого приказа"})
		}
		// управление очередью
		qh := g.queueHead
		b.addButton(&button{r: rect{qh.x + qh.w - 196, qh.y, 64, 18}, label: "← Отмена", enabled: b.w.CanUndo(u), tip: "Отменить последнее изменение очереди (Ctrl+Z)", style: 5,
			act: func() { b.command(sim.Command{Kind: sim.CmdUndo, Unit: u.ID}) }})
		b.addButton(&button{r: rect{qh.x + qh.w - 128, qh.y, 128, 18}, label: "Очистить очередь", enabled: len(u.Queue) > 0 || u.Current != nil, tip: "Отменить всю очередь (Delete)", style: 5,
			act: func() { b.command(sim.Command{Kind: sim.CmdClear, Unit: u.ID}); delete(b.cursor, u.ID) }})
	}

	// Модальные окна
	if b.pending != nil {
		cx, cy := W/2, H/2
		to := "другого юнита"
		if tu := b.w.Unit(b.pending.to); tu != nil {
			to = tu.Name
		}
		b.addButton(&button{r: rect{cx - 250, cy + 20, 240, 34}, label: "Отменить (Esc)", enabled: true, style: 3,
			tip: "Отменить незавершённую команду и выбрать: " + to, act: func() { b.resolvePending(true) }})
		b.addButton(&button{r: rect{cx + 10, cy + 20, 240, 34}, label: "Вернуться к вводу (Enter)", enabled: true, style: 3, act: func() { b.resolvePending(false) }})
	}
	if b.showMenu {
		cx, cy := W/2, H/2-120
		items := []struct {
			label string
			act   func()
		}{
			{"Продолжить", func() { b.showMenu = false }},
			{"Сохранить (F5)", func() { b.save(); b.showMenu = false }},
			{"Загрузить (F9)", func() { b.load() }},
			{"Начать заново", func() { b.g.StartBattle(b.scenarioID) }},
			{"Главное меню", func() { b.g.toMenu() }},
			{"Выход из игры", func() { b.g.quit = true }},
		}
		for i, it := range items {
			b.addButton(&button{r: rect{cx - 140, cy + float64(i)*44, 280, 36}, label: it.label, enabled: true, style: 3, act: it.act})
		}
	}
	if b.showHelp {
		b.addButton(&button{r: rect{(W+math.Min(W-60, 1240))/2 - 140, 60, 120, 30}, label: "Закрыть", enabled: true, style: 3, act: func() { b.showHelp = false }})
	}
	if b.w.Outcome != "" && !b.showMenu {
		cx, cy := b.view.x+b.view.w/2, b.view.y+b.view.h/2
		b.addButton(&button{r: rect{cx - 210, cy + 30, 200, 36}, label: "Сыграть заново", enabled: true, style: 3, act: func() { b.g.StartBattle(b.scenarioID) }})
		b.addButton(&button{r: rect{cx + 10, cy + 30, 200, 36}, label: "Главное меню", enabled: true, style: 3, act: func() { b.g.toMenu() }})
	}
}

// skillAvailability — доступно ли умение сейчас и почему нет (для подсказки).
func (b *Battle) skillAvailability(u *sim.Unit, sk *data.SkillDef) (bool, string) {
	var parts []string
	parts = append(parts, sk.Name)
	if sk.Desc != "" {
		parts = append(parts, sk.Desc)
	}
	cost := fmt.Sprintf("Стоимость: %.1f ОД", sk.APCost)
	for r, v := range sk.Costs {
		if rd := b.w.C.Resources[r]; rd != nil {
			cost += fmt.Sprintf(", %s %.0f", rd.Name, v)
		}
	}
	parts = append(parts, cost)
	info := ""
	if sk.Range > 0 {
		info = fmt.Sprintf("Дальность %d–%d", sk.RangeMin, sk.Range)
		if sk.LOS {
			info += ", нужна линия видимости"
		}
	}
	if sk.Prep > 0 {
		info += fmt.Sprintf("; подготовка %.1f с", sk.Prep)
		if sk.Interruptible {
			info += " (прерывается уроном)"
		}
	}
	if sk.Cooldown > 0 {
		info += fmt.Sprintf("; перезарядка %.0f с", sk.Cooldown)
	}
	if info != "" {
		parts = append(parts, strings.TrimPrefix(info, "; "))
	}
	ok := true
	if ready := u.Cooldowns[sk.ID]; ready > b.w.Tick {
		ok = false
		parts = append(parts, fmt.Sprintf("Сейчас: перезарядка ещё %.1f с (приказ подождёт)", b.w.C.Seconds(ready-b.w.Tick)))
	}
	for r, v := range sk.Costs {
		if u.Res[r] < int(v*data.ResScale+0.5) {
			ok = false
			parts = append(parts, fmt.Sprintf("Сейчас: не хватает ресурса «%s» (приказ подождёт восстановления)", b.w.C.Resources[r].Name))
		}
	}
	return ok, strings.Join(parts, "\n")
}

// ---------------------------------------------------------------------------
// Отрисовка панелей

func (b *Battle) drawButton(dst *ebiten.Image, bt *button) {
	hover := bt == b.hoverBtn
	switch bt.style {
	case 9, 7: // карточки и строки очереди рисуются отдельно
		return
	}
	bg := colButton
	if bt.active {
		bg = colButtonOn
	} else if hover && bt.enabled {
		bg = colButtonHi
	}
	if !bt.enabled {
		bg = colButtonDis
	}
	fg := colText
	if !bt.enabled {
		fg = colTextFaint
	}
	switch bt.style {
	case 5, 6:
		if bt.style == 6 {
			bg = color.RGBA{0, 0, 0, 0}
			if hover {
				bg = colButtonHi
			}
		}
		fillRect(dst, bt.r, bg)
		drawText(dst, bt.label, 12, false, bt.r.x+bt.r.w/2, bt.r.y+1, fg, alCenter)
		return
	case 2:
		fillRect(dst, bt.r, mix(colPanel2, bg, 0.5))
		strokeRect(dst, bt.r, 1, colPanelEdge)
		drawText(dst, bt.label, 14, false, bt.r.x+10, bt.r.y+5, fg, alLeft)
		return
	}
	fillRect(dst, bt.r, bg)
	edge := colPanelEdge
	if bt.active {
		edge = colAccent
	}
	strokeRect(dst, bt.r, 1, edge)
	size := 14.0
	if bt.style == 3 {
		size = 16
	}
	if bt.style == 4 {
		// кнопка умения: цветная метка, номер, название, стоимость
		fillRect(dst, rect{bt.r.x, bt.r.y, 4, bt.r.h}, bt.col)
		drawText(dst, "["+bt.key+"]", 13, true, bt.r.x+10, bt.r.y+5, colAccent, alLeft)
		drawText(dst, bt.label, 14, false, bt.r.x+36, bt.r.y+4, fg, alLeft)
		return
	}
	ty := bt.r.y + (bt.r.h-size)/2 - 1
	if bt.key != "" {
		lw := measure(bt.label, 13, false)
		kw := measure(bt.key, 11, true)
		tx := bt.r.x + (bt.r.w-lw-kw-4)/2
		drawText(dst, bt.key, 11, true, tx, bt.r.y+7, colAccent, alLeft)
		drawText(dst, bt.label, 13, false, tx+kw+4, bt.r.y+5, fg, alLeft)
		return
	}
	drawText(dst, bt.label, size, false, bt.r.x+bt.r.w/2, ty, fg, alCenter)
}

func (b *Battle) drawTopBar(dst *ebiten.Image) {
	W := float64(b.g.W)
	fillRect(dst, rect{0, 0, W, topH}, colPanel2)
	line(dst, 0, topH, W, topH, 1, colPanelEdge)
	// время
	sec := b.w.Seconds()
	tstr := fmt.Sprintf("%02d:%04.1f", int(sec)/60, math.Mod(sec, 60))
	x := 10.0 + 84 + 4 + 3*50 + 10
	drawText(dst, "Время боя", 11, false, x, 6, colTextDim, alLeft)
	drawText(dst, tstr, 18, true, x, 19, colText, alLeft)
	x += 90
	state := "ПАУЗА"
	col := colWarn
	if !b.clock.Paused() {
		state = fmt.Sprintf("ИДЁТ %d×", b.clock.Speed)
		col = colGood
	}
	drawText(dst, state, 11, true, x, 6, col, alLeft)
	drawText(dst, fmt.Sprintf("тик %d", b.w.Tick), 12, false, x, 23, colTextDim, alLeft)
	x += 80
	// режим ввода
	drawText(dst, "Режим:", 11, false, x, 6, colTextDim, alLeft)
	m := b.modeName()
	maxW := b.topRight - x - 12
	for measure(m, 13, false) > maxW && len([]rune(m)) > 10 {
		r := []rune(m)
		m = string(r[:len(r)-2])
	}
	drawText(dst, m, 13, true, x, 22, colAccent, alLeft)
}

func (b *Battle) logRect() rect {
	W, H := float64(b.g.W), float64(b.g.H)
	return rect{0, H - logH, W - rightW, logH}
}

func (b *Battle) drawLog(dst *ebiten.Image) {
	r := b.logRect()
	panel(dst, r)
	title := "Журнал боя"
	if b.logDetails {
		title += " — подробно (L)"
	} else {
		title += " — нажмите на строку или L для расчётов"
	}
	drawText(dst, title, 12, true, r.x+10, r.y+5, colTextDim, alLeft)
	// Строки снизу вверх
	type vis struct {
		text string
		col  color.RGBA
		ind  bool
		seq  uint64
	}
	var lines []vis
	for _, l := range b.logLines {
		c := logColor(l.kind)
		ts := fmt.Sprintf("%5.1f  ", b.w.C.Seconds(l.tick))
		lines = append(lines, vis{text: ts + l.text, col: c, seq: l.seq})
		if b.logDetails || b.logExpanded[l.seq] {
			for _, d := range l.details {
				for _, wl := range wrap(d, 12, false, r.w-80) {
					lines = append(lines, vis{text: wl, col: colTextDim, ind: true, seq: l.seq})
				}
			}
		}
	}
	lh := 17.0
	n := int((r.h - 26) / lh)
	maxScroll := max(0, len(lines)-n)
	if b.logScroll > maxScroll {
		b.logScroll = maxScroll
	}
	end := len(lines) - b.logScroll
	start := max(0, end-n)
	y := r.y + 24
	b.logHit = b.logHit[:0]
	for i := start; i < end; i++ {
		l := lines[i]
		x := r.x + 10
		size := 13.0
		if l.ind {
			x += 60
			size = 12
		}
		txt := l.text
		for measure(txt, size, false) > r.w-x+r.x-10 && len([]rune(txt)) > 4 {
			rr := []rune(txt)
			txt = string(rr[:len(rr)-2])
		}
		drawText(dst, txt, size, false, x, y, l.col, alLeft)
		b.logHit = append(b.logHit, logHit{r: rect{r.x, y, r.w, lh}, seq: l.seq})
		y += lh
	}
	if b.logScroll > 0 {
		drawText(dst, fmt.Sprintf("▼ ещё %d (колесо мыши)", b.logScroll), 11, false, r.x+r.w-10, r.y+5, colTextDim, alRight)
	}
}

type logHit struct {
	r   rect
	seq uint64
}

func (b *Battle) clickLog() {
	for _, h := range b.logHit {
		if h.r.contains(b.mx, b.my) {
			b.logExpanded[h.seq] = !b.logExpanded[h.seq]
			return
		}
	}
}

func logColor(t sim.EventType) color.RGBA {
	switch t {
	case sim.EvAttack, sim.EvDamage:
		return rgb(255, 150, 130)
	case sim.EvHeal:
		return colGood
	case sim.EvMissed, sim.EvEvaded, sim.EvEffectResisted:
		return rgb(170, 190, 220)
	case sim.EvEffectApplied, sim.EvEffectExpired, sim.EvPassiveTriggered:
		return rgb(200, 170, 255)
	case sim.EvUnitDied:
		return colBad
	case sim.EvOrderFailed, sim.EvActionInterrupted:
		return colWarn
	case sim.EvEnemySpotted:
		return rgb(255, 120, 120)
	case sim.EvBattleEnded:
		return colAccent
	case sim.EvSkillPrepStarted, sim.EvObjectPlaced, sim.EvObjectTriggered:
		return rgb(255, 190, 120)
	}
	return colText
}

func (b *Battle) drawPartyCards(dst *ebiten.Image) {
	for _, bt := range b.buttons {
		if bt.style != 9 {
			continue
		}
		var id int
		fmt.Sscan(bt.label, &id)
		u := b.w.Unit(id)
		if u == nil {
			continue
		}
		r := bt.r
		bg := withAlpha(colPanel2, 225)
		if bt == b.hoverBtn {
			bg = withAlpha(colButtonHi, 235)
		}
		fillRect(dst, r, bg)
		edge := colPanelEdge
		if bt.active {
			edge = colSelect
		}
		strokeRect(dst, r, 2, edge)
		sc := b.w.SideOf(u.Side).Color.Color()
		circle(dst, r.x+16, r.y+17, 11, sc)
		drawText(dst, u.ClassDef().Glyph, 13, true, r.x+16, r.y+9, colBg, alCenter)
		name := u.Name
		for measure(name, 13, true) > r.w-40 {
			rr := []rune(name)
			name = string(rr[:len(rr)-1])
		}
		drawText(dst, name, 13, true, r.x+32, r.y+3, colText, alLeft)
		hpf := float64(u.HP) / float64(max(1, u.HPMax()))
		bar(dst, rect{r.x + 32, r.y + 21, r.w - 40, 6}, hpf, hpColor(hpf), rgb(50, 30, 30))
		bar(dst, rect{r.x + 32, r.y + 30, r.w - 40, 5}, float64(u.AP)/float64(max(1, u.APMax())), colAP, rgb(30, 36, 60))
		st := b.w.State(u)
		q := len(u.Queue)
		if u.Current != nil {
			q++
		}
		info := sim.StateName(st)
		if q > 0 {
			info += fmt.Sprintf(" · приказов %d", q)
		}
		for measure(info, 10, false) > r.w-36 && len([]rune(info)) > 4 {
			rr := []rune(info)
			info = string(rr[:len(rr)-2])
		}
		drawText(dst, info, 10, false, r.x+32, r.y+37, colTextDim, alLeft)
	}
}

func hpColor(f float64) color.RGBA {
	if f < 0.35 {
		return colHPLow
	}
	if f < 0.65 {
		return colWarn
	}
	return colHP
}

func (b *Battle) drawRightPanel(dst *ebiten.Image) {
	W, H := float64(b.g.W), float64(b.g.H)
	r := rect{W - rightW, topH, rightW, H - topH}
	fillRect(dst, r, colPanel)
	line(dst, r.x, r.y, r.x, r.y+r.h, 1, colPanelEdge)
	u := b.selected()
	if u == nil {
		drawText(dst, "Выберите юнита (ЛКМ или Tab)", 15, false, r.x+20, r.y+20, colTextDim, alLeft)
		return
	}
	g := b.rightLayout(u)
	cls := u.ClassDef()
	// Заголовок
	sc := b.w.SideOf(u.Side).Color.Color()
	circle(dst, g.x+22, g.header.y+24, 20, sc)
	ring(dst, g.x+22, g.header.y+24, 20, 2, colSelect)
	drawText(dst, cls.Glyph, 20, true, g.x+22, g.header.y+11, colBg, alCenter)
	name := u.Name
	if cls.Important {
		name += "  ♦"
	}
	drawText(dst, name, 18, true, g.x+52, g.header.y+2, colText, alLeft)
	drawText(dst, cls.Name+" · передвижение: "+moveClassName(cls.MoveClass), 12, false, g.x+52, g.header.y+26, colTextDim, alLeft)

	// Полосы
	y := g.bars.y
	hpf := float64(u.HP) / float64(max(1, u.HPMax()))
	drawText(dst, fmt.Sprintf("HP %d / %d", u.HP, u.HPMax()), 12, false, g.x, y, colText, alLeft)
	bar(dst, rect{g.x + 110, y + 3, g.w - 110, 10}, hpf, hpColor(hpf), rgb(50, 30, 30))
	y += 18
	plan := b.w.ProjectPlan(u)
	apMax := float64(max(1, u.APMax()))
	drawText(dst, fmt.Sprintf("ОД %.1f / %.0f", data.APf(u.AP), data.APf(u.APMax())), 12, false, g.x, y, colText, alLeft)
	br := rect{g.x + 110, y + 3, g.w - 110, 10}
	bar(dst, br, float64(u.AP)/apMax, colAP, rgb(30, 36, 60))
	// резерв под очередь — штриховка поверх
	if plan.Total > 0 {
		resW := math.Min(float64(plan.Total)/apMax, 1) * br.w
		start := br.x + math.Max(0, float64(u.AP)/apMax*br.w-resW)
		fillRect(dst, rect{start, br.y, math.Min(resW, float64(u.AP)/apMax*br.w), br.h}, withAlpha(colAPRes, 150))
		if plan.Shortfall > 0 {
			sw := math.Min(float64(plan.Shortfall)/apMax, 1) * br.w
			strokeRect(dst, rect{br.x + float64(u.AP)/apMax*br.w, br.y - 1, math.Min(sw, br.w-float64(u.AP)/apMax*br.w), br.h + 2}, 1, colBad)
		}
	}
	y += 16
	regen := u.Stat("ap_regen")
	apInfo := fmt.Sprintf("+%.2f ОД/с · резерв очереди %.1f · свободно %.1f", regen, data.APf(plan.Total), data.APf(max(0, plan.Free)))
	if b.w.C.Rules.APModel == "cycle" {
		apInfo = fmt.Sprintf("восстановление в начале цикла · резерв %.1f", data.APf(plan.Total))
	}
	if plan.Shortfall > 0 {
		apInfo = fmt.Sprintf("+%.2f ОД/с · резерв %.1f · НЕ ХВАТАЕТ %.1f (~%.1f с)", regen, data.APf(plan.Total), data.APf(plan.Shortfall), plan.WaitSec)
	}
	drawText(dst, apInfo, 11, false, g.x, y, colTextDim, alLeft)
	y += 16
	for _, rid := range cls.Resources {
		rd := b.w.C.Resources[rid]
		mx := float64(max(1, b.w.ResMax(u, rid)))
		drawText(dst, fmt.Sprintf("%s %.0f / %.0f", rd.Name, float64(u.Res[rid])/data.ResScale, mx/data.ResScale), 12, false, g.x, y, colText, alLeft)
		bar(dst, rect{g.x + 150, y + 3, g.w - 150, 8}, float64(u.Res[rid])/mx, rd.Color.Color(), rgb(36, 36, 44))
		y += 18
	}
	// Состояние
	y = g.state.y
	st := b.w.State(u)
	facing := "Взгляд: " + u.Facing.Name()
	drawText(dst, facing, 13, false, g.x, y, colText, alLeft)
	stText := "Состояние: " + sim.StateName(st)
	if a := u.Action; a != nil {
		stText += fmt.Sprintf(" (%.0f%%)", b.actionProgress(a)*100)
		if a.Skill != "" {
			if sk := b.w.C.Skills[a.Skill]; sk != nil {
				stText += " — " + sk.Name
			}
		}
	}
	drawText(dst, stText, 13, false, g.x+130, y, stateColor(st), alLeft)
	y += 18
	if u.Current != nil {
		s := "Сейчас: " + b.w.Describe(u.Current)
		col := colText
		if u.Current.Status == sim.StatusWaiting && u.Current.Issue != nil {
			s += " — ждёт: " + u.Current.Issue.Text
			col = colWarn
		}
		for _, l := range wrap(s, 12, false, g.w)[:min(2, len(wrap(s, 12, false, g.w)))] {
			drawText(dst, l, 12, false, g.x, y, col, alLeft)
			y += 15
		}
	} else if u.LastIssue != nil {
		drawText(dst, "Последняя остановка: "+u.LastIssue.Text, 12, false, g.x, y, colWarn, alLeft)
	} else {
		drawText(dst, "Приказов нет", 12, false, g.x, y, colTextDim, alLeft)
	}
	// Эффекты
	y = g.effects.y
	if len(u.Effects) == 0 {
		drawText(dst, "Эффектов нет", 12, false, g.x, y+3, colTextFaint, alLeft)
	}
	b.effectHits = b.effectHits[:0]
	for li, ln := range g.effLines {
		x := g.x
		for _, e := range ln {
			d := b.w.C.Effects[e.Def]
			cw := b.effectChipWidth(e)
			c := rgb(120, 120, 140)
			if d != nil {
				c = d.Color.Color()
			}
			cr := rect{x, y + float64(li)*22, cw, 19}
			fillRect(dst, cr, withAlpha(shade(c, 0.45), 230))
			strokeRect(dst, cr, 1, c)
			drawText(dst, b.effectChipText(e), 12, false, x+6, cr.y+2, colText, alLeft)
			if d != nil {
				b.effectHits = append(b.effectHits, effectHit{r: cr, tip: d.Name + ": " + d.Desc})
			}
			x += cw + 4
		}
	}
	// Пассивные навыки
	var pn []string
	for _, p := range u.Passives() {
		pn = append(pn, p.Name)
	}
	ptxt := "Пассивные: —"
	if len(pn) > 0 {
		ptxt = "Пассивные: " + strings.Join(pn, ", ")
	}
	drawText(dst, ptxt, 12, false, g.x, g.passives.y, rgb(200, 170, 255), alLeft)
	b.passiveHit = rect{g.x, g.passives.y, measure(ptxt, 12, false), 16}

	// Очередь
	qh := g.queueHead
	drawText(dst, "Очередь приказов", 14, true, qh.x, qh.y, colText, alLeft)
	sum := fmt.Sprintf("Итого %.1f ОД · после очереди: %.1f ОД", data.APf(plan.Total), data.APf(plan.Free))
	sumCol := colTextDim
	if plan.Shortfall > 0 {
		sum = fmt.Sprintf("Итого %.1f ОД · не хватает %.1f → ожидание ~%.1f с", data.APf(plan.Total), data.APf(plan.Shortfall), plan.WaitSec)
		sumCol = colWarn
	}
	if plan.Errors > 0 {
		sum += fmt.Sprintf(" · ошибок: %d", plan.Errors)
		sumCol = colBad
	}
	drawText(dst, sum, 12, false, qh.x, qh.y+20, sumCol, alLeft)
	if len(plan.Steps) == 0 {
		drawText(dst, "Пусто. ЛКМ по карте — маршрут; по врагу — атака.", 12, false, g.x, qh.y+46, colTextFaint, alLeft)
	}
	cursorStep := b.cursorIdx(u)
	if u.Current != nil {
		cursorStep++
	}
	for ri, rr := range g.rows {
		si := g.rowStep[ri]
		s := plan.Steps[si]
		bg := colPanel2
		for _, bt := range b.buttons {
			if bt.style == 7 && bt.r == rr && bt == b.hoverBtn {
				bg = colButtonHi
			}
		}
		if s.Active {
			bg = mix(bg, rgb(60, 90, 60), 0.5)
		}
		fillRect(dst, rr, bg)
		edge := colPanelEdge
		if s.Error != nil {
			edge = colBad
		}
		strokeRect(dst, rr, 1, edge)
		num := b.orderNumber(u, s.OrderID)
		badgeCol := orderColor(s.Type)
		circle(dst, rr.x+14, rr.y+12, 10, badgeCol)
		drawText(dst, fmt.Sprint(num), 12, true, rr.x+14, rr.y+4, colBg, alCenter)
		title := s.Text
		for measure(title, 13, false) > rr.w-130 && len([]rune(title)) > 4 {
			t := []rune(title)
			title = string(t[:len(t)-2])
		}
		drawText(dst, title, 13, false, rr.x+30, rr.y+3, colText, alLeft)
		cost := fmt.Sprintf("%.1f ОД", data.APf(s.Cost))
		drawText(dst, cost, 12, true, rr.x+rr.w-78, rr.y+4, colAP, alRight)
		// вторая строка: статус/предупреждение
		var second string
		col := colTextDim
		switch {
		case s.Error != nil:
			second = "× " + s.Error.Text
			col = colBad
		case s.Active:
			second = "► " + sim.StatusName(s.Status)
			col = colGood
			if s.Status == sim.StatusWaiting {
				col = colWarn
			}
			for _, is := range s.Issues {
				if is.Code != "" {
					second += ": " + is.Text
					break
				}
			}
		case s.NeedsWait:
			second = "… ожидание ОД"
			col = colWarn
		}
		if s.Error == nil {
			for _, is := range s.Issues {
				if is.Code != "" && !s.Active {
					if second != "" {
						second += " · "
					}
					second += "! " + is.Text
					col = colWarn
				} else if is.Code == "" && second == "" {
					second = is.Text
				}
			}
		}
		for measure(second, 11, false) > rr.w-200 && len([]rune(second)) > 4 {
			t := []rune(second)
			second = string(t[:len(t)-2])
		}
		drawText(dst, second, 11, false, rr.x+30, rr.y+22, col, alLeft)
		if si == cursorStep-1 && cursorStep < len(plan.Steps) {
			line(dst, rr.x, rr.y+rr.h+2, rr.x+rr.w, rr.y+rr.h+2, 3, colAccent)
		}
	}
	// Кнопки панели (действия, умения, управление очередью) — поверх строк
	for _, bt := range b.buttons {
		if bt.r.x >= W-rightW && bt.style != 2 && bt.style != 3 {
			b.drawButton(dst, bt)
		}
	}
	drawText(dst, "Умения (клавиши 1–6):", 12, true, g.x, g.skillHead.y, colTextDim, alLeft)
	for i, id := range skillSlots(u) {
		sk := b.w.C.Skills[id]
		r := g.skills[i]
		info := fmt.Sprintf("%.1f ОД", sk.APCost)
		for rid, v := range sk.Costs {
			if rd := b.w.C.Resources[rid]; rd != nil {
				info += fmt.Sprintf(" · %.0f %s", v, strings.ToLower(string([]rune(rd.Name)[:3])))
			}
		}
		col := colTextDim
		if ready := u.Cooldowns[id]; ready > b.w.Tick {
			info = fmt.Sprintf("перезарядка %.1f с", b.w.C.Seconds(ready-b.w.Tick))
			col = colWarn
			total := b.w.Ticks(sk.Cooldown)
			if total > 0 {
				frac := float64(ready-b.w.Tick) / float64(total)
				fillRect(dst, rect{r.x + 4, r.y + r.h - 3, (r.w - 4) * frac, 3}, colWarn)
			}
		} else {
			for rid, v := range sk.Costs {
				if u.Res[rid] < int(v*data.ResScale+0.5) {
					col = colBad
				}
			}
		}
		drawText(dst, info, 12, false, r.x+r.w-8, r.y+6, col, alRight)
	}
	if len(g.rows) < len(plan.Steps) {
		drawText(dst, fmt.Sprintf("… показано %d из %d (колесо мыши)", len(g.rows), len(plan.Steps)), 11, false, g.x, g.hint.y-16, colTextDim, alLeft)
	}
	// Подсказка
	hint := "Пробел — пауза/ход · Tab — след. юнит · Backspace — удалить · Ctrl+Z — отмена · [ ] — точка вставки · F1 — справка"
	for i, l := range wrap(hint, 11, false, g.w) {
		drawText(dst, l, 11, false, g.x, g.hint.y+float64(i)*14, colTextFaint, alLeft)
	}
}

type effectHit struct {
	r   rect
	tip string
}

func moveClassName(c string) string {
	switch c {
	case "heavy":
		return "тяжёлый"
	case "foot":
		return "пеший"
	}
	return c
}

func stateColor(s sim.UnitState) color.RGBA {
	switch s {
	case sim.StateStunned, sim.StateAsleep:
		return colWarn
	case sim.StatePreparing, sim.StateChanneling:
		return rgb(255, 170, 90)
	case sim.StateMoving, sim.StateTurning:
		return colGood
	case sim.StateDead:
		return colBad
	}
	return colText
}

func orderColor(t sim.OrderType) color.RGBA {
	switch t {
	case sim.OrderMove:
		return rgb(230, 230, 230)
	case sim.OrderTurn:
		return rgb(140, 200, 255)
	case sim.OrderAttack:
		return rgb(255, 120, 100)
	case sim.OrderSkill:
		return rgb(255, 180, 70)
	case sim.OrderInteract:
		return rgb(190, 160, 110)
	case sim.OrderWait:
		return rgb(170, 170, 190)
	case sim.OrderGuard:
		return rgb(110, 200, 255)
	case sim.OrderStop:
		return rgb(240, 80, 80)
	}
	return colText
}

func orderGlyph(t sim.OrderType) string {
	switch t {
	case sim.OrderTurn:
		return "П"
	case sim.OrderAttack:
		return "А"
	case sim.OrderSkill:
		return "У"
	case sim.OrderInteract:
		return "В"
	case sim.OrderWait:
		return "Ж"
	case sim.OrderGuard:
		return "З"
	case sim.OrderStop:
		return "■"
	}
	return ""
}

// ---------------------------------------------------------------------------
// Оверлеи

func (b *Battle) drawToasts(dst *ebiten.Image) {
	y := b.view.y + b.view.h - 34
	for i := len(b.toasts) - 1; i >= 0; i-- {
		t := b.toasts[i]
		c := colText
		switch t.col {
		case ckGood:
			c = colGood
		case ckWarn:
			c = colWarn
		case ckBad:
			c = colBad
		}
		a := uint8(math.Min(1, t.t) * 230)
		w := measure(t.text, 14, false) + 24
		x := b.view.x + (b.view.w-w)/2
		fillRect(dst, rect{x, y, w, 26}, withAlpha(rgb(15, 16, 22), a))
		strokeRect(dst, rect{x, y, w, 26}, 1, withAlpha(c, a))
		drawText(dst, t.text, 14, false, x+12, y+5, withAlpha(c, a), alLeft)
		y -= 30
	}
	// Причина автопаузы (клик — камера к месту события)
	if r, s, ok := b.pauseBanner(); ok {
		bg := rgba(60, 40, 10, 235)
		if r.contains(b.mx, b.my) {
			bg = rgba(90, 60, 15, 240)
		}
		fillRect(dst, r, bg)
		strokeRect(dst, r, 2, colWarn)
		drawText(dst, s, 15, true, r.x+15, r.y+6, colAccent, alLeft)
	}
}

func (b *Battle) drawTooltipBox(dst *ebiten.Image, lines []string, cols []color.RGBA) {
	if len(lines) == 0 {
		return
	}
	w := 0.0
	for _, l := range lines {
		w = math.Max(w, measure(l, 13, false))
	}
	w += 20
	h := float64(len(lines))*17 + 10
	x, y := b.mx+18, b.my+18
	if x+w > float64(b.g.W)-4 {
		x = b.mx - w - 12
	}
	if y+h > float64(b.g.H)-4 {
		y = float64(b.g.H) - h - 4
	}
	fillRect(dst, rect{x, y, w, h}, rgba(14, 15, 20, 240))
	strokeRect(dst, rect{x, y, w, h}, 1, colPanelEdge)
	for i, l := range lines {
		c := colText
		if i < len(cols) {
			c = cols[i]
		}
		drawText(dst, l, 13, i == 0, x+10, y+5+float64(i)*17, c, alLeft)
	}
}

func (b *Battle) drawOverlays(dst *ebiten.Image) {
	W, H := float64(b.g.W), float64(b.g.H)
	if b.showAutoPause {
		for _, bt := range b.buttons {
			if bt.style == 2 {
				b.drawButton(dst, bt)
			}
		}
	}
	if b.w.Outcome != "" && !b.showMenu {
		cx, cy := b.view.x+b.view.w/2, b.view.y+b.view.h/2
		r := rect{cx - 260, cy - 70, 520, 150}
		fillRect(dst, r, rgba(15, 15, 22, 235))
		title, col := "ПОБЕДА", colAccent
		if b.w.Outcome == "defeat" {
			title, col = "ПОРАЖЕНИЕ", colBad
		}
		strokeRect(dst, r, 2, col)
		drawText(dst, title, 36, true, cx, cy-60, col, alCenter)
		drawText(dst, fmt.Sprintf("Время боя: %.1f с", b.w.Seconds()), 14, false, cx, cy-10, colTextDim, alCenter)
		for _, bt := range b.buttons {
			if bt.style == 3 && bt.r.y > cy {
				b.drawButton(dst, bt)
			}
		}
	}
	if b.pending != nil || b.showMenu || b.showHelp {
		fillRect(dst, rect{0, 0, W, H}, rgba(0, 0, 0, 150))
	}
	if b.pending != nil {
		r := rect{W/2 - 280, H/2 - 80, 560, 150}
		panel(dst, r)
		drawText(dst, "Незавершённая команда", 18, true, W/2, r.y+14, colAccent, alCenter)
		drawText(dst, "Не завершено: "+b.pending.what, 14, false, W/2, r.y+44, colText, alCenter)
		drawText(dst, "Подтвердите отмену или вернитесь и завершите ввод.", 13, false, W/2, r.y+66, colTextDim, alCenter)
	}
	if b.showMenu {
		r := rect{W/2 - 170, H/2 - 170, 340, 330}
		panel(dst, r)
		drawText(dst, "Меню", 20, true, W/2, r.y+10, colAccent, alCenter)
	}
	if b.showHelp {
		b.drawHelp(dst)
	}
	for _, bt := range b.buttons {
		if bt.style == 3 && (b.pending != nil || b.showMenu || b.showHelp) {
			b.drawButton(dst, bt)
		}
	}
}

func (b *Battle) drawHelp(dst *ebiten.Image) {
	W, H := float64(b.g.W), float64(b.g.H)
	pw := math.Min(W-60, 1240)
	r := rect{(W - pw) / 2, 50, pw, H - 100}
	panel(dst, r)
	drawText(dst, "Управление и правила", 20, true, r.x+20, r.y+14, colAccent, alLeft)
	left := []string{
		"ВРЕМЯ",
		"Пробел — пауза / продолжить; = и − — скорость 1× / 2× / 4×.",
		"На паузе можно выбирать юнитов и редактировать приказы.",
		"Кнопка «Автопауза ▼» — события, останавливающие время.",
		"",
		"ВЫБОР И КАМЕРА",
		"ЛКМ по своему юниту или карточке — выбор; Tab / Shift+Tab — следующий.",
		"Колесо — масштаб; ПКМ/СКМ с перетаскиванием или стрелки — сдвиг; C — к юниту.",
		"",
		"МАРШРУТ И ОЧЕРЕДЬ",
		"ЛКМ по клетке — точка маршрута (следующий клик продолжает путь).",
		"Ctrl+ЛКМ — новый маршрут вместо очереди; Shift+ЛКМ или K — без поворота.",
		"Перетащите свою точку маршрута — изменить промежуточный гекс.",
		"Клик по точке маршрута или строке очереди — точка вставки: умения и действия добавятся прямо в эту точку. [ и ] — сдвиг точки вставки, End — в конец.",
		"ЛКМ по врагу — дойти и атаковать; по рычагу или закрытой двери — взаимодействие.",
		"A — атака, R — поворот (мышью), G — защита, Z — ждать, F — дверь/рычаг, X — стоп.",
		"1–6 — умения; направленные: наведите мышь на сторону и нажмите ЛКМ.",
		"Backspace — удалить приказ; Delete — очистить очередь; Ctrl+Z — отменить изменение.",
		"PageUp / PageDown — переставить приказ; P — строгое/гибкое резервирование ОД.",
		"ПКМ или Esc — отменить режим; F5 / F9 — сохранить / загрузить; L — подробный журнал.",
	}
	right := []string{
		"ПРАВИЛА",
		"ОД восстанавливаются непрерывно (см. «ОД/с»). Действие списывает ОД в момент начала; отмена не возвращает ОД. Если ОД не хватает, приказ ждёт.",
		"Поворот стоит ОД за каждые 60°. При обычном движении юнит поворачивается бесплатно; без поворота шаги не вперёд дороже.",
		"Сектор атаки считается от взгляда цели: фронт, ближний фланг, дальний фланг, тыл. Сзади выше неожиданность, ниже уклонение, блок и эффективность брони. Неожиданность не означает автопопадание.",
		"Попадание, уклонение, блок и крит — независимые броски; клик по строке журнала показывает расчёт.",
		"Высота даёт бонус к точности; лес и кусты — укрытие от выстрелов; стены и закрытые двери перекрывают обзор. Затемнённые клетки не видны отряду — враги там не отображаются.",
		"У каждого приказа есть политика ошибки (строка очереди): пропустить, прервать очередь, перестроить путь, ждать. Цели и условия проверяются заново в момент исполнения.",
	}
	colW := r.w/2 - 40
	drawCol := func(items []string, x float64) {
		y := r.y + 52
		for _, l := range items {
			if l != "" && l == strings.ToUpper(l) {
				drawText(dst, l, 14, true, x, y, colAccent, alLeft)
				y += 22
				continue
			}
			if l == "" {
				y += 8
				continue
			}
			for _, wl := range wrap(l, 13, false, colW) {
				drawText(dst, wl, 13, false, x, y, colText, alLeft)
				y += 18
			}
			y += 3
		}
	}
	drawCol(left, r.x+20)
	drawCol(right, r.x+r.w/2+20)
}
