package sim

import (
	"fmt"

	"hextactics/engine/data"
	"hextactics/engine/gamemap"
	"hextactics/engine/hex"
)

// orderSkill — определение умения для приказа (Attack — обычная атака класса).
func (w *World) orderSkill(u *Unit, o *Order) (*data.SkillDef, Code) {
	if o.Type == OrderAttack {
		if u.class.BasicAttack == "" {
			return nil, ErrNoBasicAttack
		}
		return w.C.Skills[u.class.BasicAttack], CodeOK
	}
	sk := w.C.Skills[o.Skill]
	if sk == nil {
		return nil, ErrUnknownSkill
	}
	if !w.UnitHasSkill(u, sk.ID) {
		return nil, ErrSkillNotAvailable
	}
	return sk, CodeOK
}

// UnitHasSkill — владеет ли юнит умением.
func (w *World) UnitHasSkill(u *Unit, id string) bool {
	if u.class.BasicAttack == id {
		return true
	}
	for _, s := range u.class.Skills {
		if s == id {
			return true
		}
	}
	return false
}

// inRangeLOS — цель в дальности умения и (если требуется) в линии видимости.
func (w *World) inRangeLOS(from, to hex.Hex, sk *data.SkillDef) bool {
	d := hex.Distance(from, to)
	if d < sk.RangeMin || d > sk.Range {
		return false
	}
	if sk.LOS && d > 1 && !w.Map.LineOfSight(from, to).Clear {
		return false
	}
	return true
}

// RangeIssue — объяснение, почему цель недоступна с позиции from (nil — доступна).
func (w *World) RangeIssue(from, to hex.Hex, sk *data.SkillDef) *Issue {
	d := hex.Distance(from, to)
	if d > sk.Range {
		is := issue(ErrOutOfRange, fmt.Sprintf("%d > %d", d, sk.Range))
		return &is
	}
	if d < sk.RangeMin {
		is := issue(ErrTooClose, fmt.Sprintf("%d < %d", d, sk.RangeMin))
		return &is
	}
	if sk.LOS && d > 1 {
		if los := w.Map.LineOfSight(from, to); !los.Clear {
			c := w.Map.Cell(los.Blocker)
			what := "препятствие"
			if c != nil {
				what = c.Terrain.Name
				if c.IsClosedDoor() {
					what = "закрытая дверь"
				}
			}
			is := issue(ErrNoLOS, what)
			return &is
		}
	}
	return nil
}

// targetFilterOK — подходит ли юнит t как цель умения для u.
func (w *World) targetFilterOK(u, t *Unit, sk *data.SkillDef) bool {
	switch sk.TargetFilter {
	case "enemy":
		return w.Hostile(u.Side, t.Side)
	case "ally":
		return t.Side == u.Side && t.ID != u.ID
	case "ally_or_self":
		return t.Side == u.Side
	}
	return true
}

// resolveTarget определяет цель приказа в текущем состоянии мира.
// Если цель вне дальности/видимости с позиции from, возвращается goal — условие
// клетки, с которой умение можно применить (для подхода).
func (w *World) resolveTarget(u *Unit, o *Order, sk *data.SkillDef, from hex.Hex) (Target, *Issue, func(hex.Hex) bool) {
	fail := func(c Code, d string) (Target, *Issue, func(hex.Hex) bool) {
		is := issue(c, d)
		return Target{}, &is, nil
	}
	prof := w.MoveProfile(u, false, u.Facing)
	approach := func(to hex.Hex) func(hex.Hex) bool {
		return func(h hex.Hex) bool {
			if h != u.Pos {
				if ok, _ := w.Map.CanStand(h, prof); !ok || w.Blocked(u, h) {
					return false
				}
			}
			return w.inRangeLOS(h, to, sk)
		}
	}
	switch sk.Targeting {
	case "self":
		return Target{Unit: u.ID, Hex: from, Dir: u.Facing}, nil, nil
	case "unit":
		t := w.Unit(o.TargetUnit)
		if t == nil {
			return fail(ErrTargetInvalid, "")
		}
		if t.Dead {
			return fail(ErrTargetDead, t.Name)
		}
		if !w.targetFilterOK(u, t, sk) {
			return fail(ErrTargetInvalid, t.Name)
		}
		if w.Hostile(u.Side, t.Side) && !w.IsVisibleTo(u.Side, t) {
			return fail(ErrTargetNotVisible, t.Name)
		}
		tg := Target{Unit: t.ID, Hex: t.Pos}
		tg.Dir, _ = hex.DirectionTo(from, t.Pos)
		if !w.inRangeLOS(from, t.Pos, sk) {
			return tg, nil, approach(t.Pos)
		}
		return tg, nil, nil
	case "hex":
		if !w.Map.InBounds(o.Dest) {
			return fail(ErrOutOfBounds, "")
		}
		if sk.TargetFilter == "empty_hex" {
			if ok, code := w.Map.CanStand(o.Dest, &footStand); !ok {
				return fail(codeFromMap(code), "")
			}
			if w.Blocked(u, o.Dest) {
				return fail(ErrOccupied, "")
			}
		}
		tg := Target{Hex: o.Dest}
		tg.Dir, _ = hex.DirectionTo(from, o.Dest)
		if !w.inRangeLOS(from, o.Dest, sk) {
			return tg, nil, approach(o.Dest)
		}
		return tg, nil, nil
	case "direction":
		return Target{Hex: from.Neighbor(o.Dir), Dir: o.Dir}, nil, nil
	case "auto":
		t := w.autoTarget(u, sk, from)
		if t == nil {
			return fail(ErrNoTargets, fmt.Sprintf("в радиусе %d", sk.Range))
		}
		tg := Target{Unit: t.ID, Hex: t.Pos}
		tg.Dir, _ = hex.DirectionTo(from, t.Pos)
		return tg, nil, nil
	}
	return fail(ErrInvalidOrder, sk.Targeting)
}

