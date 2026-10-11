package ui

import (
	"fmt"
	"math"
	"os"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"hextactics/engine/gamemap"
	"hextactics/engine/hex"
	"hextactics/engine/sim"
)

// Mode — режим ввода (отображается в верхней панели).
type Mode int

const (
	ModeMove Mode = iota
	ModeAttack
	ModeSkill
	ModeTurn
	ModeInteract
)

const (
	topH     = 46.0
	rightW   = 420.0
	logH     = 200.0
	baseSize = 30.0
)

type press struct {
	x, y       float64
	hex        hex.Hex
	moved      bool
	button     ebiten.MouseButton
	orderID    int // перетаскивание точки маршрута
	camX, camY float64
}

type pendingSwitch struct {
	to   int
	what string
}

type toast struct {
	text string
	col  colorKind
	t    float64
}

type colorKind int

const (
	ckInfo colorKind = iota
	ckGood
	ckWarn
	ckBad
)

// Battle — экран боя.
type Battle struct {
	g          *Game
	w          *sim.World
	clock      *sim.Clock
	scenarioID string
	player     int

	zoom, camX, camY float64
	view             rect

	sel        int
	mode       Mode
	skill      string
	keepFacing bool
	cursor     map[int]int // точка вставки: индекс в очереди (−1 — конец)
	pending    *pendingSwitch

	mx, my   float64
	hover    hex.Hex
	hoverIn  bool
	lp, rp   *press
	buttons  []*button
	hoverBtn *button
	toasts   []toast

	logLines    []logLine
	logScroll   int
	logDetails  bool
	logExpanded map[uint64]bool
	queueScroll int
	lastSeq     uint64

	showAutoPause, showHelp, showMenu bool

	terrain    *ebiten.Image
	terrainKey [2]uint64
	terrainOX  float64
	terrainOY  float64
	visKey     [2]uint64
	visCells   map[hex.Hex]bool
	reachKey   [6]uint64
	reach      map[hex.Hex]gamemap.Reach

	vfx  *vfxSystem
	time float64

	topRight   float64
	showDiag   bool
	logHit     []logHit
	effectHits []effectHit
	passiveHit rect
}

func newBattle(g *Game, scenarioID string, loaded *sim.World) (*Battle, error) {
	w := loaded
	if w == nil {
		var err error
		w, err = sim.New(g.Content, scenarioID, 0)
		if err != nil {
			return nil, err
		}
	}
	b := &Battle{g: g, w: w, scenarioID: w.Scenario.ID, player: w.PlayerSide(), zoom: 1,
		cursor: map[int]int{}, logExpanded: map[uint64]bool{}, vfx: newVFX()}
	b.clock = sim.NewClock(b.player)
	if us := w.AliveUnits(b.player); len(us) > 0 {
		b.sel = us[0].ID
	}
	b.layoutView()
	b.centerOn(b.partyCenter())
	b.consumeEvents()
	b.addToast("Пауза. Выберите юнита и задайте приказы, затем нажмите Пробел", ckInfo)
	return b, nil
}

// ---------------------------------------------------------------------------
// Координаты

func (b *Battle) size() float64 { return baseSize * b.zoom }

func (b *Battle) layoutView() {
	W, H := float64(b.g.W), float64(b.g.H)
	b.view = rect{0, topH, W - rightW, H - topH - logH}
}

func (b *Battle) hexToScreen(h hex.Hex) (float64, float64) {
	x, y := hex.ToPixel(h, b.size())
	return x - b.camX + b.view.x, y - b.camY + b.view.y
}

func (b *Battle) screenToHex(sx, sy float64) hex.Hex {
	return hex.FromPixel(sx-b.view.x+b.camX, sy-b.view.y+b.camY, b.size())
}

// unitScreenPos — позиция юнита на экране с интерполяцией движения между тиками.
func (b *Battle) unitScreenPos(u *sim.Unit) (float64, float64) {
	x, y := b.hexToScreen(u.Pos)
	if a := u.Action; a != nil && a.Kind == sim.ActMove {
		t := b.actionProgress(a)
		x0, y0 := b.hexToScreen(a.From)
		x1, y1 := b.hexToScreen(a.To)
		return x0 + (x1-x0)*t, y0 + (y1-y0)*t
	}
	return x, y
}

func (b *Battle) actionProgress(a *sim.Action) float64 {
	if a.End <= a.Start {
		return 1
	}
	t := (float64(b.w.Tick) + b.clock.Alpha(b.w) - float64(a.Start)) / float64(a.End-a.Start)
	return math.Max(0, math.Min(1, t))
}

