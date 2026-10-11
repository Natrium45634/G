package sim

import (
	"fmt"
	"strings"

	"hextactics/engine/data"
	"hextactics/engine/gamemap"
	"hextactics/engine/hex"
)

// PlanStep — проекция одного приказа очереди: где он исполнится, сколько стоит,
// какие предупреждения есть. Используется интерфейсом (линии, номера, стоимость)
// и проверкой строгого резервирования ОД.
type PlanStep struct {
	OrderID    int
	Type       OrderType
	Text       string
	Active     bool
	Status     OrderStatus
	Policy     FailPolicy
	From       hex.Hex
	To         hex.Hex
	FacingFrom hex.Dir
	FacingTo   hex.Dir
	Path       []hex.Hex // перемещение (включая подход к цели)
	MoveCost   int
	TurnCost   int
	ActCost    int
	Cost       int // итог шага, милли-ОД
	Cum        int // нарастающий итог
	ResCost    map[string]int
	Area       []hex.Hex
	Target     hex.Hex
	HasTarget  bool
	TargetUnit int
	Skill      string
	Issues     []Issue // предупреждения (приказ выполнится, но с оговорками)
	Error      *Issue  // приказ в текущих условиях не выполним
	NeedsWait  bool    // к моменту шага ОД не хватит — ожидание восстановления
}

// Plan — проекция всей очереди юнита.
type Plan struct {
	Steps     []PlanStep
	Total     int // требуемые ОД на всю очередь (милли), включая остаток текущего приказа
	APNow     int
	Free      int // APNow - Total (может быть отрицательным)
	Shortfall int
	WaitSec   float64
	EndPos    hex.Hex
	EndFacing hex.Dir
	Errors    int
}

type planCacheEntry struct {
	key   [4]uint64
	steps []PlanStep
	end   hex.Hex
	endF  hex.Dir
}

// ProjectPlan рассчитывает проекцию очереди. Тяжёлая часть (пути) кэшируется до
// изменения очереди, карты или расположения юнитов.
func (w *World) ProjectPlan(u *Unit) *Plan {
	key := [4]uint64{uint64(u.PlanVer), w.Map.Version, w.OccVersion, uint64(len(u.Queue))}
	ce := w.planCache[u.ID]
	if ce == nil || ce.key != key {
		steps, end, endF := w.projectSteps(u)
		ce = &planCacheEntry{key: key, steps: steps, end: end, endF: endF}
		w.planCache[u.ID] = ce
	}
	p := &Plan{Steps: make([]PlanStep, len(ce.steps)), APNow: u.AP, EndPos: ce.end, EndFacing: ce.endF}
	copy(p.Steps, ce.steps)
	for i := range p.Steps {
		s := &p.Steps[i]
		if s.Error != nil {
			p.Errors++
		}
		s.NeedsWait = s.Cum > u.AP
	}
	if n := len(p.Steps); n > 0 {
		p.Total = p.Steps[n-1].Cum
	}
	p.Free = u.AP - p.Total
	if p.Free < 0 {
		p.Shortfall = -p.Free
		if w.C.Rules.APModel == "continuous" {
			if regen := u.stats.Get("ap_regen"); regen > 0 {
				p.WaitSec = data.APf(p.Shortfall) / regen
			}
		} else {
			p.WaitSec = w.C.Rules.CycleSeconds
		}
	}
	return p
}

// AnchorAt — позиция и ориентация юнита после исполнения первых n приказов плана
// (n = len(steps) — конец очереди). Используется как точка для новых действий.
func (p *Plan) AnchorAt(n int, u *Unit) (hex.Hex, hex.Dir) {
	if n <= 0 || len(p.Steps) == 0 {
		if n <= 0 && len(p.Steps) > 0 {
			return p.Steps[0].From, p.Steps[0].FacingFrom
		}
		return p.EndPos, p.EndFacing
	}
	if n > len(p.Steps) {
		n = len(p.Steps)
	}
	s := p.Steps[n-1]
	return s.To, s.FacingTo
}

func (w *World) projectSteps(u *Unit) ([]PlanStep, hex.Hex, hex.Dir) {
	pos, facing := u.Pos, u.Facing
	if a := u.Action; a != nil {
		switch a.Kind {
		case ActMove:
			pos = a.To
			facing = u.Facing
		case ActTurn:
			facing = a.Dir
		}
	}
	var steps []PlanStep
	resCum := map[string]int{}
	cum := 0
	usedSkill := map[string]bool{}
	project := func(o *Order, active bool) {
		s := w.projectOrder(u, o, active, pos, facing, resCum, usedSkill)
		cum += s.Cost
		s.Cum = cum
		steps = append(steps, s)
		pos, facing = s.To, s.FacingTo
	}
	if u.Current != nil {
		project(u.Current, true)
	}
	locked := false
	for _, o := range u.Queue {
		if locked {
			s := PlanStep{OrderID: o.ID, Type: o.Type, Text: w.Describe(o), From: pos, To: pos, FacingFrom: facing, FacingTo: facing, Status: o.Status, Policy: o.Policy}
			e := issue(ErrQueueLocked, "")
			s.Error = &e
			s.Cum = cum
			steps = append(steps, s)
			continue
		}
		project(o, false)
		if o.Type == OrderStop {
			locked = true
		}
	}
	return steps, pos, facing
}

