package sim

import (
	"fmt"
	"sort"

	"hextactics/engine/data"
	"hextactics/engine/hex"
)

// updateUnit продвигает текущее действие и приказы юнита на один тик.
// Условия начала/продолжения каждого приказа проверяются в момент исполнения
// по текущему состоянию мира.
func (w *World) updateUnit(u *Unit) {
	if inc, code := w.Incapacitated(u); inc {
		// Оглушённый/спящий не действует; таймеры ожидания и восстановления продолжают идти.
		if u.Action != nil && (u.Action.Kind == ActRecover || u.Action.Kind == ActWait) {
			w.progressAction(u)
		}
		if u.Current == nil && u.Action == nil && len(u.Queue) > 0 {
			w.startNextOrder(u)
		}
		if u.Current != nil && u.Action == nil {
			w.waitFor(u, u.Current, issue(code, ""))
		}
		return
	}
	if u.Action != nil {
		w.progressAction(u)
		if u.Action != nil || u.Dead {
			return
		}
	}
	// Несколько итераций: приказ может завершиться мгновенно (например, поворот на 0°),
	// тогда следующий начинается в том же тике.
	for guard := 0; guard < 8 && u.Action == nil && !u.Dead; guard++ {
		if u.Current == nil && !w.startNextOrder(u) {
			return
		}
		if !w.driveOrder(u) {
			return
		}
	}
}

// startNextOrder берёт следующий приказ из очереди.
func (w *World) startNextOrder(u *Unit) bool {
	if len(u.Queue) == 0 {
		return false
	}
	o := u.Queue[0]
	u.Queue = u.Queue[1:]
	o.Status = StatusActive
	o.StartedAt = w.Tick
	o.Issue = nil
	u.Current = o
	u.PlanVer++
	w.emit(Event{Type: EvOrderStarted, Unit: u.ID, Order: o.ID, Hex: u.Pos})
	return true
}

func (w *World) driveOrder(u *Unit) bool {
	o := u.Current
	switch o.Type {
	case OrderMove:
		return w.driveMove(u, o)
	case OrderTurn:
		return w.driveTurn(u, o)
	case OrderAttack, OrderSkill:
		return w.driveSkill(u, o)
	case OrderInteract:
		return w.driveInteract(u, o)
	case OrderWait:
		if o.Phase == 0 {
			o.Phase = 1
			w.setActive(o)
			t := w.Ticks(o.Duration)
			if t <= 0 {
				w.completeOrder(u, o, "")
				return true
			}
			u.Action = &Action{Kind: ActWait, Start: w.Tick, End: w.Tick + t}
			return false
		}
		w.completeOrder(u, o, "")
		return true
	case OrderGuard:
		return w.driveGuard(u, o)
	case OrderStop:
		w.completeOrder(u, o, "")
		for _, q := range u.Queue {
			q.Status = StatusCancelled
		}
		u.Queue = nil
		return false
	}
	return w.failOrder(u, o, issue(ErrInvalidOrder, string(o.Type)))
}

// ---------------------------------------------------------------------------
// Статусы и политики ошибок

func (w *World) setActive(o *Order) {
	if o.Status == StatusWaiting {
		o.Status = StatusActive
	}
	o.Issue = nil
	o.WaitSince = 0
}

// completeOrder — приказ выполнен.
func (w *World) completeOrder(u *Unit, o *Order, note string) {
	o.Status = StatusDone
	o.Issue = nil
	if u.Current == o {
		u.Current = nil
	}
	w.removeEffectsWhere(u, func(e *ActiveEffect) bool { return e.Order == o.ID }, "")
	u.PlanVer++
	text := ""
	if note != "" {
		text = fmt.Sprintf("%s: %s", u.Name, note)
	}
	ev := w.emit(Event{Type: EvOrderCompleted, Unit: u.ID, Order: o.ID, Hex: u.Pos, Text: text})
	ev.Visible = 1 << uint(u.Side)
	if len(u.Queue) == 0 && u.Current == nil {
		w.emit(Event{Type: EvQueueFinished, Unit: u.ID, Hex: u.Pos})
	}
}

