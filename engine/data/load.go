package data

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"path"
	"sort"
	"strings"

	"hextactics/engine/stats"
)

//go:embed content
var embedded embed.FS

// Embedded возвращает встроенный набор данных (папка content).
func Embedded() fs.FS {
	sub, err := fs.Sub(embedded, "content")
	if err != nil {
		panic(err)
	}
	return sub
}

// Content — все загруженные определения.
type Content struct {
	Rules         Rules
	StatDefs      []stats.Def
	Stats         *stats.Registry
	Resources     map[string]*ResourceDef
	ResourceOrder []string
	DamageTypes   map[string]*DamageTypeDef
	Terrains      map[string]*TerrainDef
	Effects       map[string]*EffectDef
	Skills        map[string]*SkillDef
	Passives      map[string]*PassiveDef
	Objects       map[string]*ObjectDef
	Classes       map[string]*UnitClassDef
	Scenarios     map[string]*ScenarioDef
	ScenarioOrder []string
}

// Ticks переводит секунды игрового времени в тики симуляции (ненулевая длительность ≥ 1 тика).
func (c *Content) Ticks(sec float64) int {
	if sec <= 0 {
		return 0
	}
	t := int(math.Round(sec * float64(c.Rules.TicksPerSecond)))
	if t < 1 {
		t = 1
	}
	return t
}

// Seconds переводит тики в секунды.
func (c *Content) Seconds(ticks int) float64 {
	return float64(ticks) / float64(c.Rules.TicksPerSecond)
}

