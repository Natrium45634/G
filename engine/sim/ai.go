package sim

import (
	"sort"

	"hextactics/engine/data"
	"hextactics/engine/hex"
)

// ИИ — простой тактический контроллер. Он не имеет особых прав: выбирает действие
// и выдаёт приказ через тот же командный интерфейс (apply), что и игрок, поэтому
// для него действуют те же правила движения, ОД, попаданий и умений.
// Думает только бездействующий юнит (нет текущего приказа и очереди).

func (w *World) runAI() {
	for _, u := range w.sortedUnits() {
		if u.Dead || !w.AIControlled(u.Side) {
			continue
		}
		if inc, _ := w.Incapacitated(u); inc {
			continue
		}
		if u.Current != nil || len(u.Queue) > 0 || u.Action != nil {
			continue
		}
		w.aiThink(u)
	}
}

func (w *World) aiQueue(u *Unit, spec OrderSpec) bool {
	spec.Policy = PolicySkip
	r := w.apply(Command{Kind: CmdQueue, Side: u.Side, Unit: u.ID, Index: -1, Order: &spec})
	return r.OK()
}

// aiReady — умение готово к применению прямо сейчас (перезарядка и ресурсы; ОД подождём).
func (w *World) aiReady(u *Unit, id string) *data.SkillDef {
	sk := w.C.Skills[id]
	if sk == nil {
		return nil
	}
	if u.Cooldowns[id] > w.Tick {
		return nil
	}
	for r, amt := range sk.Costs {
		if u.Res[r] < int(amt*data.ResScale+0.5) {
			return nil
		}
	}
	return sk
}

func (w *World) aiSkills(u *Unit, hint string) []*data.SkillDef {
	var out []*data.SkillDef
	for _, id := range u.class.Skills {
		if sk := w.aiReady(u, id); sk != nil && sk.AIHint == hint {
			out = append(out, sk)
		}
	}
	return out
}

func (w *World) aiThink(u *Unit) {
	ai := u.class.AI
	if ai == "" || ai == "none" {
		return
	}
	enemies := w.VisibleEnemies(u.Side)
	sort.SliceStable(enemies, func(i, j int) bool {
		di, dj := hex.Distance(u.Pos, enemies[i].Pos), hex.Distance(u.Pos, enemies[j].Pos)
		if di != dj {
			return di < dj
		}
		if enemies[i].HP != enemies[j].HP {
			return enemies[i].HP < enemies[j].HP
		}
		return enemies[i].ID < enemies[j].ID
	})
	if len(enemies) == 0 {
		// Реакция на невидимого атакующего: развернуться к источнику угрозы.
		if t := u.Threat; t != nil && w.Tick-t.Tick < w.Ticks(4) {
			if d, ok := hex.DirectionTo(u.Pos, t.From); ok && hex.Delta(u.Facing, d) >= 2 {
				w.aiQueue(u, OrderSpec{Type: OrderTurn, Dir: d})
			}
			u.Threat = nil
		}
		return
	}
	target := enemies[0]

	if ai == "support" {
		if w.aiHeal(u) || w.aiBuff(u, enemies) || w.aiSummon(u, target) {
			return
		}
	}
	if ai == "caster" || ai == "support" || ai == "melee" || ai == "ranged" {
		if w.aiAoE(u, enemies) {
			return
		}
	}
	if ai == "caster" && w.aiHeal(u) {
		return
	}
	// Контроль (оглушение, яд) — по ближайшей цели
	for _, sk := range w.aiSkills(u, "control") {
		if sk.Targeting == "unit" {
			d := hex.Distance(u.Pos, target.Pos)
			if d <= sk.Range+1 && d >= sk.RangeMin {
				if w.aiQueue(u, OrderSpec{Type: OrderSkill, Skill: sk.ID, TargetUnit: target.ID}) {
					return
				}
			}
		}
	}
	if ai == "melee" {
		for _, sk := range w.aiSkills(u, "buff") {
			if hex.Distance(u.Pos, target.Pos) <= 3 && w.aiQueue(u, OrderSpec{Type: OrderSkill, Skill: sk.ID}) {
				return
			}
		}
	}
	basic := w.C.Skills[u.class.BasicAttack]
	if basic == nil {
		return
	}
	// Стрелок отступает, если враг слишком близко для обычной атаки
	if basic.RangeMin >= 2 && hex.Distance(u.Pos, target.Pos) < basic.RangeMin && !w.HasFlag(u, "rooted") {
		if dest, ok := w.aiRetreatHex(u, target); ok {
			w.aiQueue(u, OrderSpec{Type: OrderMove, Dest: dest})
			return
		}
	}
	// Предпочитаем цель, которую можно атаковать без подхода
	for _, e := range enemies {
		if w.inRangeLOS(u.Pos, e.Pos, basic) {
			target = e
			break
		}
	}
	w.aiQueue(u, OrderSpec{Type: OrderAttack, TargetUnit: target.ID})
}

