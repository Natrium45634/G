// Package combat — правила направленного боя: секторы, обнаружение атаки,
// попадание, уклонение, блок, критический удар и урон.
//
// Пакет не знает о мире и юнитах: симуляция собирает входные числа (характеристики
// уже рассчитаны с учётом ситуативных модификаторов) и вызывает Resolve.
// Каждая проверка — отдельный бросок; порядок шагов фиксирован и описан в Resolve.
// Базовые шансы, модификаторы и итоговые значения хранятся раздельно (Mod/Outcome).
package combat

import (
	"fmt"
	"math"
	"strings"

	"hextactics/engine/data"
	"hextactics/engine/hex"
	"hextactics/engine/rng"
)

// Sector — сектор, из которого пришла угроза относительно взгляда защищающегося.
type Sector int

const (
	Front     Sector = 0
	NearFlank Sector = 1
	FarFlank  Sector = 2
	Rear      Sector = 3
)

// Key — ключ сектора для условий модификаторов.
func (s Sector) Key() string { return data.SectorKeys[s] }

// Name — русское название.
func (s Sector) Name() string { return data.SectorNames[s] }

// SectorOf определяет сектор угрозы. f — направление взгляда защищающегося,
// a — направление от защищающегося на источник угрозы, Delta = min(|f-a|, 6-|f-a|).
// Если источник лежит ровно между двумя направлениями, выбирается более
// благоприятный для защищающегося сектор (меньший Delta).
func SectorOf(defPos hex.Hex, facing hex.Dir, threat hex.Hex) (Sector, hex.Dir, bool) {
	p, alt, tie, ok := hex.DirectionsTo(defPos, threat)
	if !ok {
		return Front, facing, false
	}
	d := hex.Delta(facing, p)
	dir := p
	if tie {
		if d2 := hex.Delta(facing, alt); d2 < d {
			d, dir = d2, alt
		}
	}
	return Sector(d), dir, true
}

// Mod — именованный модификатор шанса в процентных пунктах.
type Mod struct {
	Label string
	Value int
}

func sumMods(ms []Mod) int {
	s := 0
	for _, m := range ms {
		s += m.Value
	}
	return s
}

func describeMods(base int, baseLabel string, ms []Mod) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d (%s)", base, baseLabel)
	for _, m := range ms {
		if m.Value == 0 {
			continue
		}
		fmt.Fprintf(&b, " %+d (%s)", m.Value, m.Label)
	}
	return b.String()
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// DamageInput — компонент урона (один op "damage").
type DamageInput struct {
	Type         string
	TypeName     string
	Raw          float64 // base + stat × scale
	RawText      string  // объяснение исходной величины
	Variance     int     // ±%
	ArmorApplies bool
	Armor        int
	Resist       int // %
	Vuln         int // %
	Immune       bool
}

// AttackInput — все числа, необходимые для разрешения атаки.
type AttackInput struct {
	Kind   string // melee | ranged | spell
	Sector Sector

	SurpriseMods  []Mod
	ForceSurprise string // причина обязательной неожиданности (оглушение, сон) или ""

	Accuracy int
	AccMods  []Mod

	Evadable bool
	Evasion  int

	Blockable  bool
	Block      int
	BlockPower int

	CritChance int
	CritDamage int

	Damage []DamageInput
}

// DamageResult — итог по компоненту урона.
type DamageResult struct {
	Type   string
	Amount int
}

// Outcome — результат атаки.
type Outcome struct {
	Sector         Sector
	SurpriseChance int
	Surprised      bool
	HitChance      int
	Hit            bool
	EvadeChance    int
	Evaded         bool
	BlockChance    int
	Blocked        bool
	CritChance     int
	Crit           bool
	Damage         []DamageResult
	Total          int
	Lines          []string
}

// Landed — атака достигла цели (попала и не была уклонена).
func (o *Outcome) Landed() bool { return o.Hit && !o.Evaded }