// unitFacingAngle — угол взгляда (в шагах по 60°) с интерполяцией поворота.
func (b *Battle) unitFacingAngle(u *sim.Unit) float64 {
	if a := u.Action; a != nil && a.Kind == sim.ActTurn {
		t := b.actionProgress(a)
		steps := hex.SignedSteps(a.FromDir, a.Dir)
		return float64(a.FromDir) + float64(steps)*t
	}
	if a := u.Action; a != nil && a.Kind == sim.ActMove && a.FromDir != a.Dir && u.Facing == a.Dir {
		t := math.Min(1, b.actionProgress(a)*3)
		steps := hex.SignedSteps(a.FromDir, a.Dir)
		return float64(a.FromDir) + float64(steps)*t
	}
	return float64(u.Facing)
}

func (b *Battle) mapBounds() (minX, minY, maxX, maxY float64) {
	minX, minY = math.Inf(1), math.Inf(1)
	maxX, maxY = math.Inf(-1), math.Inf(-1)
	for _, c := range b.w.Map.Cells {
		x, y := hex.ToPixel(c.Hex, b.size())
		minX, minY = math.Min(minX, x), math.Min(minY, y)
		maxX, maxY = math.Max(maxX, x), math.Max(maxY, y)
	}
	return
}

func (b *Battle) centerOn(x, y float64) {
	b.camX = x - b.view.w/2
	b.camY = y - b.view.h/2
	b.clampCam()
}

func (b *Battle) partyCenter() (float64, float64) {
	us := b.w.AliveUnits(b.player)
	if len(us) == 0 {
		minX, minY, maxX, maxY := b.mapBounds()
		return (minX + maxX) / 2, (minY + maxY) / 2
	}
	sx, sy := 0.0, 0.0
	for _, u := range us {
		x, y := hex.ToPixel(u.Pos, b.size())
		sx += x
		sy += y
	}
	n := float64(len(us))
	return sx / n, sy / n
}

func (b *Battle) clampCam() {
	minX, minY, maxX, maxY := b.mapBounds()
	pad := b.size() * 3
	b.camX = math.Max(minX-pad-b.view.w*0.5, math.Min(b.camX, maxX+pad-b.view.w*0.5))
	b.camY = math.Max(minY-pad-b.view.h*0.5, math.Min(b.camY, maxY+pad-b.view.h*0.5))
}

func (b *Battle) setZoom(z float64, sx, sy float64) {
	z = math.Max(0.55, math.Min(2.2, z))
	if z == b.zoom {
		return
	}
	// сохраняем точку под курсором
	wx := (sx - b.view.x + b.camX) / b.size()
	wy := (sy - b.view.y + b.camY) / b.size()
	b.zoom = z
	b.camX = wx*b.size() - (sx - b.view.x)
	b.camY = wy*b.size() - (sy - b.view.y)
	b.clampCam()
}

// ---------------------------------------------------------------------------
// Выбор и режимы

func (b *Battle) selected() *sim.Unit {
	u := b.w.Unit(b.sel)
	if u == nil || u.Dead || u.Side != b.player {
		return nil
	}
	return u
}

func (b *Battle) modeName() string {
	switch b.mode {
	case ModeMove:
		s := "Движение: ЛКМ — точка маршрута, по врагу — атака (Ctrl — новый маршрут)"
		if b.keepFacing {
			s = "Движение БЕЗ ПОВОРОТА (K): ориентация сохраняется, шаги не вперёд дороже"
		}
		return s
	case ModeAttack:
		return "Атака — выберите противника (ЛКМ). ПКМ/Esc — отмена"
	case ModeSkill:
		sk := b.w.C.Skills[b.skill]
		if sk == nil {
			return "Умение"
		}
		switch sk.Targeting {
		case "unit":
			return fmt.Sprintf("Умение «%s» — выберите цель (ЛКМ). ПКМ/Esc — отмена", sk.Name)
		case "hex":
			return fmt.Sprintf("Умение «%s» — выберите клетку (ЛКМ). ПКМ/Esc — отмена", sk.Name)
		case "direction":
			return fmt.Sprintf("Умение «%s» — наведите мышь для выбора направления, ЛКМ — подтвердить", sk.Name)
		}
		return "Умение «" + sk.Name + "»"
	case ModeTurn:
		return "Поворот — наведите мышь на нужную сторону и нажмите ЛКМ (стоимость за каждые 60°)"
	case ModeInteract:
		return "Взаимодействие — выберите дверь или рычаг (ЛКМ)"
	}
	return ""
}

