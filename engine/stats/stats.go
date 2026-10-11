// Package stats — универсальная система характеристик.
//
// Перечень характеристик не зашит в код: определения (Def) загружаются из данных.
// Значение вычисляется по явно заданному порядку (см. Compute):
//
//  1. базовое значение (из класса юнита или из определения);
//  2. постоянные плоские модификаторы (+X);
//  3. постоянные коэффициенты: сначала процентные (суммируются: ×(1+Σ%/100)),
//     затем мультипликативные (перемножаются);
//  4. временные и ситуативные плоские модификаторы;
//  5. временные и ситуативные процентные, затем мультипликативные;
//  6. ограничения минимум/максимум.
//
// Каждый шаг можно получить в виде объяснения (Breakdown) с указанием источника.
package stats

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Op — тип модификатора.
type Op string

const (
	OpFlat Op = "flat" // прибавить значение
	OpPct  Op = "pct"  // процент: проценты одного слоя суммируются
	OpMult Op = "mult" // множитель: перемножаются
)

// Layer — слой модификатора.
type Layer int

const (
	Permanent Layer = iota // класс, экипировка, пассивные навыки
	Temporary              // активные эффекты
)

// Condition — условие ситуативного модификатора. Пустые поля не проверяются.
type Condition struct {
	Role        string   `json:"role,omitempty"`         // attacker | defender
	Sectors     []string `json:"sectors,omitempty"`      // front | near_flank | far_flank | rear
	Terrain     []string `json:"terrain,omitempty"`      // поверхность клетки юнита
	AttackKinds []string `json:"attack_kinds,omitempty"` // melee | ranged | spell
	HPBelowPct  int      `json:"hp_below_pct,omitempty"`
	HPAbovePct  int      `json:"hp_above_pct,omitempty"`
	Surprised   *bool    `json:"surprised,omitempty"`
}

// Context — ситуация, в которой запрашивается характеристика.
type Context struct {
	Role       string
	Sector     string
	Terrain    string
	AttackKind string
	HPPct      int
	Surprised  bool
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Match проверяет условие в контексте. Без контекста ситуативные модификаторы не действуют.
func (c *Condition) Match(ctx *Context) bool {
	if c == nil {
		return true
	}
	if ctx == nil {
		return false
	}
	if c.Role != "" && c.Role != ctx.Role {
		return false
	}
	if len(c.Sectors) > 0 && !contains(c.Sectors, ctx.Sector) {
		return false
	}
	if len(c.Terrain) > 0 && !contains(c.Terrain, ctx.Terrain) {
		return false
	}
	if len(c.AttackKinds) > 0 && !contains(c.AttackKinds, ctx.AttackKind) {
		return false
	}
	if c.HPBelowPct > 0 && ctx.HPPct >= c.HPBelowPct {
		return false
	}
	if c.HPAbovePct > 0 && ctx.HPPct <= c.HPAbovePct {
		return false
	}
	if c.Surprised != nil && *c.Surprised != ctx.Surprised {
		return false
	}
	return true
}

// Describe — человекочитаемое описание условия.
func (c *Condition) Describe() string {
	if c == nil {
		return ""
	}
	var parts []string
	if c.Role == "attacker" {
		parts = append(parts, "при атаке")
	} else if c.Role == "defender" {
		parts = append(parts, "при защите")
	}
	if len(c.Sectors) > 0 {
		parts = append(parts, "сектор: "+strings.Join(c.Sectors, "/"))
	}
	if len(c.Terrain) > 0 {
		parts = append(parts, "местность: "+strings.Join(c.Terrain, "/"))
	}
	if len(c.AttackKinds) > 0 {
		parts = append(parts, "тип атаки: "+strings.Join(c.AttackKinds, "/"))
	}
	if c.HPBelowPct > 0 {
		parts = append(parts, fmt.Sprintf("здоровье < %d%%", c.HPBelowPct))
	}
	if c.HPAbovePct > 0 {
		parts = append(parts, fmt.Sprintf("здоровье > %d%%", c.HPAbovePct))
	}
	if c.Surprised != nil {
		if *c.Surprised {
			parts = append(parts, "неожиданная атака")
		} else {
			parts = append(parts, "замеченная атака")
		}
	}
	return strings.Join(parts, ", ")
}

// Modifier — модификатор характеристики.
type Modifier struct {
	Stat   string     `json:"stat"`
	Op     Op         `json:"op"`
	Value  float64    `json:"value"`
	When   *Condition `json:"when,omitempty"`
	Layer  Layer      `json:"-"`
	Source string     `json:"-"`
}

// Situational — модификатор действует только при выполнении условия.
func (m Modifier) Situational() bool { return m.When != nil }

// Def — определение характеристики.
type Def struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Group   string   `json:"group"` // base, resource, combat, defense, movement, perception, resist, vuln, regen, special
	Type    string   `json:"type"`  // int | float | percent
	Base    float64  `json:"base"`  // значение по умолчанию
	Min     *float64 `json:"min,omitempty"`
	Max     *float64 `json:"max,omitempty"`
	Unit    string   `json:"unit,omitempty"`    // "ОД", "с", "%", ...
	Display string   `json:"display,omitempty"` // value | percent | hidden
	Public  bool     `json:"public"`            // видна ли противнику
	Desc    string   `json:"desc,omitempty"`
}