// Resolve разрешает атаку. Порядок шагов:
//
//  1. обнаружение (неожиданность) — профиль сектора + модификаторы, бросок;
//  2. попадание — точность + модификаторы, ограничение [hit_min, hit_max], бросок;
//  3. уклонение — уклонение цели × коэффициент сектора (нет при неожиданности), бросок;
//  4. блок — блок цели × эффективность фронтального блока (нет при неожиданности), бросок;
//  5. критический удар — шанс атакующего (+ бонус неожиданности), бросок;
//  6. урон — для каждого компонента: разброс → крит → броня×позиционная защита →
//     сопротивление → уязвимость → иммунитет → блок → округление.
//
// Дополнительные эффекты и прерывание обрабатывает симуляция после Resolve.
func Resolve(in *AttackInput, rules *data.Rules, r *rng.RNG) Outcome {
	o := Outcome{Sector: in.Sector}
	s := int(in.Sector)
	prof := rules.Sectors
	line := func(format string, a ...any) { o.Lines = append(o.Lines, fmt.Sprintf(format, a...)) }

	line("Сектор: %s (Δ=%d)", in.Sector.Name(), s)

	// 1. Обнаружение
	roll := r.D100()
	if in.ForceSurprise != "" {
		o.SurpriseChance = 100
		o.Surprised = true
		line("Неожиданность: 100%% (%s) → неожиданная", in.ForceSurprise)
	} else {
		o.SurpriseChance = clamp(prof.Surprise[s]+sumMods(in.SurpriseMods), 0, 100)
		o.Surprised = roll < o.SurpriseChance
		verdict := "замечена"
		if o.Surprised {
			verdict = "НЕОЖИДАННАЯ"
		}
		line("Неожиданность: %s = %d%% → бросок %d → %s",
			describeMods(prof.Surprise[s], "сектор", in.SurpriseMods), o.SurpriseChance, roll, verdict)
	}

	// 2. Попадание
	o.HitChance = clamp(in.Accuracy+sumMods(in.AccMods), rules.HitMin, rules.HitMax)
	roll = r.D100()
	o.Hit = roll < o.HitChance
	line("Попадание: %s = %d%% → бросок %d → %s",
		describeMods(in.Accuracy, "точность", in.AccMods), o.HitChance, roll, yesNo(o.Hit, "попал", "промах"))
	if !o.Hit {
		return o
	}

	// 3. Уклонение
	switch {
	case !in.Evadable:
		line("Уклонение: невозможно для этой атаки")
	case o.Surprised && rules.Surprise.NegatesEvasion:
		line("Уклонение: нет (атака неожиданная)")
	default:
		o.EvadeChance = clamp(in.Evasion*prof.Evasion[s]/100, 0, rules.EvasionMax)
		if o.EvadeChance > 0 {
			roll = r.D100()
			o.Evaded = roll < o.EvadeChance
			line("Уклонение: %d × %d%% (сектор) = %d%% → бросок %d → %s",
				in.Evasion, prof.Evasion[s], o.EvadeChance, roll, yesNo(o.Evaded, "УКЛОНИЛСЯ", "нет"))
		} else {
			line("Уклонение: 0%%")
		}
	}
	if o.Evaded {
		return o
	}

	// 4. Блок
	switch {
	case !in.Blockable || in.Block <= 0:
		if in.Block > 0 {
			line("Блок: невозможен для этой атаки")
		}
	case o.Surprised && rules.Surprise.NegatesBlock:
		line("Блок: нет (атака неожиданная)")
	default:
		o.BlockChance = clamp(in.Block*prof.FrontBlock[s]/100, 0, rules.BlockMax)
		if o.BlockChance > 0 {
			roll = r.D100()
			o.Blocked = roll < o.BlockChance
			line("Блок: %d × %d%% (эффективность в секторе) = %d%% → бросок %d → %s",
				in.Block, prof.FrontBlock[s], o.BlockChance, roll, yesNo(o.Blocked, "ЗАБЛОКИРОВАНО", "нет"))
		} else {
			line("Блок: 0%% в этом секторе")
		}
	}

	// 5. Крит
	critMods := []Mod{}
	if o.Surprised && rules.Surprise.CritBonus != 0 {
		critMods = append(critMods, Mod{"неожиданность", rules.Surprise.CritBonus})
	}
	o.CritChance = clamp(in.CritChance+sumMods(critMods), 0, 100)
	roll = r.D100()
	o.Crit = roll < o.CritChance
	line("Крит: %s = %d%% → бросок %d → %s", describeMods(in.CritChance, "шанс", critMods), o.CritChance, roll, yesNo(o.Crit, "КРИТ", "нет"))

	// 6. Урон
	for _, d := range in.Damage {
		amt, lines := Mitigate(d, MitigationOpts{
			Crit: o.Crit, CritDamage: in.CritDamage, PositionalPct: prof.PositionalDefense[s],
			Blocked: o.Blocked, BlockPower: in.BlockPower, Rules: rules,
		}, r)
		o.Lines = append(o.Lines, lines...)
		o.Damage = append(o.Damage, DamageResult{Type: d.Type, Amount: amt})
		o.Total += amt
	}
	return o
}

// MitigationOpts — параметры расчёта урона.
type MitigationOpts struct {
	Crit          bool
	CritDamage    int
	PositionalPct int // эффективность брони в секторе (100 — полная)
	Blocked       bool
	BlockPower    int
	Rules         *data.Rules
}