// autoTarget — ближайший видимый подходящий враг в дальности и видимости.
func (w *World) autoTarget(u *Unit, sk *data.SkillDef, from hex.Hex) *Unit {
	var best *Unit
	bestD := 1 << 30
	for _, t := range w.AliveUnits(-1) {
		if t.ID == u.ID || !w.targetFilterOK(u, t, sk) {
			continue
		}
		if w.Hostile(u.Side, t.Side) && !w.IsVisibleTo(u.Side, t) {
			continue
		}
		if !w.inRangeLOS(from, t.Pos, sk) {
			continue
		}
		if d := hex.Distance(from, t.Pos); d < bestD {
			best, bestD = t, d
		}
	}
	return best
}

// requiredFacing — нужен ли поворот перед применением и к какому направлению.
// Для направленных умений нужна точная ориентация; для остальных цель должна быть
// в секторе атаки (Delta ≤ attack_arc_max_delta).
func (w *World) requiredFacing(facing hex.Dir, pos hex.Hex, sk *data.SkillDef, tgt Target) (hex.Dir, bool) {
	if sk.Targeting == "direction" {
		return tgt.Dir, facing != tgt.Dir
	}
	if tgt.Hex == pos {
		return facing, false
	}
	p, alt, tie, ok := hex.DirectionsTo(pos, tgt.Hex)
	if !ok {
		return facing, false
	}
	arc := w.C.Rules.AttackArcMaxDelta
	if hex.Delta(facing, p) <= arc || (tie && hex.Delta(facing, alt) <= arc) {
		return facing, false
	}
	want := p
	if tie && hex.Delta(facing, alt) < hex.Delta(facing, p) {
		want = alt
	}
	return want, true
}

// dashPreview — позиция после рывка (для умений по маршруту), без изменения мира.
func (w *World) dashPreview(u *Unit, from hex.Hex, sk *data.SkillDef, tgt Target) hex.Hex {
	if sk.Dash <= 0 || w.HasFlag(u, "rooted") {
		return from
	}
	prof := w.MoveProfile(u, false, tgt.Dir)
	cur := from
	for i := 0; i < sk.Dash; i++ {
		next := cur.Neighbor(tgt.Dir)
		if _, code := w.Map.StepCost(cur, next, prof); code != "" || w.Blocked(u, next) {
			break
		}
		cur = next
	}
	return cur
}

// SkillArea — клетки области поражения умения из точки origin.
func (w *World) SkillArea(sk *data.SkillDef, u *Unit, origin hex.Hex, tgt Target) []hex.Hex {
	size := sk.Shape.Size
	inMap := func(h hex.Hex) bool { c := w.Map.Cell(h); return c != nil && !c.BlocksLOS() }
	var out []hex.Hex
	switch sk.Shape.Type {
	case "self":
		out = []hex.Hex{origin}
	case "single":
		out = []hex.Hex{tgt.Hex}
	case "line":
		for _, h := range hex.LineDir(origin, tgt.Dir, size) {
			c := w.Map.Cell(h)
			if c == nil || (sk.Shape.StopAtWall && c.BlocksLOS()) {
				break
			}
			out = append(out, h)
		}
	case "cone":
		for _, h := range hex.Cone(origin, tgt.Dir, size, sk.Shape.HalfAngle) {
			if inMap(h) && w.Map.LineOfSight(origin, h).Clear {
				out = append(out, h)
			}
		}
	case "radius":
		for _, h := range hex.Range(tgt.Hex, size) {
			if inMap(h) && (h == tgt.Hex || w.Map.LineOfSight(tgt.Hex, h).Clear) {
				out = append(out, h)
			}
		}
	case "arc":
		for _, h := range hex.Arc(origin, tgt.Dir) {
			if inMap(h) {
				out = append(out, h)
			}
		}
	}
	return out
}