func (b *Battle) setMode(m Mode, skill string) {
	b.mode = m
	b.skill = skill
}

func (b *Battle) cancelMode() { b.setMode(ModeMove, "") }

// trySelect — выбор другого юнита с защитой незавершённой команды.
func (b *Battle) trySelect(id int) {
	if id == b.sel {
		return
	}
	if b.mode != ModeMove {
		what := "команда"
		switch b.mode {
		case ModeAttack:
			what = "атака (не выбрана цель)"
		case ModeSkill:
			if sk := b.w.C.Skills[b.skill]; sk != nil {
				what = fmt.Sprintf("умение «%s» (не выбрана цель)", sk.Name)
			}
		case ModeTurn:
			what = "поворот (не выбрано направление)"
		case ModeInteract:
			what = "взаимодействие (не выбран объект)"
		}
		b.pending = &pendingSwitch{to: id, what: what}
		return
	}
	b.sel = id
	b.queueScroll = 0
}

func (b *Battle) cycleUnit(dir int) {
	us := b.w.AliveUnits(b.player)
	if len(us) == 0 {
		return
	}
	idx := 0
	for i, u := range us {
		if u.ID == b.sel {
			idx = i
		}
	}
	idx = (idx + dir + len(us)) % len(us)
	b.trySelect(us[idx].ID)
	if b.pending == nil {
		x, y := hex.ToPixel(us[idx].Pos, b.size())
		b.centerOn(x, y)
	}
}

// cursorIdx — позиция вставки новых приказов в очередь юнита.
func (b *Battle) cursorIdx(u *sim.Unit) int {
	c, ok := b.cursor[u.ID]
	if !ok || c < 0 || c > len(u.Queue) {
		return len(u.Queue)
	}
	return c
}

// anchor — проектная позиция и ориентация юнита в точке вставки.
func (b *Battle) anchor(u *sim.Unit) (hex.Hex, hex.Dir, *sim.Plan) {
	plan := b.w.ProjectPlan(u)
	n := b.cursorIdx(u)
	if u.Current != nil {
		n++
	}
	if n == 0 {
		pos, f := u.Pos, u.Facing
		if a := u.Action; a != nil && a.Kind == sim.ActMove {
			pos = a.To
		}
		return pos, f, plan
	}
	if n > len(plan.Steps) {
		n = len(plan.Steps)
	}
	s := plan.Steps[n-1]
	return s.To, s.FacingTo, plan
}

func (b *Battle) addToast(s string, k colorKind) {
	b.toasts = append(b.toasts, toast{text: s, col: k, t: 4})
	if len(b.toasts) > 4 {
		b.toasts = b.toasts[len(b.toasts)-4:]
	}
}

// issue — выдача приказа выбранному юниту через командный интерфейс.
func (b *Battle) issue(spec sim.OrderSpec) bool {
	u := b.selected()
	if u == nil {
		return false
	}
	idx := b.cursorIdx(u)
	atEnd := idx == len(u.Queue)
	r := b.w.Apply(sim.Command{Kind: sim.CmdQueue, Side: b.player, Unit: u.ID, Index: idx, Order: &spec})
	if !r.OK() {
		b.addToast("Отклонено: "+r.Message, ckBad)
		return false
	}
	if !atEnd {
		b.cursor[u.ID] = idx + 1
	}
	switch {
	case r.Status == sim.ResWaiting:
		b.addToast(r.Message, ckWarn)
	case len(r.Warnings) > 0:
		b.addToast("Принято. "+r.Warnings[0].Text, ckWarn)
	}
	return true
}

func (b *Battle) command(c sim.Command) {
	c.Side = b.player
	r := b.w.Apply(c)
	if !r.OK() {
		b.addToast(r.Message, ckBad)
	} else if r.Message != "" {
		b.addToast(r.Message, ckInfo)
	}
}

// ---------------------------------------------------------------------------
// Обновление

