package sim

import (
	"fmt"

	"hextactics/engine/data"
	"hextactics/engine/hex"
)

// dispatch рассылает новые события пассивным навыкам (реактивные навыки подключены
// к общей системе событий). Порождённые реакциями события обрабатываются в том же
// тике в порядке появления; общее число ограничено max_events_per_tick.
func (w *World) dispatch() {
	limit := w.C.Rules.MaxEventsPerTick
	if limit <= 0 {
		limit = 2000
	}
	processed := 0
	for w.dispatched < w.eventSeq && processed < limit {
		w.dispatched++
		processed++
		evs := w.EventsSince(w.dispatched - 1)
		if len(evs) == 0 {
			continue
		}
		ev := evs[0]
		w.reactTo(ev)
	}
	if w.dispatched < w.eventSeq {
		// Защита от бесконечных цепочек реакций: оставшиеся события не порождают реакций.
		w.dispatched = w.eventSeq
	}
}

func (w *World) reactTo(ev Event) {
	switch ev.Type {
	case EvBlocked:
		w.fireTriggers(w.Unit(ev.Unit), "blocked", ev, w.Unit(ev.Other))
	case EvEvaded:
		w.fireTriggers(w.Unit(ev.Unit), "evaded", ev, w.Unit(ev.Other))
	case EvDamage:
		if ev.Amount > 0 {
			w.fireTriggers(w.Unit(ev.Unit), "damaged", ev, w.Unit(ev.Other))
		}
	case EvHitDealt:
		w.fireTriggers(w.Unit(ev.Unit), "hit_dealt", ev, w.Unit(ev.Other))
	case EvUnitDied:
		dead := w.Unit(ev.Unit)
		if dead == nil {
			return
		}
		if k := w.Unit(ev.Other); k != nil {
			w.fireTriggers(k, "killed", ev, dead)
		}
		for _, a := range w.AliveUnits(dead.Side) {
			w.fireTriggersNear(a, "ally_died", ev, w.Unit(ev.Other), dead.Pos)
		}
	}
}

func (w *World) fireTriggers(u *Unit, event string, ev Event, other *Unit) {
	w.fireTriggersNear(u, event, ev, other, hex.Hex{})
}

func (w *World) fireTriggersNear(u *Unit, event string, ev Event, other *Unit, at hex.Hex) {
	if u == nil || u.Dead {
		return
	}
	for _, p := range u.passives {
		for i, tr := range p.Triggers {
			if tr.Event != event {
				continue
			}
			key := fmt.Sprintf("%s#%d", p.ID, i)
			if !w.triggerAllowed(u, &tr, key, ev, at) {
				continue
			}
			if tr.Chance > 0 && tr.Chance < 100 && w.RNG.D100() >= tr.Chance {
				continue
			}
			if tr.Once {
				if u.Once == nil {
					u.Once = map[string]bool{}
				}
				u.Once[key] = true
			}
			if tr.Cooldown > 0 {
				if u.TrigReady == nil {
					u.TrigReady = map[string]int{}
				}
				u.TrigReady[key] = w.Tick + w.Ticks(tr.Cooldown)
			}
			w.emit(Event{Type: EvPassiveTriggered, Unit: u.ID, Other: idOf(other), Hex: u.Pos, Skill: p.ID,
				Text: fmt.Sprintf("%s: срабатывает «%s»", u.Name, p.Name)})
			w.applyOps(tr.Ops, &OpContext{Source: u, Self: u, Target: other, Attacker: other, Label: p.Name})
			if u.Dead {
				return
			}
		}
	}
}

func (w *World) triggerAllowed(u *Unit, tr *data.Trigger, key string, ev Event, at hex.Hex) bool {
	if tr.Once && u.Once[key] {
		return false
	}
	if tr.Cooldown > 0 && u.TrigReady[key] > w.Tick {
		return false
	}
	if len(tr.AttackKinds) > 0 && !containsStr(tr.AttackKinds, ev.Kind) {
		return false
	}
	if tr.HPBelowPct > 0 && u.HP*100 >= tr.HPBelowPct*u.HPMax() {
		return false
	}
	if tr.Radius > 0 && hex.Distance(u.Pos, at) > tr.Radius {
		return false
	}
	return true
}