// UnitsInArea — юниты в области с учётом фильтра affects (порядок: порядок клеток области).
func (w *World) UnitsInArea(u *Unit, sk *data.SkillDef, area []hex.Hex) []*Unit {
	var out []*Unit
	seen := map[int]bool{}
	for _, h := range area {
		t := w.UnitAt(h)
		if t == nil || t.Dead || seen[t.ID] {
			continue
		}
		ok := false
		switch sk.Affects {
		case "enemies":
			ok = w.Hostile(u.Side, t.Side)
		case "allies":
			ok = t.Side == u.Side
		case "self":
			ok = t.ID == u.ID
		default: // all
			ok = t.ID != u.ID
		}
		if ok {
			seen[t.ID] = true
			out = append(out, t)
		}
	}
	return out
}

func isHexOp(op string) bool { return op == "place_object" || op == "summon" }

// resolveSkill применяет умение: рывок, область, атаки или операции.
func (w *World) resolveSkill(u *Unit, sk *data.SkillDef, tgt Target) {
	origin := u.Pos
	if sk.Targeting == "direction" || sk.Shape.Type == "line" || sk.Shape.Type == "cone" || sk.Shape.Type == "arc" {
		if sk.Targeting != "direction" {
			if d, ok := hex.DirectionTo(u.Pos, tgt.Hex); ok {
				tgt.Dir = d
			}
		}
	}
	if sk.Dash > 0 {
		origin = w.doDash(u, tgt.Dir, sk.Dash)
		if sk.Targeting == "direction" {
			tgt.Hex = origin.Neighbor(tgt.Dir)
		}
	}
	area := w.SkillArea(sk, u, origin, tgt)
	w.emit(Event{Type: EvSkillResolved, Unit: u.ID, Other: tgt.Unit, Skill: sk.ID, Hex: tgt.Hex, From: origin, Area: area, Kind: sk.Kind})

	var hexOps, unitOps []data.OpDef
	for _, op := range sk.Ops {
		if isHexOp(op.Op) {
			hexOps = append(hexOps, op)
		} else {
			unitOps = append(unitOps, op)
		}
	}
	if len(hexOps) > 0 {
		w.applyOps(hexOps, &OpContext{Source: u, Self: u, Hex: tgt.Hex, Label: sk.Name, Skill: sk.ID})
	}
	if len(unitOps) == 0 {
		return
	}
	targets := w.UnitsInArea(u, sk, area)
	if sk.Shape.Type == "self" {
		targets = []*Unit{u}
	}
	if len(targets) == 0 {
		w.emit(Event{Type: EvInfo, Unit: u.ID, Skill: sk.ID, Hex: tgt.Hex,
			Text: fmt.Sprintf("%s: «%s» — в области никого нет", u.Name, sk.Name)})
		return
	}
	for _, t := range targets {
		if u.Dead {
			return
		}
		if t.Dead {
			continue
		}
		if sk.Attack {
			w.performAttack(u, t, sk)
		} else {
			w.applyOps(unitOps, &OpContext{Source: u, Self: u, Target: t, Label: sk.Name, Skill: sk.ID, Hex: t.Pos})
		}
	}
}

// doDash — рывок по прямой до n клеток (останавливается перед препятствием или юнитом).
func (w *World) doDash(u *Unit, dir hex.Dir, n int) hex.Hex {
	end := w.dashPreview(u, u.Pos, &data.SkillDef{Dash: n}, Target{Dir: dir})
	if end == u.Pos {
		u.Facing = dir
		return u.Pos
	}
	from := u.Pos
	delete(w.occ, from)
	w.occ[end] = u.ID
	u.Pos = end
	u.Facing = dir
	w.OccVersion++
	w.visDirty = true
	w.emit(Event{Type: EvUnitDashed, Unit: u.ID, From: from, Hex: end,
		Text: fmt.Sprintf("%s совершает рывок на %d кл.", u.Name, hex.Distance(from, end))})
	w.checkTraps(u)
	return u.Pos
}

// footStand — профиль для проверки «свободной клетки» у умений (ловушки, призыв).
var footStand = gamemap.MoveProfile{Class: "foot", StepCost: data.APScale, MaxClimb: 9}
