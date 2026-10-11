package sim

import (
	"fmt"

	"hextactics/engine/combat"
	"hextactics/engine/data"
	"hextactics/engine/hex"
)

// OpContext — контекст выполнения операций (умение, эффект, ловушка, реакция пассивки).
type OpContext struct {
	Source   *Unit   // источник (заклинатель, наложивший эффект, владелец ловушки)
	Self     *Unit   // владелец умения/навыка (для target=self)
	Target   *Unit   // основная цель
	Attacker *Unit   // атакующий (для реакций)
	Hex      hex.Hex // клетка (для place_object, summon)
	Mult     int     // множитель (заряды эффекта)
	Label    string  // название источника для журнала
	Skill    string
	Side     int // сторона источника, если источника-юнита нет (ловушка погибшего)
}

func (w *World) opTarget(o data.OpDef, ctx *OpContext) *Unit {
	switch o.Target {
	case "self":
		return ctx.Self
	case "attacker":
		return ctx.Attacker
	case "source":
		return ctx.Source
	}
	return ctx.Target
}

// applyOps выполняет список операций по порядку.
func (w *World) applyOps(ops []data.OpDef, ctx *OpContext) {
	for _, o := range ops {
		if o.Chance > 0 && o.Chance < 100 {
			if w.RNG.D100() >= o.Chance {
				continue
			}
		}
		mult := max(1, ctx.Mult)
		switch o.Op {
		case "damage":
			t := w.opTarget(o, ctx)
			if t == nil || t.Dead {
				continue
			}
			dt := w.C.DamageTypes[o.DamageType]
			raw, txt := w.rawAmount(o, ctx.Source, nil)
			raw *= float64(mult)
			if mult > 1 {
				txt += fmt.Sprintf(" ×%d (заряды)", mult)
			}
			in := combat.DamageInput{Type: dt.ID, TypeName: dt.Name, Raw: raw, RawText: txt, Variance: o.Variance,
				ArmorApplies: dt.Armor, Armor: int(t.stats.Get("armor")),
				Resist: int(t.stats.Get(dt.ResistStat)), Vuln: int(t.stats.Get(dt.VulnStat)),
				Immune: t.HasTag("immune_damage:" + dt.ID)}
			amt, lines := combat.Mitigate(in, combat.MitigationOpts{PositionalPct: 100, Rules: &w.C.Rules}, &w.RNG)
			w.dealDamage(t, amt, dt.ID, ctx.Source, DamageOpts{Label: ctx.Label, Details: lines, Skill: ctx.Skill})
		case "heal":
			t := w.opTarget(o, ctx)
			if t == nil || t.Dead {
				continue
			}
			raw, txt := w.rawAmount(o, ctx.Source, nil)
			w.heal(t, int(raw*float64(mult)+0.5), ctx.Source, ctx.Label, txt)
		case "apply_effect":
			t := w.opTarget(o, ctx)
			if t == nil || t.Dead {
				continue
			}
			dur := o.Duration
			if dur == 0 {
				dur = -1
			}
			src := 0
			if ctx.Source != nil {
				src = ctx.Source.ID
			}
			w.applyEffect(t, o.Effect, dur, src, nil)
		case "remove_effect":
			t := w.opTarget(o, ctx)
			if t != nil {
				w.removeEffectsWhere(t, func(e *ActiveEffect) bool { return e.Def == o.Effect }, "снят")
			}
		case "restore":
			t := w.opTarget(o, ctx)
			if t == nil || t.Dead {
				continue
			}
			switch o.Resource {
			case "ap":
				t.AP = min(t.APMax(), t.AP+data.AP(o.Amount))
			case "hp":
				w.heal(t, int(o.Amount+0.5), ctx.Source, ctx.Label, "")
			default:
				if _, ok := t.Res[o.Resource]; ok {
					t.Res[o.Resource] = min(w.ResMax(t, o.Resource), t.Res[o.Resource]+int(o.Amount*data.ResScale+0.5))
				}
			}
		case "place_object":
			w.placeObject(o.Object, ctx)
		case "summon":
			w.summon(o.Class, max(1, o.Count), ctx)
		}
	}
}

// DamageOpts — параметры нанесения урона.
type DamageOpts struct {
	Kind       string // melee/ranged/spell — для отражения
	Label      string
	Details    []string
	Silent     bool // не писать строку в журнал (атака уже описана)
	Crit       bool
	Skill      string
	NoRedirect bool
	NoReflect  bool
}