func (w *World) projectOrder(u *Unit, o *Order, active bool, pos hex.Hex, facing hex.Dir, resCum map[string]int, used map[string]bool) PlanStep {
	s := PlanStep{OrderID: o.ID, Type: o.Type, Text: w.Describe(o), Active: active, Status: o.Status, Policy: o.Policy,
		From: pos, To: pos, FacingFrom: facing, FacingTo: facing}
	errf := func(c Code, d string) PlanStep {
		e := issue(c, d)
		s.Error = &e
		return s
	}
	warn := func(c Code, d string) { s.Issues = append(s.Issues, issue(c, d)) }
	if active && o.Status == StatusWaiting && o.Issue != nil {
		s.Issues = append(s.Issues, *o.Issue)
	}
	blocker := w.blockerFor(u)
	moveAlong := func(path []hex.Hex, keep bool) {
		cur := pos
		f := facing
		for _, h := range path {
			c, _ := w.stepAPCost(u, cur, h, keep, f)
			s.MoveCost += c
			if !keep {
				if d, ok := hex.DirectionTo(cur, h); ok {
					f = d
				}
			}
			cur = h
		}
		s.Path = append(s.Path, path...)
		pos = cur
		facing = f
	}
	findPath := func(dest hex.Hex, keep bool) (gamemap.PathResult, bool) {
		prof := w.MoveProfile(u, keep, facing)
		res := w.Map.FindPath(pos, dest, prof, blocker)
		if res.OK {
			return res, true
		}
		if res.Code == gamemap.CodeOccupied || res.Code == gamemap.CodeNoPath {
			res2 := w.Map.FindPath(pos, dest, prof, nil)
			if res2.OK {
				if res.Code == gamemap.CodeOccupied {
					warn(ErrOccupied, "клетка сейчас занята")
				} else {
					warn(ErrPathBlocked, "путь сейчас перекрыт юнитами")
				}
				return res2, true
			}
		}
		return res, false
	}

	switch o.Type {
	case OrderMove:
		if active && u.Action != nil && u.Action.Kind == ActMove && u.Action.To == o.Dest {
			break
		}
		if pos == o.Dest {
			break
		}
		res, ok := findPath(o.Dest, o.KeepFacing)
		if !ok {
			return errf(codeFromMap(res.Code), "")
		}
		moveAlong(res.Path, o.KeepFacing)
	case OrderTurn:
		s.TurnCost = w.TurnCost(u, facing, o.Dir)
		facing = o.Dir
	case OrderAttack, OrderSkill:
		sk, code := w.orderSkill(u, o)
		if code != CodeOK {
			return errf(code, o.Skill)
		}
		s.Skill = sk.ID
		if active && o.Phase > 0 {
			// уже оплачено и подготавливается/восстанавливается
			if a := u.Action; a != nil && a.Skill == sk.ID {
				s.Target, s.HasTarget, s.TargetUnit = a.Target.Hex, true, a.Target.Unit
				s.Area = w.SkillArea(sk, u, w.dashPreview(u, pos, sk, a.Target), a.Target)
			}
			break
		}
		var tgt Target
		switch sk.Targeting {
		case "self":
			tgt = Target{Unit: u.ID, Hex: pos}
		case "unit":
			t := w.Unit(o.TargetUnit)
			if t == nil {
				return errf(ErrTargetInvalid, "")
			}
			if t.Dead {
				return errf(ErrTargetDead, t.Name)
			}
			if !w.targetFilterOK(u, t, sk) {
				return errf(ErrTargetInvalid, t.Name)
			}
			if w.Hostile(u.Side, t.Side) && !w.IsVisibleTo(u.Side, t) {
				warn(ErrTargetNotVisible, t.Name)
			}
			tgt = Target{Unit: t.ID, Hex: t.Pos}
		case "hex":
			if !w.Map.InBounds(o.Dest) {
				return errf(ErrOutOfBounds, "")
			}
			tgt = Target{Hex: o.Dest}
			if sk.TargetFilter == "empty_hex" {
				if ok, c := w.Map.CanStand(o.Dest, &footStand); !ok {
					return errf(codeFromMap(c), "")
				}
				if w.Blocked(u, o.Dest) {
					warn(ErrOccupied, "клетка сейчас занята")
				}
			}
		case "direction":
			tgt = Target{Hex: pos.Neighbor(o.Dir), Dir: o.Dir}
		case "auto":
			if t := w.autoTarget(u, sk, pos); t != nil {
				tgt = Target{Unit: t.ID, Hex: t.Pos}
			} else {
				warn(ErrNoTargets, "сейчас целей нет — будет ждать")
				tgt = Target{Hex: pos.Neighbor(facing), Dir: facing}
			}
		}
		if sk.Targeting != "direction" {
			tgt.Dir, _ = hex.DirectionTo(pos, tgt.Hex)
		}
		if (sk.Targeting == "unit" || sk.Targeting == "hex") && !w.inRangeLOS(pos, tgt.Hex, sk) {
			if o.NoApproach || sk.NoApproach {
				if ri := w.RangeIssue(pos, tgt.Hex, sk); ri != nil {
					warn(ri.Code, "подход отключён — будет ждать")
				}
			} else {
				prof := w.MoveProfile(u, false, facing)
				tg := tgt.Hex
				goal := func(h hex.Hex) bool {
					if h != u.Pos {
						if ok, _ := w.Map.CanStand(h, prof); !ok || w.Blocked(u, h) {
							return false
						}
					}
					return w.inRangeLOS(h, tg, sk)
				}
				res := w.Map.FindPathToAny(pos, goal, prof, blocker, -1)
				if !res.OK {
					ri := w.RangeIssue(pos, tgt.Hex, sk)
					if ri == nil {
						return errf(ErrNoPath, "")
					}
					return errf(ri.Code, "нет пути к позиции для атаки")
				}
				moveAlong(res.Path, false)
				warn(CodeOK, fmt.Sprintf("подход: %d кл.", len(res.Path)))
				if sk.Targeting != "direction" {
					tgt.Dir, _ = hex.DirectionTo(pos, tgt.Hex)
				}
			}
		}
		if sk.RequiresFacing {
			if want, need := w.requiredFacing(facing, pos, sk, tgt); need {
				s.TurnCost = w.TurnCost(u, facing, want)
				facing = want
				warn(CodeOK, fmt.Sprintf("разворот → %s (+%.2f ОД)", want.Name(), data.APf(s.TurnCost)))
			}
		}
		s.ActCost = data.AP(sk.APCost)
		for _, r := range sortedKeys(sk.Costs) {
			need := int(sk.Costs[r]*data.ResScale + 0.5)
			if s.ResCost == nil {
				s.ResCost = map[string]int{}
			}
			s.ResCost[r] = need
			resCum[r] += need
			if resCum[r] > u.Res[r] {
				name := r
				if rd := w.C.Resources[r]; rd != nil {
					name = rd.Name
				}
				warn(ErrInsufficientResource, fmt.Sprintf("%s: нужно %.0f, есть %.0f — ожидание", name, float64(resCum[r])/data.ResScale, float64(u.Res[r])/data.ResScale))
			}
		}
		if ready := u.Cooldowns[sk.ID]; ready > w.Tick {
			warn(ErrOnCooldown, fmt.Sprintf("ещё %.1f с", w.C.Seconds(ready-w.Tick)))
		} else if used[sk.ID] && sk.Cooldown > 0 {
			warn(ErrOnCooldown, fmt.Sprintf("повторное применение — перезарядка %.0f с", sk.Cooldown))
		}
		used[sk.ID] = true
		origin := w.dashPreview(u, pos, sk, tgt)
		if sk.Dash > 0 && origin != pos {
			s.Path = append(s.Path, hex.Line(pos, origin)[1:]...)
			pos = origin
			if sk.Targeting == "direction" {
				tgt.Hex = pos.Neighbor(tgt.Dir)
			}
		}
		s.Area = w.SkillArea(sk, u, origin, tgt)
		s.Target, s.HasTarget, s.TargetUnit = tgt.Hex, true, tgt.Unit
		if sk.Affects == "all" {
			var names []string
			for _, t := range w.UnitsInArea(u, sk, s.Area) {
				if t.Side == u.Side {
					names = append(names, t.Name)
				}
			}
			if len(names) > 0 {
				warn(ErrFriendlyFire, strings.Join(names, ", "))
			}
		}
	case OrderInteract:
		c := w.Map.Cell(o.Dest)
		if c == nil || c.Feature == nil {
			return errf(ErrNoInteractable, "")
		}
		s.Target, s.HasTarget = o.Dest, true
		if hex.Distance(pos, o.Dest) > 1 {
			prof := w.MoveProfile(u, false, facing)
			dest := o.Dest
			goal := func(h hex.Hex) bool {
				ok, _ := w.Map.CanStand(h, prof)
				return ok && hex.Distance(h, dest) == 1 && (h == u.Pos || !w.Blocked(u, h))
			}
			res := w.Map.FindPathToAny(pos, goal, prof, blocker, -1)
			if !res.OK {
				return errf(ErrNoPath, "к элементу не подойти")
			}
			moveAlong(res.Path, false)
		} else if hex.Distance(pos, o.Dest) == 0 && c.Feature.Kind == "door" {
			return errf(ErrCannotCloseOccupied, "")
		}
		if !(active && u.Action != nil && u.Action.Kind == ActInteract) {
			s.ActCost = data.AP(w.C.Rules.InteractCostAP)
		}
	case OrderWait, OrderGuard, OrderStop:
	}
	s.Cost = s.MoveCost + s.TurnCost + s.ActCost
	s.To, s.FacingTo = pos, facing
	return s
}