// waitFor — ожидание условий, которые разрешатся сами (ОД, перезарядка, ресурс, оглушение).
func (w *World) waitFor(u *Unit, o *Order, is Issue) bool {
	changed := o.Status != StatusWaiting || o.Issue == nil || o.Issue.Code != is.Code
	o.Status = StatusWaiting
	o.Issue = &is
	if changed {
		u.PlanVer++
		w.emit(Event{Type: EvOrderWaiting, Unit: u.ID, Order: o.ID, Code: is.Code, Hex: u.Pos})
	}
	return false
}

// blockedWait — обработка блокирующего условия согласно политике приказа.
// abort/skip — немедленная неудача; wait/repath — ожидание с тайм-аутом, затем пропуск.
func (w *World) blockedWait(u *Unit, o *Order, is Issue) bool {
	switch o.Policy {
	case PolicyWait, PolicyRepath:
		if o.WaitSince == 0 {
			o.WaitSince = w.Tick
		}
		if w.Tick-o.WaitSince >= w.Ticks(w.C.Rules.WaitTimeout) {
			t := issue(ErrWaitTimeout, is.Text)
			return w.endFailed(u, o, t, false)
		}
		changed := o.Status != StatusWaiting || o.Issue == nil || o.Issue.Code != is.Code
		o.Status = StatusWaiting
		o.Issue = &is
		if changed {
			u.PlanVer++
			w.emit(Event{Type: EvOrderWaiting, Unit: u.ID, Order: o.ID, Code: is.Code, Hex: u.Pos})
		}
		return false
	}
	return w.failOrder(u, o, is)
}

// failOrder — неудача приказа по его политике (abort прерывает всю очередь).
func (w *World) failOrder(u *Unit, o *Order, is Issue) bool {
	return w.endFailed(u, o, is, o.Policy == PolicyAbort)
}

func (w *World) endFailed(u *Unit, o *Order, is Issue, abort bool) bool {
	o.Status = StatusFailed
	if is.Code == ErrInterrupted {
		o.Status = StatusInterrupted
	}
	o.Issue = &is
	if u.Current == o {
		u.Current = nil
	}
	w.removeEffectsWhere(u, func(e *ActiveEffect) bool { return e.Order == o.ID }, "")
	u.LastIssue = &is
	text := fmt.Sprintf("%s: приказ «%s» не выполнен — %s", u.Name, w.Describe(o), is.Text)
	if abort && len(u.Queue) > 0 {
		for _, q := range u.Queue {
			q.Status = StatusCancelled
		}
		text += fmt.Sprintf(". Очередь прервана (отменено приказов: %d)", len(u.Queue))
		u.Queue = nil
	} else if len(u.Queue) > 0 {
		text += ". Переход к следующему приказу"
	}
	u.PlanVer++
	ev := w.emit(Event{Type: EvOrderFailed, Unit: u.ID, Order: o.ID, Code: is.Code, Hex: u.Pos, Text: text})
	ev.Visible = 1 << uint(u.Side) // приказы — внутренняя информация стороны
	if len(u.Queue) == 0 && u.Current == nil {
		w.emit(Event{Type: EvQueueFinished, Unit: u.ID, Hex: u.Pos})
	}
	return true
}

// ---------------------------------------------------------------------------
// Действия

