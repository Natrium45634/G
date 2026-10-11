package sim

import (
	"fmt"
	"strings"

	"hextactics/engine/combat"
	"hextactics/engine/data"
	"hextactics/engine/hex"
	"hextactics/engine/stats"
)

func (w *World) ctxFor(u *Unit, role string, sector combat.Sector, kind string, at hex.Hex) *stats.Context {
	terrain := ""
	if c := w.Map.Cell(at); c != nil {
		terrain = c.Terrain.ID
	}
	hpPct := 100
	if m := u.HPMax(); m > 0 {
		hpPct = u.HP * 100 / m
	}
	return &stats.Context{Role: role, Sector: sector.Key(), Terrain: terrain, AttackKind: kind, HPPct: hpPct}
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// buildAttack собирает входные данные атаки att по def умением sk с позиции attPos.
func (w *World) buildAttack(att, def *Unit, sk *data.SkillDef, attPos hex.Hex) *combat.AttackInput {
	r := &w.C.Rules
	sector, _, _ := combat.SectorOf(def.Pos, def.Facing, attPos)
	kind := sk.AttackKind
	ac := w.ctxFor(att, "attacker", sector, kind, attPos)
	dc := w.ctxFor(def, "defender", sector, kind, def.Pos)
	in := &combat.AttackInput{Kind: kind, Sector: sector}

	// Обнаружение
	if v := int(att.stats.GetCtx("stealth", ac)); v != 0 {
		in.SurpriseMods = append(in.SurpriseMods, combat.Mod{Label: "скрытность атакующего", Value: v})
	}
	if v := int(def.stats.GetCtx("awareness", dc)); v != 0 {
		in.SurpriseMods = append(in.SurpriseMods, combat.Mod{Label: "бдительность цели", Value: -v})
	}
	if !w.IsVisibleTo(def.Side, att) && r.Surprise.UnseenAttackerBonus != 0 {
		in.SurpriseMods = append(in.SurpriseMods, combat.Mod{Label: "атакующий не виден", Value: r.Surprise.UnseenAttackerBonus})
	}
	if r.Surprise.DistractedBonus != 0 {
		for _, n := range def.Pos.Neighbors() {
			o := w.UnitAt(n)
			if o != nil && o.ID != att.ID && !o.Dead && w.Hostile(def.Side, o.Side) {
				in.SurpriseMods = append(in.SurpriseMods, combat.Mod{Label: "цель связана боем", Value: r.Surprise.DistractedBonus})
				break
			}
		}
	}
	incap := false
	if w.HasFlag(def, "asleep") {
		in.ForceSurprise, incap = "цель спит", true
	} else if w.HasFlag(def, "stunned") {
		in.ForceSurprise, incap = "цель оглушена", true
	}

	// Попадание
	in.Accuracy = int(att.stats.GetCtx("accuracy", ac))
	if sk.AccuracyMod != 0 {
		in.AccMods = append(in.AccMods, combat.Mod{Label: "умение", Value: sk.AccuracyMod})
	}
	ca, cd := w.Map.Cell(attPos), w.Map.Cell(def.Pos)
	if ca != nil && cd != nil && ca.Height != cd.Height && r.HeightHitBonus != 0 {
		in.AccMods = append(in.AccMods, combat.Mod{Label: "высота", Value: (ca.Height - cd.Height) * r.HeightHitBonus})
	}
	dist := hex.Distance(attPos, def.Pos)
	if dist > 1 && containsStr(r.CoverAppliesTo, kind) {
		if cd != nil && cd.Cover() > 0 {
			in.AccMods = append(in.AccMods, combat.Mod{Label: "укрытие цели: " + cd.Terrain.Name, Value: -cd.Cover()})
		}
		if los := w.Map.LineOfSight(attPos, def.Pos); los.SoftCover > 0 {
			in.AccMods = append(in.AccMods, combat.Mod{Label: "частичные препятствия", Value: -min(los.SoftCover, r.SoftCoverMax)})
		}
	}

	// Защита
	in.Evadable = !sk.Unevadable
	if !incap {
		in.Evasion = int(def.stats.GetCtx("evasion", dc))
		in.Block = int(def.stats.GetCtx("block", dc))
	}
	in.Blockable = !sk.Unblockable
	in.BlockPower = int(def.stats.GetCtx("block_power", dc))
	in.CritChance = int(att.stats.GetCtx("crit_chance", ac))
	in.CritDamage = int(att.stats.GetCtx("crit_damage", ac))

	// Урон
	for _, op := range sk.Ops {
		if op.Op != "damage" {
			continue
		}
		dt := w.C.DamageTypes[op.DamageType]
		raw, txt := w.rawAmount(op, att, ac)
		in.Damage = append(in.Damage, combat.DamageInput{
			Type: dt.ID, TypeName: dt.Name, Raw: raw, RawText: txt, Variance: op.Variance,
			ArmorApplies: dt.Armor, Armor: int(def.stats.GetCtx("armor", dc)),
			Resist: int(def.stats.GetCtx(dt.ResistStat, dc)), Vuln: int(def.stats.GetCtx(dt.VulnStat, dc)),
			Immune: def.HasTag("immune_damage:" + dt.ID),
		})
	}
	return in
}

// rawAmount — базовая величина операции: base + характеристика источника × scale%.
func (w *World) rawAmount(op data.OpDef, src *Unit, ctx *stats.Context) (float64, string) {
	v := op.Base
	txt := trimF(op.Base)
	if op.Stat != "" && src != nil && op.Scale != 0 {
		sv := src.stats.GetCtx(op.Stat, ctx)
		add := sv * op.Scale / 100
		v += add
		txt = fmt.Sprintf("%s + %s(%s)×%s%% = %s", trimF(op.Base), trimF(sv), w.C.Stats.Name(op.Stat), trimF(op.Scale), trimF(v))
	}
	return v, txt
}

func trimF(v float64) string {
	s := fmt.Sprintf("%.1f", v)
	s = strings.TrimSuffix(s, ".0")
	return s
}

// performAttack — атака с броском попадания и применением операций умения.
func (w *World) performAttack(att, def *Unit, sk *data.SkillDef) combat.Outcome {
	in := w.buildAttack(att, def, sk, att.Pos)
	o := combat.Resolve(in, &w.C.Rules, &w.RNG)
	head := fmt.Sprintf("%s → %s «%s»", att.Name, def.Name, sk.Name)
	var tags []string
	tags = append(tags, o.Sector.Name())
	if o.Surprised {
		tags = append(tags, "неожиданно")
	}
	switch {
	case !o.Hit:
		w.emit(Event{Type: EvMissed, Unit: att.ID, Other: def.ID, Skill: sk.ID, Kind: sk.AttackKind, Hex: def.Pos, From: att.Pos,
			Text: fmt.Sprintf("%s: промах (%s)", head, strings.Join(tags, ", ")), Details: o.Lines})
		return o
	case o.Evaded:
		w.emit(Event{Type: EvEvaded, Unit: def.ID, Other: att.ID, Skill: sk.ID, Kind: sk.AttackKind, Hex: def.Pos, From: att.Pos,
			Text: fmt.Sprintf("%s: уклонение (%s)", head, strings.Join(tags, ", ")), Details: o.Lines})
		return o
	}
	if o.Blocked {
		tags = append(tags, "блок")
	}
	if o.Crit {
		tags = append(tags, "КРИТ")
	}
	w.emit(Event{Type: EvAttack, Unit: att.ID, Other: def.ID, Skill: sk.ID, Kind: sk.AttackKind, Hex: def.Pos, From: att.Pos,
		Amount: o.Total, Crit: o.Crit, Text: fmt.Sprintf("%s: %d урона (%s)", head, o.Total, strings.Join(tags, ", ")), Details: o.Lines})
	if o.Blocked {
		w.emit(Event{Type: EvBlocked, Unit: def.ID, Other: att.ID, Skill: sk.ID, Kind: sk.AttackKind, Hex: def.Pos})
	}
	for i, dr := range o.Damage {
		if def.Dead {
			break
		}
		w.dealDamage(def, dr.Amount, dr.Type, att, DamageOpts{Kind: sk.AttackKind, Silent: true, Crit: o.Crit && i == 0, Skill: sk.ID})
		if att.Side != def.Side {
			if dt := w.C.DamageTypes[dr.Type]; dt != nil {
				w.reveal(att.Side, def.ID, dt.ResistStat, dt.VulnStat)
			}
		}
	}
	w.emit(Event{Type: EvHitDealt, Unit: att.ID, Other: def.ID, Skill: sk.ID, Kind: sk.AttackKind, Amount: o.Total})
	var rest []data.OpDef
	for _, op := range sk.Ops {
		if op.Op != "damage" && !isHexOp(op.Op) {
			rest = append(rest, op)
		}
	}
	if len(rest) > 0 && !def.Dead {
		w.applyOps(rest, &OpContext{Source: att, Self: att, Target: def, Attacker: att, Label: sk.Name, Skill: sk.ID, Hex: def.Pos})
	}
	return o
}

// AttackPreview — расчётные шансы атаки для подсказки игроку (без бросков).
// Скрытые характеристики цели в подсказку не выводятся.
func (w *World) AttackPreview(att, def *Unit, sk *data.SkillDef, from hex.Hex) combat.Preview {
	in := w.buildAttack(att, def, sk, from)
	return combat.Estimate(in, &w.C.Rules)
}

// reveal — сторона узнаёт характеристики юнита (например, сопротивление после удара).
func (w *World) reveal(side, unitID int, statIDs ...string) {
	if w.Known[side] == nil {
		w.Known[side] = map[int]map[string]bool{}
	}
	if w.Known[side][unitID] == nil {
		w.Known[side][unitID] = map[string]bool{}
	}
	for _, s := range statIDs {
		if s != "" {
			w.Known[side][unitID][s] = true
		}
	}
}

// KnownStat — известна ли стороне характеристика юнита.
func (w *World) KnownStat(side int, u *Unit, stat string) bool {
	if u.Side == side {
		return true
	}
	if d, ok := w.C.Stats.Get(stat); ok && d.Public {
		return true
	}
	return w.Known[side] != nil && w.Known[side][u.ID] != nil && w.Known[side][u.ID][stat]
}