func (b *Battle) Update(dt float64) error {
	b.time += dt
	b.layoutView()
	mx, my := ebiten.CursorPosition()
	b.mx, b.my = float64(mx), float64(my)
	b.hover = b.screenToHex(b.mx, b.my)
	b.hoverIn = b.view.contains(b.mx, b.my) && b.w.Map.InBounds(b.hover) && !b.overlayActive()
	b.buildButtons()

	if err := b.handleKeys(); err != nil {
		return err
	}
	b.handleMouse()

	// Время идёт, только если не открыто меню
	if !b.showMenu {
		b.clock.Advance(b.w, dt)
	}
	b.consumeEvents()
	b.vfx.update(dt)
	for i := range b.toasts {
		b.toasts[i].t -= dt
	}
	for len(b.toasts) > 0 && b.toasts[0].t <= 0 {
		b.toasts = b.toasts[1:]
	}
	if u := b.w.Unit(b.sel); u != nil && u.Dead {
		if us := b.w.AliveUnits(b.player); len(us) > 0 {
			b.sel = us[0].ID
		}
	}
	return nil
}

func (b *Battle) overlayActive() bool {
	return b.showMenu || b.showHelp || b.pending != nil
}

func ctrl() bool {
	return ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyMeta)
}

func shift() bool { return ebiten.IsKeyPressed(ebiten.KeyShift) }

func (b *Battle) handleKeys() error {
	jp := inpututil.IsKeyJustPressed
	if b.pending != nil {
		if jp(ebiten.KeyEscape) || jp(ebiten.KeyY) {
			b.resolvePending(true)
		} else if jp(ebiten.KeyEnter) || jp(ebiten.KeyN) {
			b.resolvePending(false)
		}
		return nil
	}
	if b.showHelp {
		if jp(ebiten.KeyEscape) || jp(ebiten.KeyF1) || jp(ebiten.KeyH) {
			b.showHelp = false
		}
		return nil
	}
	if b.showMenu {
		if jp(ebiten.KeyEscape) {
			b.showMenu = false
		}
		return nil
	}
	if jp(ebiten.KeyEscape) {
		if b.mode != ModeMove || b.showAutoPause {
			b.cancelMode()
			b.showAutoPause = false
		} else {
			b.showMenu = true
			b.clock.SetSpeed(0)
		}
	}
	if jp(ebiten.KeySpace) {
		b.togglePause()
	}
	if jp(ebiten.KeyEqual) || jp(ebiten.KeyKPAdd) {
		b.changeSpeed(1)
	}
	if jp(ebiten.KeyMinus) || jp(ebiten.KeyKPSubtract) {
		b.changeSpeed(-1)
	}
	if jp(ebiten.KeyF1) || jp(ebiten.KeyH) {
		b.showHelp = true
	}
	if jp(ebiten.KeyF5) {
		b.save()
	}
	if jp(ebiten.KeyF9) {
		b.load()
	}
	if jp(ebiten.KeyTab) {
		if shift() {
			b.cycleUnit(-1)
		} else {
			b.cycleUnit(1)
		}
	}
	if jp(ebiten.KeyL) {
		b.logDetails = !b.logDetails
	}
	if jp(ebiten.KeyF3) {
		b.showDiag = !b.showDiag
	}
	if jp(ebiten.KeyP) {
		mode := "strict"
		if b.w.QueueMode == "strict" {
			mode = "flexible"
		}
		b.command(sim.Command{Kind: sim.CmdSetQueueMode, Mode: mode})
		b.addToast("Резервирование ОД: "+queueModeName(b.w.QueueMode), ckInfo)
	}
	if jp(ebiten.KeyC) {
		if u := b.selected(); u != nil {
			x, y := hex.ToPixel(u.Pos, b.size())
			b.centerOn(x, y)
		}
	}
	// Камера стрелками
	pan := 12.0
	if ebiten.IsKeyPressed(ebiten.KeyArrowLeft) {
		b.camX -= pan
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowRight) {
		b.camX += pan
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowUp) {
		b.camY -= pan
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowDown) {
		b.camY += pan
	}
	b.clampCam()

	u := b.selected()
	if u == nil {
		return nil
	}
	if ctrl() && jp(ebiten.KeyZ) {
		b.command(sim.Command{Kind: sim.CmdUndo, Unit: u.ID})
		return nil
	}
	switch {
	case jp(ebiten.KeyM):
		b.cancelMode()
	case jp(ebiten.KeyA):
		b.setMode(ModeAttack, "")
	case jp(ebiten.KeyR):
		b.setMode(ModeTurn, "")
	case jp(ebiten.KeyF):
		b.setMode(ModeInteract, "")
	case jp(ebiten.KeyK):
		b.keepFacing = !b.keepFacing
		if b.keepFacing {
			b.addToast("Автоповорот при движении ВЫКЛЮЧЕН", ckInfo)
		} else {
			b.addToast("Автоповорот при движении включён", ckInfo)
		}
	case jp(ebiten.KeyG):
		b.issue(sim.OrderSpec{Type: sim.OrderGuard, Duration: b.w.C.Rules.GuardSeconds, React: true})
	case jp(ebiten.KeyZ) && !ctrl():
		dur := 1.0
		if shift() {
			dur = 3
		}
		b.issue(sim.OrderSpec{Type: sim.OrderWait, Duration: dur})
	case jp(ebiten.KeyX):
		b.issue(sim.OrderSpec{Type: sim.OrderStop})
	case jp(ebiten.KeyBackspace):
		b.deleteAtCursor(u)
	case jp(ebiten.KeyDelete):
		b.command(sim.Command{Kind: sim.CmdClear, Unit: u.ID})
		delete(b.cursor, u.ID)
	case jp(ebiten.KeyBracketLeft):
		b.moveCursor(u, -1)
	case jp(ebiten.KeyBracketRight):
		b.moveCursor(u, 1)
	case jp(ebiten.KeyPageUp):
		b.reorderAtCursor(u, -1)
	case jp(ebiten.KeyPageDown):
		b.reorderAtCursor(u, 1)
	case jp(ebiten.KeyEnd):
		delete(b.cursor, u.ID)
	}
	slots := []ebiten.Key{ebiten.Key1, ebiten.Key2, ebiten.Key3, ebiten.Key4, ebiten.Key5, ebiten.Key6}
	for i, k := range slots {
		if jp(k) && i < len(u.ClassDef().Skills) {
			b.activateSkill(u, u.ClassDef().Skills[i])
		}
	}
	return nil
}