func (w *World) aiHeal(u *Unit) bool {
	for _, sk := range w.aiSkills(u, "heal") {
		var best *Unit
		bestPct := 60
		for _, a := range w.AliveUnits(u.Side) {
			if !w.targetFilterOK(u, a, sk) {
				continue
			}
			pct := a.HP * 100 / max(1, a.HPMax())
			if pct < bestPct && hex.Distance(u.Pos, a.Pos) <= sk.Range+3 {
				best, bestPct = a, pct
			}
		}
		if best != nil && w.aiQueue(u, OrderSpec{Type: OrderSkill, Skill: sk.ID, TargetUnit: best.ID}) {
			return true
		}
	}
	return false
}

func (w *World) aiBuff(u *Unit, enemies []*Unit) bool {
	for _, sk := range w.aiSkills(u, "buff") {
		allies := 0
		for _, a := range w.AliveUnits(u.Side) {
			if hex.Distance(a.Pos, u.Pos) <= 2 {
				allies++
			}
		}
		if allies >= 2 && hex.Distance(u.Pos, enemies[0].Pos) <= 6 {
			if w.aiQueue(u, OrderSpec{Type: OrderSkill, Skill: sk.ID}) {
				return true
			}
		}
	}
	return false
}

func (w *World) aiSummon(u *Unit, target *Unit) bool {
	for _, sk := range w.aiSkills(u, "summon") {
		var best hex.Hex
		bestD := 1 << 30
		for _, h := range hex.Range(u.Pos, sk.Range) {
			if h == u.Pos || hex.Distance(u.Pos, h) < sk.RangeMin {
				continue
			}
			if ok, _ := w.Map.CanStand(h, &footStand); !ok || w.Blocked(nil, h) {
				continue
			}
			if d := hex.Distance(h, target.Pos); d < bestD {
				best, bestD = h, d
			}
		}
		if bestD < 1<<30 && w.aiQueue(u, OrderSpec{Type: OrderSkill, Skill: sk.ID, Dest: best}) {
			return true
		}
	}
	return false
}

// aiAoE — умение по области, если оно заденет достаточно врагов и не заденет союзников.
func (w *World) aiAoE(u *Unit, enemies []*Unit) bool {
	for _, sk := range w.aiSkills(u, "aoe") {
		need := max(1, sk.AIMinTargets)
		type cand struct {
			spec  OrderSpec
			score int
		}
		var best *cand
		eval := func(spec OrderSpec, tgt Target) {
			area := w.SkillArea(sk, u, w.dashPreview(u, u.Pos, sk, tgt), tgt)
			hits, friends := 0, 0
			for _, t := range w.UnitsInArea(u, sk, area) {
				if w.Hostile(u.Side, t.Side) {
					hits++
				} else {
					friends++
				}
			}
			if hits >= need && friends == 0 && (best == nil || hits > best.score) {
				best = &cand{spec: spec, score: hits}
			}
		}
		switch sk.Targeting {
		case "direction":
			for d := hex.Dir(0); d < 6; d++ {
				eval(OrderSpec{Type: OrderSkill, Skill: sk.ID, Dir: d}, Target{Hex: u.Pos.Neighbor(d), Dir: d})
			}
		case "hex":
			for _, e := range enemies {
				if !w.inRangeLOS(u.Pos, e.Pos, sk) {
					continue
				}
				d, _ := hex.DirectionTo(u.Pos, e.Pos)
				eval(OrderSpec{Type: OrderSkill, Skill: sk.ID, Dest: e.Pos}, Target{Hex: e.Pos, Dir: d})
			}
		}
		if best != nil && w.aiQueue(u, best.spec) {
			return true
		}
	}
	return false
}

func (w *World) aiRetreatHex(u *Unit, from *Unit) (hex.Hex, bool) {
	prof := w.MoveProfile(u, false, u.Facing)
	field := w.Map.Reachable(u.Pos, prof, w.blockerFor(u), data.AP(3))
	var best hex.Hex
	bestScore := -1
	keys := make([]hex.Hex, 0, len(field))
	for h := range field {
		keys = append(keys, h)
	}
	hex.SortHexes(keys, u.Pos)
	for _, h := range keys {
		d := hex.Distance(h, from.Pos)
		if d < 2 {
			continue
		}
		score := d*10 - field[h].Cost/data.APScale
		if score > bestScore {
			best, bestScore = h, score
		}
	}
	return best, bestScore >= 0
}