func (w *World) progressAction(u *Unit) {
	a := u.Action
	o := u.Current
	switch a.Kind {
	case ActMove:
		if !a.Switched && w.Tick >= a.Start+(a.End-a.Start+1)/2 {
			a.Switched = true
			delete(w.occ, a.From)
			delete(w.resv, a.To)
			w.occ[a.To] = u.ID
			w.resv[a.From] = u.ID
			u.Pos = a.To
			w.OccVersion++
			w.visDirty = true
			w.emit(Event{Type: EvUnitMoved, Unit: u.ID, From: a.From, Hex: a.To})
			w.checkTraps(u)
			if u.Dead {
				return
			}
		}
		if w.Tick >= a.End {
			if w.resv[a.From] == u.ID {
				delete(w.resv, a.From)
			}
			u.Action = nil
			w.OccVersion++
			if o != nil && o.Type == OrderMove {
				o.PathIdx++
			}
			if o != nil && o.Cancel {
				w.cancelOrder(u, o)
			}
		}
	case ActTurn:
		if w.Tick >= a.End {
			u.Facing = a.Dir
			u.Action = nil
			w.emit(Event{Type: EvUnitTurned, Unit: u.ID, Hex: u.Pos, Amount: int(a.Dir)})
		}
	case ActPrep:
		if w.Tick >= a.End {
			u.Action = nil
			w.finishPrep(u, a)
		}
	case ActChannel:
		sk := w.C.Skills[a.Skill]
		if sk != nil && sk.Channel != nil && w.Tick >= a.NextTick && w.Tick <= a.End {
			a.NextTick += max(1, w.Ticks(sk.Channel.Interval))
			w.applyOps(sk.Channel.Ops, &OpContext{Source: u, Self: u, Target: u, Label: sk.Name, Skill: sk.ID})
		}
		if !u.Dead && u.Action == a && w.Tick >= a.End {
			u.Action = nil
			w.endChannelEffect(u, sk)
			if o != nil {
				w.completeOrder(u, o, "")
			}
		}
	case ActRecover:
		if w.Tick >= a.End {
			u.Action = nil
			if o != nil && o.Phase >= 2 {
				w.completeOrder(u, o, "")
			}
		}
	case ActWait:
		if w.Tick >= a.End {
			u.Action = nil
			if o != nil && o.Type == OrderWait {
				w.completeOrder(u, o, "")
			}
		}
	case ActInteract:
		if w.Tick >= a.End {
			u.Action = nil
			w.finishInteract(u, a)
		}
	}
}

func (w *World) endChannelEffect(u *Unit, sk *data.SkillDef) {
	if sk != nil && sk.Channel != nil && sk.Channel.Effect != "" {
		w.removeEffectsWhere(u, func(e *ActiveEffect) bool { return e.Def == sk.Channel.Effect && e.Remaining < 0 }, "")
	}
}

// interrupt прерывает подготовку или каналирование.
func (w *World) interrupt(u *Unit, reason string) {
	a := u.Action
	if a == nil || (a.Kind != ActPrep && a.Kind != ActChannel) {
		return
	}
	u.Action = nil
	sk := w.C.Skills[a.Skill]
	name := a.Skill
	if sk != nil {
		name = sk.Name
		w.endChannelEffect(u, sk)
	}
	w.emit(Event{Type: EvActionInterrupted, Unit: u.ID, Skill: a.Skill, Hex: u.Pos,
		Text: fmt.Sprintf("%s: «%s» прервано (%s)", u.Name, name, reason)})
	if o := u.Current; o != nil {
		if a.Kind == ActChannel {
			w.completeOrder(u, o, "")
		} else {
			w.failOrder(u, o, issue(ErrInterrupted, reason))
		}
	}
}

// onIncapacitated — юнит оглушён или уснул: текущее действие прекращается.
func (w *World) onIncapacitated(u *Unit) {
	a := u.Action
	if a == nil {
		return
	}
	switch a.Kind {
	case ActMove:
		if a.Switched {
			if w.resv[a.From] == u.ID {
				delete(w.resv, a.From)
			}
			if o := u.Current; o != nil && o.Type == OrderMove {
				o.PathIdx++
			}
		} else if w.resv[a.To] == u.ID {
			delete(w.resv, a.To)
		}
		u.Action = nil
		w.OccVersion++
	case ActTurn, ActInteract:
		u.Action = nil
	case ActPrep, ActChannel:
		w.interrupt(u, "потеря контроля")
	}
}

