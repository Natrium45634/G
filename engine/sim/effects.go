package sim

import (
	"fmt"

	"hextactics/engine/data"
	"hextactics/engine/hex"
	"hextactics/engine/stats"
)

// refreshEffectMods пересобирает временные модификаторы юнита из активных эффектов.
// Вызывается только при изменении набора эффектов — не каждый кадр.
func (w *World) refreshEffectMods(u *Unit) {
	var mods []stats.Modifier
	for _, e := range u.Effects {
		d := w.C.Effects[e.Def]
		if d == nil {
			continue
		}
		stacks := max(1, e.Stacks)
		for _, m := range d.Modifiers {
			m.Source = "эффект: " + d.Name
			for i := 0; i < stacks; i++ {
				mods = append(mods, m)
			}
		}
	}
	u.stats.SetTemporary(mods)
	if u.AP > u.APMax() {
		u.AP = u.APMax()
	}
	if hm := u.HPMax(); u.HP > hm {
		u.HP = hm
	}
	u.PlanVer++
}

// findEffect — активный эффект по ID определения.
func (w *World) findEffect(u *Unit, def string) *ActiveEffect {
	for _, e := range u.Effects {
		if e.Def == def {
			return e
		}
	}
	return nil
}

// applyEffect накладывает эффект. durSec < 0 — длительность из определения (0 — бессрочно).
// Учитываются: иммунитет (тег immune_effect:<тег>), сопротивление наложению (resist_stat),
// упорство (tenacity — сокращение длительности отрицательных эффектов), правило наложения.
func (w *World) applyEffect(u *Unit, def string, durSec float64, source int, details *[]string) bool {
	return w.applyEffectEx(u, def, durSec, source, details, false, 0)
}

func (w *World) applyEffectEx(u *Unit, def string, durSec float64, source int, details *[]string, quiet bool, orderID int) bool {
	d := w.C.Effects[def]
	if d == nil || u.Dead {
		return false
	}
	note := func(s string) {
		if details != nil {
			*details = append(*details, s)
		}
	}
	// Иммунитет
	immune := u.HasTag("immune_effect:" + d.ID)
	for _, t := range d.Tags {
		if u.HasTag("immune_effect:" + t) {
			immune = true
		}
	}
	if immune {
		note(fmt.Sprintf("%s: иммунитет к эффекту «%s»", u.Name, d.Name))
		w.emit(Event{Type: EvEffectResisted, Unit: u.ID, Other: source, Effect: def, Hex: u.Pos,
			Text: fmt.Sprintf("%s невосприимчив к эффекту «%s»", u.Name, d.Name)})
		return false
	}
	// Сопротивление наложению
	if d.Kind == "debuff" && d.ResistStat != "" {
		chance := int(u.stats.Get(d.ResistStat))
		if chance > 0 {
			roll := w.RNG.D100()
			if roll < chance {
				note(fmt.Sprintf("%s: сопротивление «%s» %d%% → бросок %d → устоял", u.Name, d.Name, chance, roll))
				w.emit(Event{Type: EvEffectResisted, Unit: u.ID, Other: source, Effect: def, Hex: u.Pos,
					Text: fmt.Sprintf("%s устоял против «%s»", u.Name, d.Name)})
				return false
			}
			note(fmt.Sprintf("%s: сопротивление «%s» %d%% → бросок %d → не устоял", u.Name, d.Name, chance, roll))
		}
	}
	dur := durSec
	if dur < 0 {
		dur = d.Duration
	}
	ticks := -1
	if dur > 0 {
		ticks = w.Ticks(dur)
		if d.Kind == "debuff" {
			if ten := int(u.stats.Get("tenacity")); ten > 0 {
				ticks = max(1, ticks*(100-ten)/100)
				note(fmt.Sprintf("%s: упорство −%d%% длительности", u.Name, ten))
			}
		}
	}
	if ex := w.findEffect(u, def); ex != nil {
		switch d.Stacking {
		case "ignore":
			return false
		case "stack":
			if d.MaxStacks <= 0 || ex.Stacks < d.MaxStacks {
				ex.Stacks++
			}
		}
		if ticks < 0 || ex.Remaining < 0 {
			ex.Remaining = -1
		} else if ticks > ex.Remaining {
			ex.Remaining = ticks
			ex.Total = ticks
		}
		ex.Source = source
		if d.Absorb > ex.Absorb {
			ex.Absorb = d.Absorb
		}
		w.refreshEffectMods(u)
		if !quiet {
			w.emit(Event{Type: EvEffectApplied, Unit: u.ID, Other: source, Effect: def, Hex: u.Pos, Amount: ex.Stacks,
				Text: fmt.Sprintf("%s: эффект «%s» обновлён (%s)", u.Name, d.Name, durText(w, ex.Remaining))})
		}
		return true
	}
	e := &ActiveEffect{Def: def, Source: source, Remaining: ticks, Total: ticks, Stacks: 1, Absorb: d.Absorb, Order: orderID}
	if d.Periodic != nil {
		e.NextTick = w.Ticks(d.Periodic.Interval)
	}
	u.Effects = append(u.Effects, e)
	w.refreshEffectMods(u)
	if !quiet {
		w.emit(Event{Type: EvEffectApplied, Unit: u.ID, Other: source, Effect: def, Hex: u.Pos, Amount: 1,
			Text: fmt.Sprintf("%s: эффект «%s» (%s)", u.Name, d.Name, durText(w, ticks))})
	}
	if d.HasFlag("stunned") || d.HasFlag("asleep") {
		w.onIncapacitated(u)
	}
	return true
}