func queueModeName(m string) string {
	if m == "strict" {
		return "строгое (очередь не больше текущих ОД)"
	}
	return "гибкое (очередь ждёт восстановления ОД)"
}

func (b *Battle) togglePause() {
	b.clock.TogglePause()
	if !b.clock.Paused() {
		b.showMenu = false
	}
}

func (b *Battle) changeSpeed(d int) {
	sp := b.clock.Speed
	if sp == 0 {
		sp = b.clock.LastSpeed
	}
	idx := 0
	for i, s := range sim.Speeds {
		if s == sp {
			idx = i
		}
	}
	idx = max(0, min(len(sim.Speeds)-1, idx+d))
	if b.clock.Paused() {
		b.clock.LastSpeed = sim.Speeds[idx]
	} else {
		b.clock.SetSpeed(sim.Speeds[idx])
	}
}

// activateSkill — выбор умения по горячей клавише или кнопке.
func (b *Battle) activateSkill(u *sim.Unit, id string) {
	sk := b.w.C.Skills[id]
	if sk == nil {
		return
	}
	switch sk.Targeting {
	case "self", "auto":
		b.issue(sim.OrderSpec{Type: sim.OrderSkill, Skill: id})
		b.cancelMode()
	default:
		b.setMode(ModeSkill, id)
	}
}

func (b *Battle) deleteAtCursor(u *sim.Unit) {
	if len(u.Queue) == 0 {
		if u.Current != nil {
			b.command(sim.Command{Kind: sim.CmdCancelCurrent, Unit: u.ID})
		}
		return
	}
	idx := b.cursorIdx(u) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(u.Queue) {
		idx = len(u.Queue) - 1
	}
	b.command(sim.Command{Kind: sim.CmdRemove, Unit: u.ID, OrderID: u.Queue[idx].ID})
	if c, ok := b.cursor[u.ID]; ok && c > 0 {
		b.cursor[u.ID] = c - 1
	}
}

func (b *Battle) moveCursor(u *sim.Unit, d int) {
	c := b.cursorIdx(u) + d
	if c < 0 {
		c = 0
	}
	if c >= len(u.Queue) {
		delete(b.cursor, u.ID)
		return
	}
	b.cursor[u.ID] = c
}

func (b *Battle) reorderAtCursor(u *sim.Unit, d int) {
	idx := b.cursorIdx(u) - 1
	if idx < 0 || idx >= len(u.Queue) {
		return
	}
	b.command(sim.Command{Kind: sim.CmdReorder, Unit: u.ID, OrderID: u.Queue[idx].ID, NewIndex: idx + d})
	if c, ok := b.cursor[u.ID]; ok {
		b.cursor[u.ID] = max(1, min(len(u.Queue), c+d))
	}
}

func (b *Battle) resolvePending(cancel bool) {
	p := b.pending
	b.pending = nil
	if p == nil {
		return
	}
	if cancel {
		b.cancelMode()
		b.sel = p.to
		b.addToast("Незавершённая команда отменена", ckInfo)
	}
}