// cancelOrder — отмена приказа игроком.
func (w *World) cancelOrder(u *Unit, o *Order) {
	o.Status = StatusCancelled
	if u.Current == o {
		u.Current = nil
	}
	w.removeEffectsWhere(u, func(e *ActiveEffect) bool { return e.Order == o.ID }, "")
	u.PlanVer++
	w.emit(Event{Type: EvOrderCancelled, Unit: u.ID, Order: o.ID, Hex: u.Pos})
}

// ---------------------------------------------------------------------------
// Движение и поворот

func (w *World) stepAPCost(u *Unit, from, to hex.Hex, keepFacing bool, facing hex.Dir) (int, Code) {
	prof := w.MoveProfile(u, keepFacing, facing)
	c, code := w.Map.StepCost(from, to, prof)
	if code != "" {
		return 0, codeFromMap(code)
	}
	if !keepFacing && !w.C.Rules.TurnFreeWhileMoving {
		if d, ok := hex.DirectionTo(from, to); ok {
			c += hex.Delta(facing, d) * data.AP(u.stats.Get("turn_cost"))
		}
	}
	return c, CodeOK
}

func (w *World) startMoveStep(u *Unit, next hex.Hex, cost int, keepFacing bool) {
	u.AP -= cost
	dir, _ := hex.DirectionTo(u.Pos, next)
	fromDir := u.Facing
	if !keepFacing {
		u.Facing = dir
	}
	pct := 100
	if c := w.Map.Cell(next); c != nil {
		if p, ok := w.MoveProfile(u, false, 0).TerrainPct(c.Terrain); ok {
			pct = p
		}
	}
	dur := u.stats.Get("move_time") * float64(pct) / 100
	if keepFacing && dir != u.Facing {
		dur = dur * float64(w.C.Rules.StrafeTimePct) / 100
	}
	ticks := max(1, w.Ticks(dur))
	u.Action = &Action{Kind: ActMove, Start: w.Tick, End: w.Tick + ticks, From: u.Pos, To: next, FromDir: fromDir, Dir: dir, Cost: cost}
	w.resv[next] = u.ID
	w.OccVersion++
}

func (w *World) startTurn(u *Unit, to hex.Dir, cost int) {
	steps := hex.Delta(u.Facing, to)
	u.AP -= cost
	ticks := max(1, w.Ticks(float64(steps)*u.stats.Get("turn_time")))
	u.Action = &Action{Kind: ActTurn, Start: w.Tick, End: w.Tick + ticks, FromDir: u.Facing, Dir: to, Cost: cost, From: u.Pos, To: u.Pos}
}

// TurnCost — стоимость поворота в милли-ОД.
func (w *World) TurnCost(u *Unit, from, to hex.Dir) int {
	return hex.Delta(from, to) * data.AP(u.stats.Get("turn_cost"))
}