func durText(w *World, ticks int) string {
	if ticks < 0 {
		return "бессрочно"
	}
	return fmt.Sprintf("%.1f с", w.C.Seconds(ticks))
}

// removeEffect снимает эффект по индексу.
func (w *World) removeEffectAt(u *Unit, i int, reason string) {
	e := u.Effects[i]
	d := w.C.Effects[e.Def]
	u.Effects = append(u.Effects[:i], u.Effects[i+1:]...)
	w.refreshEffectMods(u)
	text := ""
	if d != nil && reason != "" {
		text = fmt.Sprintf("%s: «%s» — %s", u.Name, d.Name, reason)
	}
	w.emit(Event{Type: EvEffectExpired, Unit: u.ID, Effect: e.Def, Hex: u.Pos, Text: text})
}

// removeEffectsWhere снимает эффекты, удовлетворяющие условию.
func (w *World) removeEffectsWhere(u *Unit, pred func(*ActiveEffect) bool, reason string) {
	for i := len(u.Effects) - 1; i >= 0; i-- {
		if pred(u.Effects[i]) {
			w.removeEffectAt(u, i, reason)
		}
	}
}

// updateEffects — длительности, периодические действия и ауры.
func (w *World) updateEffects(u *Unit) {
	for i := 0; i < len(u.Effects) && !u.Dead; i++ {
		e := u.Effects[i]
		d := w.C.Effects[e.Def]
		if d == nil {
			continue
		}
		if d.Periodic != nil {
			e.NextTick--
			if e.NextTick <= 0 {
				e.NextTick = w.Ticks(d.Periodic.Interval)
				src := w.Unit(e.Source)
				w.applyOps(d.Periodic.Ops, &OpContext{Source: src, Self: u, Target: u, Mult: max(1, e.Stacks), Label: d.Name})
				if u.Dead {
					return
				}
			}
		}
		if d.Aura != nil {
			e.AuraTimer--
			if e.AuraTimer <= 0 {
				e.AuraTimer = w.Ticks(d.Aura.Interval)
				w.applyAura(u, d)
			}
		}
	}
	for i := len(u.Effects) - 1; i >= 0 && !u.Dead; i-- {
		e := u.Effects[i]
		if e.Remaining > 0 {
			e.Remaining--
			if e.Remaining == 0 {
				d := w.C.Effects[e.Def]
				reason := "закончился"
				if d != nil && d.Duration <= 1 && d.Aura == nil {
					reason = "" // короткие эффекты от аур не засоряют журнал
				}
				w.removeEffectAt(u, i, reason)
			}
		}
	}
}

// applyAura накладывает эффект ауры на юнитов в радиусе (короткий эффект, обновляемый по таймеру).
func (w *World) applyAura(carrier *Unit, d *data.EffectDef) {
	a := d.Aura
	for _, t := range w.AliveUnits(-1) {
		if hex.Distance(t.Pos, carrier.Pos) > a.Radius {
			continue
		}
		ally := t.Side == carrier.Side
		if (a.Affects == "allies" && !ally) || (a.Affects == "enemies" && ally) {
			continue
		}
		w.applyEffectEx(t, a.Effect, -1, carrier.ID, nil, true, 0)
	}
}