// dealDamage наносит урон: поглощение → перенаправление → здоровье → снятие сна →
// прерывание → отражение → смерть. Возвращает нанесённый урон.
func (w *World) dealDamage(t *Unit, amount int, dtype string, src *Unit, opt DamageOpts) int {
	if t.Dead || amount <= 0 {
		return 0
	}
	details := append([]string(nil), opt.Details...)
	// Поглощение
	for i := 0; i < len(t.Effects) && amount > 0; i++ {
		e := t.Effects[i]
		if e.Absorb <= 0 {
			continue
		}
		take := min(e.Absorb, amount)
		e.Absorb -= take
		amount -= take
		name := e.Def
		if d := w.C.Effects[e.Def]; d != nil {
			name = d.Name
		}
		details = append(details, fmt.Sprintf("«%s» поглотил %d урона", name, take))
		if e.Absorb == 0 {
			w.removeEffectAt(t, i, "исчерпан")
			i--
		}
	}
	// Перенаправление
	if !opt.NoRedirect {
		for _, e := range t.Effects {
			d := w.C.Effects[e.Def]
			if d == nil || d.Redirect == nil || amount <= 0 {
				continue
			}
			p := w.Unit(e.Source)
			if p == nil || p.Dead || p.ID == t.ID || hex.Distance(p.Pos, t.Pos) > d.Redirect.Range {
				continue
			}
			red := amount * d.Redirect.Pct / 100
			if red <= 0 {
				continue
			}
			amount -= red
			details = append(details, fmt.Sprintf("%d урона перенаправлено на %s («%s»)", red, p.Name, d.Name))
			w.dealDamage(p, red, dtype, src, DamageOpts{Label: "перенаправление: " + d.Name, NoRedirect: true, NoReflect: true})
		}
	}
	if amount <= 0 {
		w.emit(Event{Type: EvDamage, Unit: t.ID, Other: idOf(src), Hex: t.Pos, Amount: 0, Kind: dtype, Skill: opt.Skill,
			Text: fmt.Sprintf("%s: урон полностью поглощён", t.Name), Details: details})
		return 0
	}
	t.HP -= amount
	dangerous := amount*100 >= w.C.Rules.DangerousDamagePct*t.HPMax()
	text := ""
	if !opt.Silent {
		srcName := opt.Label
		if srcName == "" && src != nil {
			srcName = src.Name
		}
		text = fmt.Sprintf("%s получает %d урона (%s) [%s]", t.Name, amount, w.C.DamageTypes[dtype].Name, srcName)
	}
	w.emit(Event{Type: EvDamage, Unit: t.ID, Other: idOf(src), Hex: t.Pos, Amount: amount, Kind: dtype, Skill: opt.Skill,
		Crit: opt.Crit, Dangerous: dangerous, Text: text, Details: details})
	// Сон прерывается уроном
	w.removeEffectsWhere(t, func(e *ActiveEffect) bool {
		d := w.C.Effects[e.Def]
		return d != nil && d.BreakOnDamage
	}, "прерван уроном")
	// Прерывание подготовки/каналирования
	if w.C.Rules.InterruptOnDamage && t.Action != nil && t.Action.Interruptible && !t.Dead && t.HP > 0 {
		w.interrupt(t, fmt.Sprintf("получен урон %d", amount))
	}
	if src != nil && w.Hostile(src.Side, t.Side) {
		t.Threat = &ThreatMemory{From: src.Pos, Tick: w.Tick}
	}
	// Отражение
	if !opt.NoReflect && opt.Kind == "melee" && src != nil && !src.Dead {
		if pct := int(t.stats.Get("reflect_melee")); pct > 0 {
			if ref := amount * pct / 100; ref > 0 {
				w.dealDamage(src, ref, dtype, t, DamageOpts{Label: "отражение: " + t.Name, NoReflect: true, NoRedirect: true})
			}
		}
	}
	if t.HP <= 0 {
		w.kill(t, src)
	}
	return amount
}

func idOf(u *Unit) int {
	if u == nil {
		return 0
	}
	return u.ID
}

// heal восстанавливает здоровье.
func (w *World) heal(t *Unit, amount int, src *Unit, label, txt string) {
	if t.Dead || amount <= 0 {
		return
	}
	before := t.HP
	t.HP = min(t.HPMax(), t.HP+amount)
	got := t.HP - before
	var details []string
	if txt != "" {
		details = []string{"Лечение: " + txt}
	}
	w.emit(Event{Type: EvHeal, Unit: t.ID, Other: idOf(src), Hex: t.Pos, Amount: got,
		Text: fmt.Sprintf("%s восстанавливает %d HP [%s]", t.Name, got, label), Details: details})
}

// kill — гибель юнита.
func (w *World) kill(u *Unit, killer *Unit) {
	if u.Dead {
		return
	}
	u.Dead = true
	u.HP = 0
	u.Action = nil
	if u.Current != nil {
		u.Current.Status = StatusCancelled
		u.Current = nil
	}
	for _, q := range u.Queue {
		q.Status = StatusCancelled
	}
	u.Queue = nil
	for h, id := range w.occ {
		if id == u.ID {
			delete(w.occ, h)
		}
	}
	for h, id := range w.resv {
		if id == u.ID {
			delete(w.resv, h)
		}
	}
	w.OccVersion++
	w.visDirty = true
	text := fmt.Sprintf("%s погибает", u.Name)
	if killer != nil && killer.ID != u.ID {
		text += fmt.Sprintf(" (%s)", killer.Name)
	}
	w.emit(Event{Type: EvUnitDied, Unit: u.ID, Other: idOf(killer), Hex: u.Pos, Dangerous: u.class.Important, Text: text})
}