func (w *World) driveMove(u *Unit, o *Order) bool {
	if w.HasFlag(u, "rooted") {
		return w.waitFor(u, o, issue(ErrRooted, ""))
	}
	prof := w.MoveProfile(u, o.KeepFacing, u.Facing)
	if o.PathIdx >= len(o.Path) {
		if u.Pos == o.Dest {
			w.completeOrder(u, o, "")
			return true
		}
		res := w.Map.FindPath(u.Pos, o.Dest, prof, w.blockerFor(u))
		if !res.OK {
			return w.blockedWait(u, o, issue(codeFromMap(res.Code), ""))
		}
		o.Path, o.PathIdx = res.Path, 0
		u.PlanVer++
	}
	next := o.Path[o.PathIdx]
	_, code := w.stepAPCost(u, u.Pos, next, o.KeepFacing, u.Facing)
	if w.Blocked(u, next) || code != CodeOK {
		reason := issue(ErrPathBlocked, "")
		if code != CodeOK {
			reason = issue(code, "")
		}
		if o.Policy == PolicyRepath {
			res := w.Map.FindPath(u.Pos, o.Dest, prof, w.blockerFor(u))
			if res.OK && len(res.Path) > 0 {
				o.Path, o.PathIdx = res.Path, 0
				next = o.Path[0]
				u.PlanVer++
				if !o.Repathed {
					o.Repathed = true
					w.emit(Event{Type: EvInfo, Unit: u.ID, Hex: u.Pos, Text: fmt.Sprintf("%s: путь перекрыт — маршрут перестроен", u.Name)})
				}
			} else {
				return w.blockedWait(u, o, reason)
			}
		} else {
			return w.blockedWait(u, o, reason)
		}
	}
	cost, code := w.stepAPCost(u, u.Pos, next, o.KeepFacing, u.Facing)
	if code != CodeOK {
		return w.blockedWait(u, o, issue(code, ""))
	}
	if u.AP < cost {
		return w.waitFor(u, o, issue(ErrInsufficientAP, fmt.Sprintf("нужно %.2f", data.APf(cost))))
	}
	w.setActive(o)
	w.startMoveStep(u, next, cost, o.KeepFacing)
	return false
}

func (w *World) driveTurn(u *Unit, o *Order) bool {
	if hex.Delta(u.Facing, o.Dir) == 0 {
		w.completeOrder(u, o, "")
		return true
	}
	cost := w.TurnCost(u, u.Facing, o.Dir)
	if u.AP < cost {
		return w.waitFor(u, o, issue(ErrInsufficientAP, fmt.Sprintf("нужно %.2f", data.APf(cost))))
	}
	w.setActive(o)
	w.startTurn(u, o.Dir, cost)
	return false
}

// approachStep делает один шаг к ближайшей клетке, удовлетворяющей goal.
func (w *World) approachStep(u *Unit, o *Order, goal func(hex.Hex) bool, why Issue) bool {
	if w.HasFlag(u, "rooted") {
		return w.waitFor(u, o, issue(ErrRooted, why.Text))
	}
	prof := w.MoveProfile(u, false, u.Facing)
	res := w.Map.FindPathToAny(u.Pos, goal, prof, w.blockerFor(u), -1)
	if !res.OK || len(res.Path) == 0 {
		return w.blockedWait(u, o, issue(ErrNoPath, why.Text))
	}
	cost, code := w.stepAPCost(u, u.Pos, res.Path[0], false, u.Facing)
	if code != CodeOK {
		return w.blockedWait(u, o, issue(code, ""))
	}
	if u.AP < cost {
		return w.waitFor(u, o, issue(ErrInsufficientAP, fmt.Sprintf("нужно %.2f", data.APf(cost))))
	}
	w.setActive(o)
	w.startMoveStep(u, res.Path[0], cost, false)
	return false
}

// ---------------------------------------------------------------------------
// Атака и умения