// Format — форматирование значения по типу характеристики.
func (d *Def) Format(v float64) string {
	switch d.Type {
	case "int":
		return fmt.Sprintf("%d%s", int(math.Round(v)), d.unitSuffix())
	case "percent":
		return fmt.Sprintf("%s%%", trimFloat(v))
	default:
		return trimFloat(v) + d.unitSuffix()
	}
}

func (d *Def) unitSuffix() string {
	if d.Unit == "" || d.Unit == "%" {
		return ""
	}
	return " " + d.Unit
}

func trimFloat(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "-0" {
		s = "0"
	}
	return s
}

// Registry — реестр определений характеристик.
type Registry struct {
	defs  map[string]*Def
	Order []string
}

// NewRegistry строит реестр; ID должны быть уникальны.
func NewRegistry(defs []Def) (*Registry, error) {
	r := &Registry{defs: map[string]*Def{}}
	for i := range defs {
		d := defs[i]
		if d.ID == "" {
			return nil, fmt.Errorf("характеристика без id (позиция %d)", i)
		}
		if _, dup := r.defs[d.ID]; dup {
			return nil, fmt.Errorf("повтор характеристики %q", d.ID)
		}
		r.defs[d.ID] = &d
		r.Order = append(r.Order, d.ID)
	}
	return r, nil
}

// Get возвращает определение.
func (r *Registry) Get(id string) (*Def, bool) {
	d, ok := r.defs[id]
	return d, ok
}

// Has — есть ли характеристика.
func (r *Registry) Has(id string) bool { _, ok := r.defs[id]; return ok }

// Name — имя характеристики (или id).
func (r *Registry) Name(id string) string {
	if d, ok := r.defs[id]; ok {
		return d.Name
	}
	return id
}

// ---------------------------------------------------------------------------

// Step — шаг расчёта для объяснения.
type Step struct {
	Stage  string  // base | perm_flat | perm_pct | perm_mult | temp_flat | temp_pct | temp_mult | clamp
	Source string  // источник модификатора
	Op     Op      // тип
	Value  float64 // значение модификатора
	Result float64 // значение после шага
}

// Breakdown — объяснение расчёта характеристики.
type Breakdown struct {
	Stat  string
	Name  string
	Base  float64
	Final float64
	Steps []Step
}

// Lines — текстовое объяснение.
func (b Breakdown) Lines() []string {
	var out []string
	for _, s := range b.Steps {
		switch s.Stage {
		case "base":
			out = append(out, fmt.Sprintf("база %s", trimFloat(s.Result)))
		case "clamp":
			out = append(out, fmt.Sprintf("ограничение → %s", trimFloat(s.Result)))
		default:
			sign := ""
			switch s.Op {
			case OpFlat:
				if s.Value >= 0 {
					sign = "+"
				}
				out = append(out, fmt.Sprintf("%s%s (%s) → %s", sign, trimFloat(s.Value), s.Source, trimFloat(s.Result)))
			case OpPct:
				if s.Value >= 0 {
					sign = "+"
				}
				out = append(out, fmt.Sprintf("%s%s%% (%s) → %s", sign, trimFloat(s.Value), s.Source, trimFloat(s.Result)))
			case OpMult:
				out = append(out, fmt.Sprintf("×%s (%s) → %s", trimFloat(s.Value), s.Source, trimFloat(s.Result)))
			}
		}
	}
	return out
}

// Block — набор характеристик конкретного экземпляра (юнита).
// Базовые значения и безусловные модификаторы кэшируются; кэш сбрасывается при изменении.
type Block struct {
	reg   *Registry
	base  map[string]float64
	perm  []Modifier
	temp  []Modifier
	cache map[string]float64
}

// NewBlock создаёт блок с базовыми значениями (отсутствующие берутся из определений).
func NewBlock(reg *Registry, base map[string]float64) *Block {
	b := &Block{reg: reg, base: map[string]float64{}, cache: map[string]float64{}}
	for k, v := range base {
		b.base[k] = v
	}
	return b
}

// Registry возвращает реестр блока.
func (b *Block) Registry() *Registry { return b.reg }

// SetPermanent заменяет постоянные модификаторы.
func (b *Block) SetPermanent(mods []Modifier) {
	b.perm = append(b.perm[:0], mods...)
	for i := range b.perm {
		b.perm[i].Layer = Permanent
	}
	b.invalidate()
}

// SetTemporary заменяет временные модификаторы (от активных эффектов).
func (b *Block) SetTemporary(mods []Modifier) {
	b.temp = append(b.temp[:0], mods...)
	for i := range b.temp {
		b.temp[i].Layer = Temporary
	}
	b.invalidate()
}