func decodeStrict(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func readJSON(fsys fs.FS, name string, v any) error {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if err := decodeStrict(b, v); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// Load загружает определения из файловой системы (встроенной или папки data рядом с игрой).
func Load(fsys fs.FS) (*Content, error) {
	c := &Content{
		Resources:   map[string]*ResourceDef{},
		DamageTypes: map[string]*DamageTypeDef{},
		Terrains:    map[string]*TerrainDef{},
		Effects:     map[string]*EffectDef{},
		Skills:      map[string]*SkillDef{},
		Passives:    map[string]*PassiveDef{},
		Objects:     map[string]*ObjectDef{},
		Classes:     map[string]*UnitClassDef{},
		Scenarios:   map[string]*ScenarioDef{},
	}
	if err := readJSON(fsys, "rules.json", &c.Rules); err != nil {
		return nil, err
	}
	if err := readJSON(fsys, "stats.json", &c.StatDefs); err != nil {
		return nil, err
	}
	reg, err := stats.NewRegistry(c.StatDefs)
	if err != nil {
		return nil, fmt.Errorf("stats.json: %w", err)
	}
	c.Stats = reg

	var resources []ResourceDef
	var dtypes []DamageTypeDef
	var terrains []TerrainDef
	var effects []EffectDef
	var skills []SkillDef
	var passives []PassiveDef
	var objects []ObjectDef
	var classes []UnitClassDef
	for _, f := range []struct {
		name string
		v    any
	}{
		{"resources.json", &resources}, {"damage_types.json", &dtypes}, {"terrain.json", &terrains},
		{"effects.json", &effects}, {"skills.json", &skills}, {"passives.json", &passives},
		{"objects.json", &objects}, {"units.json", &classes},
	} {
		if err := readJSON(fsys, f.name, f.v); err != nil {
			return nil, err
		}
	}
	var errs []string
	dup := func(kind, id string) { errs = append(errs, fmt.Sprintf("%s: повтор id %q", kind, id)) }
	for i := range resources {
		r := &resources[i]
		if _, ok := c.Resources[r.ID]; ok {
			dup("resources", r.ID)
		}
		c.Resources[r.ID] = r
		c.ResourceOrder = append(c.ResourceOrder, r.ID)
	}
	for i := range dtypes {
		d := &dtypes[i]
		if _, ok := c.DamageTypes[d.ID]; ok {
			dup("damage_types", d.ID)
		}
		c.DamageTypes[d.ID] = d
	}
	for i := range terrains {
		t := &terrains[i]
		if _, ok := c.Terrains[t.ID]; ok {
			dup("terrain", t.ID)
		}
		c.Terrains[t.ID] = t
	}
	for i := range effects {
		e := &effects[i]
		if _, ok := c.Effects[e.ID]; ok {
			dup("effects", e.ID)
		}
		c.Effects[e.ID] = e
	}
	for i := range skills {
		s := &skills[i]
		if _, ok := c.Skills[s.ID]; ok {
			dup("skills", s.ID)
		}
		c.Skills[s.ID] = s
	}
	for i := range passives {
		p := &passives[i]
		if _, ok := c.Passives[p.ID]; ok {
			dup("passives", p.ID)
		}
		c.Passives[p.ID] = p
	}
	for i := range objects {
		o := &objects[i]
		if _, ok := c.Objects[o.ID]; ok {
			dup("objects", o.ID)
		}
		c.Objects[o.ID] = o
	}
	for i := range classes {
		u := &classes[i]
		if _, ok := c.Classes[u.ID]; ok {
			dup("units", u.ID)
		}
		c.Classes[u.ID] = u
	}

	// Сценарии: все файлы scenarios/*.json
	entries, err := fs.ReadDir(fsys, "scenarios")
	if err != nil {
		return nil, fmt.Errorf("scenarios: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var sc ScenarioDef
		if err := readJSON(fsys, path.Join("scenarios", e.Name()), &sc); err != nil {
			return nil, err
		}
		if _, ok := c.Scenarios[sc.ID]; ok {
			dup("scenarios", sc.ID)
		}
		c.Scenarios[sc.ID] = &sc
		c.ScenarioOrder = append(c.ScenarioOrder, sc.ID)
	}
	sort.Strings(c.ScenarioOrder)

	errs = append(errs, c.validate()...)
	if len(errs) > 0 {
		return nil, errors.New("ошибки в данных:\n  " + strings.Join(errs, "\n  "))
	}
	return c, nil
}

// LoadEmbedded загружает встроенные данные.
func LoadEmbedded() (*Content, error) { return Load(Embedded()) }

// validate проверяет перекрёстные ссылки, чтобы ошибки в данных обнаруживались при запуске.
func (c *Content) validate() []string {
	var errs []string
	bad := func(format string, a ...any) { errs = append(errs, fmt.Sprintf(format, a...)) }
	r := c.Rules
	if r.TicksPerSecond <= 0 {
		bad("rules: ticks_per_second должен быть > 0")
	}
	if r.APModel != "continuous" && r.APModel != "cycle" {
		bad("rules: ap_model должен быть continuous или cycle")
	}
	if r.QueueMode != "strict" && r.QueueMode != "flexible" {
		bad("rules: queue_mode должен быть strict или flexible")
	}
	for k, v := range r.FailPolicies {
		switch v {
		case "abort", "skip", "repath", "wait":
		default:
			bad("rules: неизвестная политика %q для %q", v, k)
		}
	}
	if r.GuardEffect != "" && c.Effects[r.GuardEffect] == nil {
		bad("rules: guard_effect %q не найден", r.GuardEffect)
	}
	stat := func(where, id string) {
		if id != "" && !c.Stats.Has(id) {
			bad("%s: неизвестная характеристика %q", where, id)
		}
	}
	mods := func(where string, ms []stats.Modifier) {
		for _, m := range ms {
			stat(where, m.Stat)
			switch m.Op {
			case stats.OpFlat, stats.OpPct, stats.OpMult:
			default:
				bad("%s: неизвестный тип модификатора %q", where, m.Op)
			}
		}
	}
	var ops func(where string, list []OpDef)
	ops = func(where string, list []OpDef) {
		for _, o := range list {
			switch o.Op {
			case "damage":
				if c.DamageTypes[o.DamageType] == nil {
					bad("%s: неизвестный тип урона %q", where, o.DamageType)
				}
				stat(where, o.Stat)
			case "heal":
				stat(where, o.Stat)
			case "apply_effect", "remove_effect":
				if c.Effects[o.Effect] == nil {
					bad("%s: неизвестный эффект %q", where, o.Effect)
				}
			case "restore":
				if o.Resource != "ap" && o.Resource != "hp" && c.Resources[o.Resource] == nil {
					bad("%s: неизвестный ресурс %q", where, o.Resource)
				}
			case "place_object":
				if c.Objects[o.Object] == nil {
					bad("%s: неизвестный объект %q", where, o.Object)
				}
			case "summon":
				if c.Classes[o.Class] == nil {
					bad("%s: неизвестный класс %q", where, o.Class)
				}
			default:
				bad("%s: неизвестная операция %q", where, o.Op)
			}
			switch o.Target {
			case "", "target", "self", "attacker", "source":
			default:
				bad("%s: неизвестная цель операции %q", where, o.Target)
			}
		}
	}
	for _, rd := range c.Resources {
		stat("resources/"+rd.ID, rd.MaxStat)
		stat("resources/"+rd.ID, rd.RegenStat)
	}
	for _, d := range c.DamageTypes {
		stat("damage_types/"+d.ID, d.ResistStat)
		stat("damage_types/"+d.ID, d.VulnStat)
	}
	for _, e := range c.Effects {
		w := "effects/" + e.ID
		mods(w, e.Modifiers)
		stat(w, e.ResistStat)
		if e.Periodic != nil {
			if e.Periodic.Interval <= 0 {
				bad("%s: interval должен быть > 0", w)
			}
			ops(w, e.Periodic.Ops)
		}
		if e.Aura != nil && c.Effects[e.Aura.Effect] == nil {
			bad("%s: аура ссылается на неизвестный эффект %q", w, e.Aura.Effect)
		}
		switch e.Stacking {
		case "", "refresh", "stack", "ignore":
		default:
			bad("%s: неизвестный stacking %q", w, e.Stacking)
		}
	}
	for _, s := range c.Skills {
		w := "skills/" + s.ID
		switch s.Targeting {
		case "self", "unit", "hex", "direction", "auto":
		default:
			bad("%s: неизвестный targeting %q", w, s.Targeting)
		}
		switch s.Shape.Type {
		case "single", "line", "cone", "radius", "arc", "self":
		default:
			bad("%s: неизвестная форма %q", w, s.Shape.Type)
		}
		if s.Shape.Type == "cone" && s.Shape.HalfAngle != 30 && s.Shape.HalfAngle != 60 {
			bad("%s: half_angle конуса должен быть 30 или 60", w)
		}
		for res := range s.Costs {
			if c.Resources[res] == nil {
				bad("%s: неизвестный ресурс %q", w, res)
			}
		}
		ops(w, s.Ops)
		if s.Channel != nil {
			ops(w+"/channel", s.Channel.Ops)
			if s.Channel.Interval <= 0 {
				bad("%s: channel.interval должен быть > 0", w)
			}
			if s.Channel.Effect != "" && c.Effects[s.Channel.Effect] == nil {
				bad("%s: channel.effect %q не найден", w, s.Channel.Effect)
			}
		}
		if s.APCost < 0 {
			bad("%s: отрицательная стоимость ОД", w)
		}
	}
	for _, p := range c.Passives {
		w := "passives/" + p.ID
		mods(w, p.Modifiers)
		for t := range p.MoveCost {
			if c.Terrains[t] == nil {
				bad("%s: неизвестная поверхность %q", w, t)
			}
		}
		for _, tr := range p.Triggers {
			switch tr.Event {
			case "blocked", "evaded", "damaged", "hit_dealt", "ally_died", "killed":
			default:
				bad("%s: неизвестное событие %q", w, tr.Event)
			}
			ops(w, tr.Ops)
		}
	}
	for _, o := range c.Objects {
		ops("objects/"+o.ID, o.Ops)
	}
	for _, u := range c.Classes {
		w := "units/" + u.ID
		for k := range u.Stats {
			stat(w, k)
		}
		for _, r := range u.Resources {
			if c.Resources[r] == nil {
				bad("%s: неизвестный ресурс %q", w, r)
			}
		}
		if u.BasicAttack != "" && c.Skills[u.BasicAttack] == nil {
			bad("%s: неизвестная базовая атака %q", w, u.BasicAttack)
		}
		for _, s := range u.Skills {
			if c.Skills[s] == nil {
				bad("%s: неизвестное умение %q", w, s)
			}
		}
		for _, p := range u.Passives {
			if c.Passives[p] == nil {
				bad("%s: неизвестный пассивный навык %q", w, p)
			}
		}
	}
	for _, sc := range c.Scenarios {
		w := "scenarios/" + sc.ID
		for ch, t := range sc.Legend {
			if len([]rune(ch)) != 1 {
				bad("%s: ключ легенды %q должен быть одним символом", w, ch)
			}
			if c.Terrains[t] == nil {
				bad("%s: легенда ссылается на неизвестную поверхность %q", w, t)
			}
		}
		for i, row := range sc.Rows {
			for _, ch := range row {
				if _, ok := sc.Legend[string(ch)]; !ok {
					bad("%s: строка %d содержит символ %q без легенды", w, i, string(ch))
					break
				}
			}
		}
		for _, u := range sc.Units {
			if c.Classes[u.Class] == nil {
				bad("%s: неизвестный класс %q", w, u.Class)
			}
			for _, e := range u.Effects {
				if c.Effects[e] == nil {
					bad("%s: неизвестный эффект %q", w, e)
				}
			}
		}
		if len(sc.Sides) == 0 {
			bad("%s: не заданы стороны", w)
		}
	}
	sort.Strings(errs)
	return errs
}

// ExportTo копирует встроенные данные в writeFile (для модификаций).
func ExportEmbedded(writeFile func(name string, data []byte) error) error {
	return fs.WalkDir(Embedded(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(Embedded(), p)
		if err != nil {
			return err
		}
		return writeFile(p, b)
	})
}