func (w *World) driveSkill(u *Unit, o *Order) bool {
	sk, code := w.orderSkill(u, o)
	if code != CodeOK {
		return w.failOrder(u, o, issue(code, o.Skill))
	}
	if o.Phase > 0 {
		// Подготовка/восстановление закончились без перехода — завершаем.
		w.completeOrder(u, o, "")
		return true
	}
	tgt, is, goal := w.resolveTarget(u, o, sk, u.Pos)
	if is != nil {
		switch is.Code {
		case ErrTargetNotVisible, ErrNoTargets, ErrOccupied:
			return w.blockedWait(u, o, *is)
		}
		return w.failOrder(u, o, *is)
	}
	if goal != nil {
		why := issue(ErrOutOfRange, "")
		if o.NoApproach || sk.NoApproach || sk.Targeting == "auto" {
			return w.blockedWait(u, o, why)
		}
		return w.approachStep(u, o, goal, why)
	}
	if sk.RequiresFacing {
		if want, need := w.requiredFacing(u.Facing, u.Pos, sk, tgt); need {
			if !w.C.Rules.AutoTurnToAttack {
				return w.failOrder(u, o, issue(ErrWrongFacing, ""))
			}
			cost := w.TurnCost(u, u.Facing, want)
			if u.AP < cost {
				return w.waitFor(u, o, issue(ErrInsufficientAP, fmt.Sprintf("поворот: нужно %.2f", data.APf(cost))))
			}
			w.setActive(o)
			w.startTurn(u, want, cost)
			return false
		}
	}
	if is := w.readiness(u, sk); is != nil {
		return w.waitFor(u, o, *is)
	}
	w.setActive(o)
	w.paySkill(u, sk)
	prep := w.actionTicks(u, sk.Prep)
	a := &Action{Kind: ActPrep, Start: w.Tick, End: w.Tick + prep, Skill: sk.ID, Target: tgt,
		Interruptible: sk.Interruptible, Cost: data.AP(sk.APCost), From: u.Pos, To: u.Pos}
	u.Action = a
	o.Phase = 1
	text := ""
	if prep >= w.Ticks(0.5) {
		text = fmt.Sprintf("%s готовит «%s» (%.1f с)", u.Name, sk.Name, w.C.Seconds(prep))
	}
	area := w.SkillArea(sk, u, w.dashPreview(u, u.Pos, sk, tgt), tgt)
	w.emit(Event{Type: EvSkillPrepStarted, Unit: u.ID, Other: tgt.Unit, Skill: sk.ID, Hex: tgt.Hex,
		Area: area, Dangerous: sk.Dangerous, Text: text, Amount: prep})
	if prep == 0 {
		u.Action = nil
		w.finishPrep(u, a)
		return u.Current != o && u.Action == nil
	}
	return false
}

func (w *World) actionTicks(u *Unit, sec float64) int {
	if sec <= 0 {
		return 0
	}
	spd := u.stats.Get("action_speed")
	if spd <= 0 {
		spd = 100
	}
	return max(1, w.Ticks(sec*100/spd))
}

// readiness — готовность умения: перезарядка, ресурсы, ОД (ожидаемые условия).
func (w *World) readiness(u *Unit, sk *data.SkillDef) *Issue {
	if ready := u.Cooldowns[sk.ID]; ready > w.Tick {
		is := issue(ErrOnCooldown, fmt.Sprintf("ещё %.1f с", w.C.Seconds(ready-w.Tick)))
		return &is
	}
	for _, r := range sortedKeys(sk.Costs) {
		need := int(sk.Costs[r]*data.ResScale + 0.5)
		if u.Res[r] < need {
			name := r
			if rd := w.C.Resources[r]; rd != nil {
				name = rd.Name
			}
			is := issue(ErrInsufficientResource, fmt.Sprintf("%s %.0f/%.0f", name, float64(u.Res[r])/data.ResScale, sk.Costs[r]))
			return &is
		}
	}
	if cost := data.AP(sk.APCost); u.AP < cost {
		is := issue(ErrInsufficientAP, fmt.Sprintf("нужно %.2f", data.APf(cost)))
		return &is
	}
	return nil
}

func (w *World) paySkill(u *Unit, sk *data.SkillDef) {
	u.AP -= data.AP(sk.APCost)
	for _, r := range sortedKeys(sk.Costs) {
		u.Res[r] -= int(sk.Costs[r]*data.ResScale + 0.5)
	}
}