// ---------------------------------------------------------------------------
// Мышь

func (b *Battle) handleMouse() {
	_, wy := ebiten.Wheel()
	if wy != 0 {
		switch {
		case b.view.contains(b.mx, b.my) && !b.overlayActive():
			b.setZoom(b.zoom*math.Pow(1.12, wy), b.mx, b.my)
		case b.logRect().contains(b.mx, b.my):
			b.logScroll = max(0, b.logScroll+int(wy*3))
		case b.mx > float64(b.g.W)-rightW:
			b.queueScroll = max(0, b.queueScroll-int(wy))
		}
	}
	b.hoverBtn = nil
	for i := len(b.buttons) - 1; i >= 0; i-- {
		if bt := b.buttons[i]; bt.r.contains(b.mx, b.my) {
			b.hoverBtn = bt
			break
		}
	}
	jp := inpututil.IsMouseButtonJustPressed
	jr := inpututil.IsMouseButtonJustReleased

	if jp(ebiten.MouseButtonLeft) {
		if b.hoverBtn != nil {
			if b.hoverBtn.enabled && b.hoverBtn.act != nil {
				b.hoverBtn.act()
			} else if !b.hoverBtn.enabled && b.hoverBtn.tip != "" {
				b.addToast(b.hoverBtn.tip, ckWarn)
			}
			return
		}
		if b.overlayActive() {
			return
		}
		if b.logRect().contains(b.mx, b.my) {
			b.clickLog()
			return
		}
		if r, _, ok := b.pauseBanner(); ok && r.contains(b.mx, b.my) {
			if b.clock.ReasonAt {
				x, y := hex.ToPixel(b.clock.ReasonHex, b.size())
				b.centerOn(x, y)
			}
			return
		}
		if b.hoverIn {
			b.lp = &press{x: b.mx, y: b.my, hex: b.hover, button: ebiten.MouseButtonLeft}
			if u := b.selected(); u != nil && b.mode == ModeMove {
				for _, o := range u.Queue {
					if o.Type == sim.OrderMove && o.Dest == b.hover {
						b.lp.orderID = o.ID
					}
				}
			}
		}
	}
	if (jp(ebiten.MouseButtonRight) || jp(ebiten.MouseButtonMiddle)) && b.view.contains(b.mx, b.my) && !b.overlayActive() {
		btn := ebiten.MouseButtonRight
		if jp(ebiten.MouseButtonMiddle) {
			btn = ebiten.MouseButtonMiddle
		}
		b.rp = &press{x: b.mx, y: b.my, button: btn, camX: b.camX, camY: b.camY}
	}
	if b.rp != nil {
		dx, dy := b.mx-b.rp.x, b.my-b.rp.y
		if math.Hypot(dx, dy) > 5 {
			b.rp.moved = true
		}
		if b.rp.moved {
			b.camX = b.rp.camX - dx
			b.camY = b.rp.camY - dy
			b.clampCam()
		}
		if jr(b.rp.button) {
			if !b.rp.moved && b.rp.button == ebiten.MouseButtonRight {
				b.rightClick()
			}
			b.rp = nil
		}
	}
	if b.lp != nil {
		if math.Hypot(b.mx-b.lp.x, b.my-b.lp.y) > 6 {
			b.lp.moved = true
		}
		if jr(ebiten.MouseButtonLeft) {
			p := b.lp
			b.lp = nil
			if p.orderID != 0 && p.moved && b.hoverIn && b.hover != p.hex {
				b.editWaypoint(p.orderID, b.hover)
			} else if !p.moved || p.hex == b.hover {
				b.leftClick(p.hex)
			}
		}
	}
}

// pauseBanner — баннер причины автопаузы.
func (b *Battle) pauseBanner() (rect, string, bool) {
	if !b.clock.Paused() || b.clock.Reason == "" {
		return rect{}, "", false
	}
	s := "II Автопауза: " + b.clock.Reason + " — Пробел: продолжить"
	if b.clock.ReasonAt {
		s += ", клик: показать"
	}
	w := measure(s, 15, true) + 30
	return rect{b.view.x + (b.view.w-w)/2, b.view.y + 66, w, 30}, s, true
}

func (b *Battle) rightClick() {
	if b.mode != ModeMove {
		b.cancelMode()
		return
	}
	if b.showAutoPause {
		b.showAutoPause = false
	}
}

