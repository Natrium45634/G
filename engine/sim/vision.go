package sim

import (
	"fmt"

	"hextactics/engine/hex"
)

// Видимость: юнит видит врага, если тот в пределах дальности обзора (sight) и есть
// линия видимости. Сторона видит врага, если его видит хотя бы один её юнит.
// Ориентация на обзор пока не влияет (секторное зрение — следующий этап),
// но влияет на обнаружение атак (см. combat).

// IsVisibleTo — видит ли сторона юнита.
func (w *World) IsVisibleTo(side int, u *Unit) bool {
	if u == nil {
		return false
	}
	if u.Side == side {
		return true
	}
	if side < 0 || side >= len(w.visible) {
		return false
	}
	return w.visible[side][u.ID]
}

// UnitSees — видит ли юнит a клетку h.
func (w *World) UnitSees(a *Unit, h hex.Hex) bool {
	if a.Dead {
		return false
	}
	sight := int(a.stats.Get("sight"))
	if hex.Distance(a.Pos, h) > sight {
		return false
	}
	return w.Map.LineOfSight(a.Pos, h).Clear
}

func (w *World) updateVision(emitEvents bool) {
	w.visDirty = false
	for s := range w.Sides {
		side := w.Sides[s].ID
		if side < 0 || side >= len(w.visible) {
			continue
		}
		next := map[int]bool{}
		for _, e := range w.Units {
			if e.Dead || e.Side == side {
				continue
			}
			for _, a := range w.Units {
				if a.Side != side || a.Dead {
					continue
				}
				if w.UnitSees(a, e.Pos) {
					next[e.ID] = true
					break
				}
			}
		}
		prev := w.visible[side]
		w.visible[side] = next
		if !emitEvents {
			continue
		}
		for _, e := range w.sortedUnits() {
			if next[e.ID] && !prev[e.ID] {
				ev := w.emit(Event{Type: EvEnemySpotted, Unit: e.ID, Hex: e.Pos, Kind: fmt.Sprint(side),
					Text: fmt.Sprintf("%s: замечен противник — %s", w.SideOf(side).Name, e.Name)})
				ev.Visible = 1 << uint(side)
			} else if !next[e.ID] && prev[e.ID] && !e.Dead {
				ev := w.emit(Event{Type: EvEnemyLost, Unit: e.ID, Hex: e.Pos, Kind: fmt.Sprint(side)})
				ev.Visible = 1 << uint(side)
			}
		}
	}
}

// VisibleEnemies — видимые стороной враги (по ID).
func (w *World) VisibleEnemies(side int) []*Unit {
	var out []*Unit
	for _, u := range w.Units {
		if !u.Dead && u.Side != side && w.IsVisibleTo(side, u) {
			out = append(out, u)
		}
	}
	return out
}