func sortedKeys(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// finishPrep — окончание подготовки: повторная проверка цели и применение умения.
func (w *World) finishPrep(u *Unit, a *Action) {
	o := u.Current
	sk := w.C.Skills[a.Skill]
	if sk == nil {
		return
	}
	tgt := a.Target
	if tgt.Unit != 0 && sk.Targeting != "self" {
		t := w.Unit(tgt.Unit)
		if t == nil || t.Dead {
			w.fizzle(u, o, sk, issue(ErrTargetDead, ""))
			return
		}
		if !w.inRangeLOS(u.Pos, t.Pos, sk) {
			w.fizzle(u, o, sk, issue(ErrTargetMoved, t.Name))
			return
		}
		tgt.Hex = t.Pos
	}
	if sk.TargetFilter == "empty_hex" && w.Blocked(u, tgt.Hex) {
		w.fizzle(u, o, sk, issue(ErrOccupied, ""))
		return
	}
	w.resolveSkill(u, sk, tgt)
	if u.Dead {
		return
	}
	u.Cooldowns[sk.ID] = w.Tick + w.Ticks(sk.Cooldown)
	if o == nil {
		return
	}
	o.Phase = 2
	if sk.Channel != nil {
		if sk.Channel.Effect != "" {
			w.applyEffectEx(u, sk.Channel.Effect, 0, u.ID, nil, true, 0)
		}
		u.Action = &Action{Kind: ActChannel, Start: w.Tick, End: w.Tick + w.Ticks(sk.Channel.Duration),
			NextTick: w.Tick + max(1, w.Ticks(sk.Channel.Interval)), Skill: sk.ID, Target: tgt,
			Interruptible: sk.Interruptible, From: u.Pos, To: u.Pos}
		return
	}
	if rec := w.actionTicks(u, sk.Recovery); rec > 0 {
		u.Action = &Action{Kind: ActRecover, Start: w.Tick, End: w.Tick + rec, Skill: sk.ID, From: u.Pos, To: u.Pos}
		return
	}
	w.completeOrder(u, o, "")
}

// fizzle — умение не сработало после подготовки (ресурсы уже потрачены).
func (w *World) fizzle(u *Unit, o *Order, sk *data.SkillDef, is Issue) {
	w.emit(Event{Type: EvInfo, Unit: u.ID, Skill: sk.ID, Hex: u.Pos,
		Text: fmt.Sprintf("%s: «%s» не достигло цели — %s", u.Name, sk.Name, is.Text)})
	if o != nil {
		w.failOrder(u, o, is)
	}
}

// ---------------------------------------------------------------------------
// Защитная стойка и взаимодействие

func (w *World) driveGuard(u *Unit, o *Order) bool {
	if o.Phase == 0 {
		o.Phase = 1
		o.StartedAt = w.Tick
		w.setActive(o)
		if ge := w.C.Rules.GuardEffect; ge != "" {
			w.applyEffectEx(u, ge, 0, u.ID, nil, true, o.ID)
		}
		w.emit(Event{Type: EvInfo, Unit: u.ID, Hex: u.Pos, Text: fmt.Sprintf("%s занимает защитную стойку", u.Name)})
	}
	if o.React {
		if sk := w.C.Skills[u.class.BasicAttack]; sk != nil && w.readiness(u, sk) == nil {
			if t := w.reactionTarget(u, sk); t != nil {
				o.Status = StatusDone
				w.removeEffectsWhere(u, func(e *ActiveEffect) bool { return e.Order == o.ID }, "")
				w.emit(Event{Type: EvOrderCompleted, Unit: u.ID, Order: o.ID, Hex: u.Pos,
					Text: fmt.Sprintf("%s: противник %s вошёл в зону — атака", u.Name, t.Name)})
				na := &Order{ID: w.NextOrderID, OrderSpec: OrderSpec{Type: OrderAttack, TargetUnit: t.ID, NoApproach: true, Policy: PolicySkip}, Status: StatusActive, StartedAt: w.Tick}
				w.NextOrderID++
				u.Current = na
				u.PlanVer++
				w.emit(Event{Type: EvOrderStarted, Unit: u.ID, Order: na.ID, Hex: u.Pos})
				return true
			}
		}
	}
	if o.Duration > 0 && w.Tick-o.StartedAt >= w.Ticks(o.Duration) {
		w.completeOrder(u, o, "")
		return true
	}
	return false
}

// reactionTarget — ближайший видимый враг в зоне обычной атаки и в секторе атаки.
func (w *World) reactionTarget(u *Unit, sk *data.SkillDef) *Unit {
	var best *Unit
	bestD := 1 << 30
	for _, t := range w.AliveUnits(-1) {
		if !w.Hostile(u.Side, t.Side) || !w.IsVisibleTo(u.Side, t) {
			continue
		}
		if !w.inRangeLOS(u.Pos, t.Pos, sk) {
			continue
		}
		if sk.RequiresFacing {
			if _, need := w.requiredFacing(u.Facing, u.Pos, sk, Target{Unit: t.ID, Hex: t.Pos}); need {
				continue
			}
		}
		if d := hex.Distance(u.Pos, t.Pos); d < bestD {
			best, bestD = t, d
		}
	}
	return best
}

func (w *World) driveInteract(u *Unit, o *Order) bool {
	c := w.Map.Cell(o.Dest)
	if c == nil || c.Feature == nil {
		return w.failOrder(u, o, issue(ErrNoInteractable, ""))
	}
	d := hex.Distance(u.Pos, o.Dest)
	if d == 0 && c.Feature.Kind == "door" {
		return w.failOrder(u, o, issue(ErrCannotCloseOccupied, ""))
	}
	if d > 1 {
		dest := o.Dest
		prof := w.MoveProfile(u, false, u.Facing)
		goal := func(h hex.Hex) bool {
			ok, _ := w.Map.CanStand(h, prof)
			return ok && hex.Distance(h, dest) == 1 && !w.Blocked(u, h)
		}
		return w.approachStep(u, o, goal, issue(ErrOutOfRange, "нужно подойти вплотную"))
	}
	cost := data.AP(w.C.Rules.InteractCostAP)
	if u.AP < cost {
		return w.waitFor(u, o, issue(ErrInsufficientAP, fmt.Sprintf("нужно %.2f", data.APf(cost))))
	}
	w.setActive(o)
	u.AP -= cost
	u.Action = &Action{Kind: ActInteract, Start: w.Tick, End: w.Tick + max(1, w.Ticks(w.C.Rules.InteractTime)),
		Target: Target{Hex: o.Dest}, Cost: cost, From: u.Pos, To: u.Pos}
	return false
}

func (w *World) finishInteract(u *Unit, a *Action) {
	o := u.Current
	c := w.Map.Cell(a.Target.Hex)
	if c == nil || c.Feature == nil {
		if o != nil {
			w.failOrder(u, o, issue(ErrNoInteractable, ""))
		}
		return
	}
	var doors []*FeatureRef
	if c.Feature.Kind == "lever" {
		for _, id := range c.Feature.Links {
			if dc := w.Map.FeatureCell(id); dc != nil {
				doors = append(doors, &FeatureRef{ID: id, Hex: dc.Hex, Open: dc.Feature.Open})
			}
		}
		c.Feature.Open = !c.Feature.Open
	} else {
		doors = append(doors, &FeatureRef{ID: c.Feature.ID, Hex: c.Hex, Open: c.Feature.Open})
	}
	changed := false
	for _, d := range doors {
		if d.Open && w.UnitAt(d.Hex) != nil {
			w.emit(Event{Type: EvInfo, Unit: u.ID, Hex: d.Hex, Text: fmt.Sprintf("%s: %s", u.Name, Message(ErrCannotCloseOccupied))})
			continue
		}
		if w.Map.SetDoor(d.ID, !d.Open) {
			changed = true
			verb := "открыта"
			if d.Open {
				verb = "закрыта"
			}
			w.emit(Event{Type: EvMapChanged, Unit: u.ID, Hex: d.Hex, Text: fmt.Sprintf("%s: дверь %s", u.Name, verb)})
		}
	}
	if changed {
		w.visDirty = true
		w.OccVersion++
	}
	if o != nil {
		w.completeOrder(u, o, "")
	}
}

// FeatureRef — ссылка на элемент карты.
type FeatureRef struct {
	ID   string
	Hex  hex.Hex
	Open bool
}