func (b *Battle) editWaypoint(orderID int, to hex.Hex) {
	u := b.selected()
	if u == nil {
		return
	}
	for _, o := range u.Queue {
		if o.ID == orderID {
			spec := o.OrderSpec
			spec.Dest = to
			r := b.w.Apply(sim.Command{Kind: sim.CmdEdit, Side: b.player, Unit: u.ID, OrderID: orderID, Order: &spec})
			if !r.OK() {
				b.addToast("Точку нельзя перенести: "+r.Message, ckBad)
			} else {
				b.addToast("Промежуточная точка перенесена", ckGood)
			}
			return
		}
	}
}

func (b *Battle) visibleUnitAt(h hex.Hex) *sim.Unit {
	t := b.w.UnitAt(h)
	if t == nil || t.Dead || !b.w.IsVisibleTo(b.player, t) {
		return nil
	}
	return t
}

func (b *Battle) leftClick(h hex.Hex) {
	u := b.selected()
	t := b.visibleUnitAt(h)
	// Выбор своего юнита одним нажатием
	if t != nil && t.Side == b.player && (b.mode == ModeMove || b.mode == ModeAttack || b.mode == ModeTurn || b.mode == ModeInteract) {
		if u == nil || t.ID != u.ID {
			b.trySelect(t.ID)
			return
		}
	}
	if u == nil {
		return
	}
	switch b.mode {
	case ModeMove:
		// Клик по своей точке маршрута — выбрать её как точку вставки
		for i, o := range u.Queue {
			if o.Type == sim.OrderMove && o.Dest == h {
				b.cursor[u.ID] = i + 1
				if i+1 >= len(u.Queue) {
					delete(b.cursor, u.ID)
				}
				b.addToast(fmt.Sprintf("Точка вставки: после приказа №%d — новые действия добавятся в эту точку", b.orderNumber(u, o.ID)), ckInfo)
				return
			}
		}
		if t != nil && b.w.Hostile(b.player, t.Side) {
			b.issue(sim.OrderSpec{Type: sim.OrderAttack, TargetUnit: t.ID})
			return
		}
		if c := b.w.Map.Cell(h); c != nil && c.Feature != nil && (c.Feature.Kind == "lever" || c.IsClosedDoor()) {
			b.issue(sim.OrderSpec{Type: sim.OrderInteract, Dest: h})
			return
		}
		if ctrl() {
			b.command(sim.Command{Kind: sim.CmdClear, Unit: u.ID})
			delete(b.cursor, u.ID)
		}
		b.issue(sim.OrderSpec{Type: sim.OrderMove, Dest: h, KeepFacing: b.keepFacing || shift()})
	case ModeAttack:
		if t == nil || !b.w.Hostile(b.player, t.Side) {
			b.addToast("Выберите видимого противника", ckWarn)
			return
		}
		if b.issue(sim.OrderSpec{Type: sim.OrderAttack, TargetUnit: t.ID}) {
			b.cancelMode()
		}
	case ModeTurn:
		pos, _, _ := b.anchor(u)
		if d, ok := b.mouseDir(pos); ok && b.issue(sim.OrderSpec{Type: sim.OrderTurn, Dir: d}) {
			b.cancelMode()
		}
	case ModeInteract:
		if b.issue(sim.OrderSpec{Type: sim.OrderInteract, Dest: h}) {
			b.cancelMode()
		}
	case ModeSkill:
		sk := b.w.C.Skills[b.skill]
		if sk == nil {
			b.cancelMode()
			return
		}
		var ok bool
		switch sk.Targeting {
		case "unit":
			if t == nil {
				b.addToast("Выберите юнита-цель", ckWarn)
				return
			}
			ok = b.issue(sim.OrderSpec{Type: sim.OrderSkill, Skill: sk.ID, TargetUnit: t.ID})
		case "hex":
			ok = b.issue(sim.OrderSpec{Type: sim.OrderSkill, Skill: sk.ID, Dest: h})
		case "direction":
			pos, _, _ := b.anchor(u)
			if d, dok := b.mouseDir(pos); dok {
				ok = b.issue(sim.OrderSpec{Type: sim.OrderSkill, Skill: sk.ID, Dir: d})
			}
		}
		if ok {
			b.cancelMode()
		}
	}
}

// mouseDir — направление от клетки к курсору (одно из шести).
func (b *Battle) mouseDir(from hex.Hex) (hex.Dir, bool) {
	x, y := b.hexToScreen(from)
	dx, dy := b.mx-x, b.my-y
	if math.Hypot(dx, dy) < 4 {
		return 0, false
	}
	ang := math.Atan2(-dy, dx) * 180 / math.Pi
	if ang < 0 {
		ang += 360
	}
	return hex.Dir(int(math.Round(ang/60)) % 6), true
}