// SetBase меняет базовое значение.
func (b *Block) SetBase(id string, v float64) {
	b.base[id] = v
	b.invalidate()
}

func (b *Block) invalidate() {
	for k := range b.cache {
		delete(b.cache, k)
	}
}

// BaseOf — базовое значение.
func (b *Block) BaseOf(id string) float64 {
	if v, ok := b.base[id]; ok {
		return v
	}
	if d, ok := b.reg.Get(id); ok {
		return d.Base
	}
	return 0
}

// Get — значение без ситуативных модификаторов (кэшируется).
func (b *Block) Get(id string) float64 {
	if v, ok := b.cache[id]; ok {
		return v
	}
	v := b.compute(id, nil, nil)
	b.cache[id] = v
	return v
}

// GetCtx — значение с учётом ситуативных модификаторов.
func (b *Block) GetCtx(id string, ctx *Context) float64 {
	if ctx == nil {
		return b.Get(id)
	}
	return b.compute(id, ctx, nil)
}

// Explain — значение с объяснением всех шагов.
func (b *Block) Explain(id string, ctx *Context) Breakdown {
	bd := Breakdown{Stat: id, Name: b.reg.Name(id)}
	bd.Final = b.compute(id, ctx, &bd)
	bd.Base = b.BaseOf(id)
	return bd
}

// Modifiers — все модификаторы указанной характеристики (для интерфейса).
func (b *Block) Modifiers(id string) []Modifier {
	var out []Modifier
	for _, m := range b.perm {
		if m.Stat == id {
			out = append(out, m)
		}
	}
	for _, m := range b.temp {
		if m.Stat == id {
			out = append(out, m)
		}
	}
	return out
}

func (b *Block) compute(id string, ctx *Context, bd *Breakdown) float64 {
	v := b.BaseOf(id)
	rec := func(stage, src string, op Op, val float64) {
		if bd != nil {
			bd.Steps = append(bd.Steps, Step{Stage: stage, Source: src, Op: op, Value: val, Result: v})
		}
	}
	rec("base", "", "", 0)

	// Слой «постоянные»: безусловные постоянные модификаторы.
	// Ситуативные модификаторы (с условием) применяются вместе с временными, независимо от источника.
	var permFlat, permPct, permMult, tmpFlat, tmpPct, tmpMult []Modifier
	add := func(m Modifier) {
		if m.Stat != id {
			return
		}
		if m.Situational() {
			if !m.When.Match(ctx) {
				return
			}
			switch m.Op {
			case OpFlat:
				tmpFlat = append(tmpFlat, m)
			case OpPct:
				tmpPct = append(tmpPct, m)
			case OpMult:
				tmpMult = append(tmpMult, m)
			}
			return
		}
		perm := m.Layer == Permanent
		switch m.Op {
		case OpFlat:
			if perm {
				permFlat = append(permFlat, m)
			} else {
				tmpFlat = append(tmpFlat, m)
			}
		case OpPct:
			if perm {
				permPct = append(permPct, m)
			} else {
				tmpPct = append(tmpPct, m)
			}
		case OpMult:
			if perm {
				permMult = append(permMult, m)
			} else {
				tmpMult = append(tmpMult, m)
			}
		}
	}
	for _, m := range b.perm {
		add(m)
	}
	for _, m := range b.temp {
		add(m)
	}
	applyFlat := func(stage string, ms []Modifier) {
		for _, m := range ms {
			v += m.Value
			rec(stage, m.Source, OpFlat, m.Value)
		}
	}
	applyPct := func(stage string, ms []Modifier) {
		if len(ms) == 0 {
			return
		}
		sum := 0.0
		var srcs []string
		for _, m := range ms {
			sum += m.Value
			srcs = append(srcs, m.Source)
		}
		v = v * (1 + sum/100)
		rec(stage, strings.Join(srcs, ", "), OpPct, sum)
	}
	applyMult := func(stage string, ms []Modifier) {
		for _, m := range ms {
			v *= m.Value
			rec(stage, m.Source, OpMult, m.Value)
		}
	}
	applyFlat("perm_flat", permFlat)
	applyPct("perm_pct", permPct)
	applyMult("perm_mult", permMult)
	applyFlat("temp_flat", tmpFlat)
	applyPct("temp_pct", tmpPct)
	applyMult("temp_mult", tmpMult)

	if d, ok := b.reg.Get(id); ok {
		c := v
		if d.Min != nil && c < *d.Min {
			c = *d.Min
		}
		if d.Max != nil && c > *d.Max {
			c = *d.Max
		}
		if c != v {
			v = c
			rec("clamp", "", "", 0)
		}
	}
	return v
}

// SortedIDs — идентификаторы с базовыми значениями (для отладки/сохранения).
func (b *Block) SortedIDs() []string {
	ids := make([]string, 0, len(b.base))
	for k := range b.base {
		ids = append(ids, k)
	}
	sort.Strings(ids)
	return ids
}