// Mitigate рассчитывает итоговый урон компонента с объяснением.
// Используется и для атак, и для урона без броска попадания (ловушки, горение).
func Mitigate(d DamageInput, o MitigationOpts, r *rng.RNG) (int, []string) {
	var lines []string
	v := d.Raw
	txt := fmt.Sprintf("Урон (%s): %s", d.TypeName, d.RawText)
	if d.Variance > 0 {
		roll := r.Range(-d.Variance, d.Variance)
		v = v * float64(100+roll) / 100
		txt += fmt.Sprintf(" → разброс %+d%% = %.1f", roll, v)
	}
	if o.Crit {
		v = v * float64(o.CritDamage) / 100
		txt += fmt.Sprintf(" → крит ×%d%% = %.1f", o.CritDamage, v)
	}
	if d.ArmorApplies && d.Armor > 0 {
		pos := o.PositionalPct
		eff := float64(d.Armor) * float64(pos) / 100
		v -= eff
		if v < 0 {
			v = 0
		}
		txt += fmt.Sprintf(" → броня %d×%d%% = −%.1f → %.1f", d.Armor, pos, eff, v)
	}
	if d.Resist != 0 {
		res := d.Resist
		if o.Rules != nil && res > o.Rules.ResistMax {
			res = o.Rules.ResistMax
		}
		v = v * float64(100-res) / 100
		txt += fmt.Sprintf(" → сопротивление %d%% = %.1f", res, v)
	}
	if d.Vuln != 0 {
		v = v * float64(100+d.Vuln) / 100
		txt += fmt.Sprintf(" → уязвимость +%d%% = %.1f", d.Vuln, v)
	}
	if d.Immune {
		v = 0
		txt += " → ИММУНИТЕТ = 0"
	}
	if o.Blocked {
		v = v * float64(100-o.BlockPower) / 100
		txt += fmt.Sprintf(" → блок −%d%% = %.1f", o.BlockPower, v)
	}
	amt := int(math.Round(v))
	minDmg := 0
	if o.Rules != nil {
		minDmg = o.Rules.MinDamage
	}
	if !d.Immune && d.Raw > 0 && amt < minDmg {
		amt = minDmg
		txt += fmt.Sprintf(" → минимум %d", minDmg)
	}
	txt += fmt.Sprintf(" ⇒ %d", amt)
	lines = append(lines, txt)
	return amt, lines
}

func yesNo(b bool, y, n string) string {
	if b {
		return y
	}
	return n
}

// Preview — расчётные шансы без бросков (для подсказки игроку).
type Preview struct {
	Sector         Sector
	SurpriseChance int
	HitChance      int
	EvadeChance    int
	BlockChance    int
	CritChance     int
	// Вероятность, что атака нанесёт урон (попадание и не уклонение), с учётом неожиданности.
	LandChance int
	Lines      []string
}

// Estimate считает шансы для подсказки (без расходования случайных чисел).
func Estimate(in *AttackInput, rules *data.Rules) Preview {
	s := int(in.Sector)
	prof := rules.Sectors
	p := Preview{Sector: in.Sector}
	if in.ForceSurprise != "" {
		p.SurpriseChance = 100
	} else {
		p.SurpriseChance = clamp(prof.Surprise[s]+sumMods(in.SurpriseMods), 0, 100)
	}
	p.HitChance = clamp(in.Accuracy+sumMods(in.AccMods), rules.HitMin, rules.HitMax)
	if in.Evadable {
		p.EvadeChance = clamp(in.Evasion*prof.Evasion[s]/100, 0, rules.EvasionMax)
	}
	if in.Blockable && in.Block > 0 {
		p.BlockChance = clamp(in.Block*prof.FrontBlock[s]/100, 0, rules.BlockMax)
	}
	p.CritChance = in.CritChance
	// Уклонение действует только при замеченной атаке
	evadeEff := float64(p.EvadeChance) / 100
	if rules.Surprise.NegatesEvasion {
		evadeEff *= float64(100-p.SurpriseChance) / 100
	}
	p.LandChance = int(math.Round(float64(p.HitChance) * (1 - evadeEff)))
	p.Lines = append(p.Lines,
		fmt.Sprintf("Сектор цели: %s", in.Sector.Name()),
		fmt.Sprintf("Попадание: %s = %d%%", describeMods(in.Accuracy, "точность", in.AccMods), p.HitChance),
		fmt.Sprintf("Неожиданность: %d%%", p.SurpriseChance),
		fmt.Sprintf("Уклонение цели: %d%%  Блок: %d%%", p.EvadeChance, p.BlockChance),
		fmt.Sprintf("Итоговый шанс нанести урон ≈ %d%%", p.LandChance),
	)
	return p
}