func (b *Battle) orderNumber(u *sim.Unit, id int) int {
	n := 1
	if u.Current != nil {
		if u.Current.ID == id {
			return 1
		}
		n = 2
	}
	for i, o := range u.Queue {
		if o.ID == id {
			return n + i
		}
	}
	return 0
}

// ---------------------------------------------------------------------------
// Сохранение

func (b *Battle) save() {
	data, err := b.w.Save(b.clock)
	if err == nil {
		err = os.WriteFile(b.g.SavePath, data, 0o644)
	}
	if err != nil {
		b.addToast("Ошибка сохранения: "+err.Error(), ckBad)
		return
	}
	b.addToast("Сохранено: "+b.g.SavePath, ckGood)
}

func (b *Battle) load() {
	raw, err := os.ReadFile(b.g.SavePath)
	if err != nil {
		b.addToast("Нет сохранения ("+b.g.SavePath+")", ckBad)
		return
	}
	w, clock, err := sim.Load(b.g.Content, raw)
	if err != nil {
		b.addToast("Ошибка загрузки: "+err.Error(), ckBad)
		return
	}
	nb, _ := newBattle(b.g, w.Scenario.ID, w)
	if clock != nil {
		clock.SetSpeed(0)
		clock.MaxTicks = 200
		nb.clock = clock
	}
	nb.toasts = nil
	nb.addToast("Загружено сохранение", ckGood)
	b.g.cur = nb
}

// ---------------------------------------------------------------------------
// События → журнал и визуальные эффекты

type logLine struct {
	seq     uint64
	tick    int
	text    string
	details []string
	kind    sim.EventType
}

func (b *Battle) consumeEvents() {
	evs := b.w.EventsSince(b.lastSeq)
	for i := range evs {
		e := &evs[i]
		b.lastSeq = e.Seq
		if !e.VisibleTo(b.player) {
			continue
		}
		// Визуальные эффекты — только для свежих событий (после загрузки или
		// при сильном отставании кадров не выводим «пачку» старых надписей).
		if b.w.Tick-e.Tick <= b.w.Ticks(1) {
			b.vfx.onEvent(b, e)
		}
		if e.Text != "" {
			b.logLines = append(b.logLines, logLine{seq: e.Seq, tick: e.Tick, text: e.Text, details: e.Details, kind: e.Type})
		}
	}
	if len(b.logLines) > 600 {
		b.logLines = b.logLines[len(b.logLines)-500:]
	}
}

// ---------------------------------------------------------------------------
// Кэши отображения

// visibleCells — клетки, которые видит отряд игрока (затемнение остальных).
func (b *Battle) visibleCells() map[hex.Hex]bool {
	key := [2]uint64{b.w.OccVersion, b.w.Map.Version}
	if b.visCells != nil && key == b.visKey {
		return b.visCells
	}
	vis := map[hex.Hex]bool{}
	for _, u := range b.w.AliveUnits(b.player) {
		sight := int(u.Stat("sight"))
		for _, h := range hex.Range(u.Pos, sight) {
			if vis[h] || !b.w.Map.InBounds(h) {
				continue
			}
			if b.w.Map.LineOfSight(u.Pos, h).Clear {
				vis[h] = true
			}
		}
	}
	b.visCells, b.visKey = vis, key
	return vis
}

// reachField — область доступности от точки вставки (кэшируется).
func (b *Battle) reachField(u *sim.Unit, from hex.Hex, facing hex.Dir) map[hex.Hex]gamemap.Reach {
	kf := uint64(0)
	if b.keepFacing {
		kf = 1 + uint64(facing)
	}
	key := [6]uint64{uint64(u.ID), uint64(u.PlanVer), b.w.OccVersion, b.w.Map.Version, uint64(from.Q+1000)<<16 | uint64(from.R+1000), kf}
	if b.reach != nil && key == b.reachKey {
		return b.reach
	}
	prof := b.w.MoveProfile(u, b.keepFacing, facing)
	b.reach = b.w.Map.Reachable(from, prof, func(h hex.Hex) bool { return b.w.Blocked(u, h) }, u.APMax())
	b.reachKey = key
	return b.reach
}

// skillSlots — умения юнита в порядке горячих клавиш.
func skillSlots(u *sim.Unit) []string { return u.ClassDef().Skills }

func sortedUnitsByID(us []*sim.Unit) []*sim.Unit {
	out := append([]*sim.Unit(nil), us...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
