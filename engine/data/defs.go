// Package data — определения игровых сущностей, загружаемые из JSON.
//
// Определения (класс юнита, умение, эффект, пассивный навык, объект, поверхность)
// отделены от экземпляров, которые создаёт симуляция. Добавление новой
// характеристики, умения или эффекта не требует изменения кода ядра — достаточно
// описать их в JSON и сослаться на существующие операции (Op) и формы (Shape).
package data

import (
	"image/color"
	"math"

	"hextactics/engine/stats"
)

// RGB — цвет в данных: [r, g, b].
type RGB [3]uint8

// Color преобразует в color.RGBA.
func (c RGB) Color() color.RGBA { return color.RGBA{c[0], c[1], c[2], 255} }

// ---------------------------------------------------------------------------
// Правила

// SectorProfile — коэффициенты по секторам: индекс = Delta (0 фронт, 1 ближний фланг,
// 2 дальний фланг, 3 тыл). Все значения — проценты.
type SectorProfile struct {
	Surprise          [4]int `json:"surprise"`           // базовая вероятность неожиданности
	Evasion           [4]int `json:"evasion"`            // доля базового уклонения
	FrontBlock        [4]int `json:"front_block"`        // эффективность блока
	PositionalDefense [4]int `json:"positional_defense"` // эффективность брони
}

// SurpriseRules — модификаторы обнаружения атаки.
type SurpriseRules struct {
	UnseenAttackerBonus int  `json:"unseen_attacker_bonus"` // атакующий невидим для стороны защищающегося
	DistractedBonus     int  `json:"distracted_bonus"`      // защищающийся связан боем с другим противником
	NegatesEvasion      bool `json:"negates_evasion"`       // неожиданная атака исключает уклонение
	NegatesBlock        bool `json:"negates_block"`         // неожиданная атака исключает блок
	CritBonus           int  `json:"crit_bonus"`            // + к шансу крита (не автокрит)
}

// Rules — глобальные правила симуляции. Альтернативы оставлены настраиваемыми.
type Rules struct {
	TicksPerSecond      int               `json:"ticks_per_second"`
	APModel             string            `json:"ap_model"` // continuous | cycle
	CycleSeconds        float64           `json:"cycle_seconds"`
	QueueMode           string            `json:"queue_mode"` // strict | flexible
	TurnFreeWhileMoving bool              `json:"turn_free_while_moving"`
	StrafeCostPct       int               `json:"strafe_cost_pct"`
	StrafeTimePct       int               `json:"strafe_time_pct"`
	AutoTurnToAttack    bool              `json:"auto_turn_to_attack"`
	AttackArcMaxDelta   int               `json:"attack_arc_max_delta"`
	InterruptOnDamage   bool              `json:"interrupt_on_damage"`
	ClimbCostAP         float64           `json:"climb_cost_ap"`
	MaxClimb            int               `json:"max_climb"`
	HeightHitBonus      int               `json:"height_hit_bonus"`
	HitMin              int               `json:"hit_min"`
	HitMax              int               `json:"hit_max"`
	EvasionMax          int               `json:"evasion_max"`
	BlockMax            int               `json:"block_max"`
	ResistMax           int               `json:"resist_max"`
	SoftCoverMax        int               `json:"soft_cover_max"`
	CoverAppliesTo      []string          `json:"cover_applies_to"`
	MinDamage           int               `json:"min_damage"`
	Sectors             SectorProfile     `json:"sectors"`
	Surprise            SurpriseRules     `json:"surprise"`
	FailPolicies        map[string]string `json:"fail_policies"` // тип приказа → политика по умолчанию
	WaitTimeout         float64           `json:"wait_timeout"`
	VisionUpdateTicks   int               `json:"vision_update_ticks"`
	AIThinkSeconds      float64           `json:"ai_think_seconds"`
	GuardEffect         string            `json:"guard_effect"`
	GuardSeconds        float64           `json:"guard_seconds"`
	InteractCostAP      float64           `json:"interact_cost_ap"`
	InteractTime        float64           `json:"interact_time"`
	MaxEventsPerTick    int               `json:"max_events_per_tick"`
	DangerousDamagePct  int               `json:"dangerous_damage_pct"` // урон ≥ % от макс. HP — «опасная атака»
}

// SectorKeys — ключи секторов по Delta.
var SectorKeys = [4]string{"front", "near_flank", "far_flank", "rear"}

// SectorNames — русские названия секторов по Delta.
var SectorNames = [4]string{"фронт", "ближний фланг", "дальний фланг", "тыл"}

// ---------------------------------------------------------------------------

// ResourceDef — расходуемый ресурс (мана, выносливость). Текущее значение хранится у юнита.
type ResourceDef struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	MaxStat   string `json:"max_stat"`
	RegenStat string `json:"regen_stat"`
	Color     RGB    `json:"color"`
}

// DamageTypeDef — тип урона.
type DamageTypeDef struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Armor      bool   `json:"armor"`       // учитывается ли броня
	ResistStat string `json:"resist_stat"` // сопротивление (снижает урон)
	VulnStat   string `json:"vuln_stat"`   // уязвимость (повышает урон)
	Color      RGB    `json:"color"`
}

// TerrainDef — тип поверхности клетки.
type TerrainDef struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Color     RGB            `json:"color"`
	Pattern   string         `json:"pattern,omitempty"`   // визуальный узор
	MoveCost  map[string]int `json:"move_cost"`           // класс передвижения → % стоимости; нет ключа — непроходимо
	BlocksLOS bool           `json:"blocks_los"`          // полностью перекрывает обзор
	LOSCover  int            `json:"los_cover,omitempty"` // частичное препятствие на линии выстрела, %
	Cover     int            `json:"cover,omitempty"`     // укрытие для стоящего в клетке от дальних атак, %
	Biome     string         `json:"biome,omitempty"`
	Desc      string         `json:"desc,omitempty"`
}

// ---------------------------------------------------------------------------
// Операции

// OpDef — универсальная операция, из которых собираются умения, эффекты, ловушки и реакции.
//
// Поддерживаемые op: damage, heal, apply_effect, remove_effect, restore, place_object, summon.
type OpDef struct {
	Op         string  `json:"op"`
	Target     string  `json:"target,omitempty"` // target (по умолчанию) | self | attacker | source
	DamageType string  `json:"type,omitempty"`
	Base       float64 `json:"base,omitempty"`
	Stat       string  `json:"stat,omitempty"`  // характеристика источника
	Scale      float64 `json:"scale,omitempty"` // % от характеристики
	Variance   int     `json:"variance,omitempty"`
	Effect     string  `json:"effect,omitempty"`
	Duration   float64 `json:"duration,omitempty"`
	Chance     int     `json:"chance,omitempty"` // 0 = 100%
	Resource   string  `json:"resource,omitempty"`
	Amount     float64 `json:"amount,omitempty"`
	Object     string  `json:"object,omitempty"`
	Class      string  `json:"class,omitempty"`
	Count      int     `json:"count,omitempty"`
}

// ---------------------------------------------------------------------------
// Эффекты

// Periodic — периодическое действие эффекта.
type Periodic struct {
	Interval float64 `json:"interval"`
	Ops      []OpDef `json:"ops"`
}

// Redirect — перенаправление части урона источнику эффекта.
type Redirect struct {
	Pct   int `json:"pct"`
	Range int `json:"range"`
}

// Aura — аура: периодически накладывает эффект на юнитов в радиусе.
type Aura struct {
	Radius   int     `json:"radius"`
	Effect   string  `json:"effect"`
	Affects  string  `json:"affects"` // allies | enemies | all
	Interval float64 `json:"interval"`
}

// EffectDef — определение эффекта (активный эффект — экземпляр у юнита).
type EffectDef struct {
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	Desc          string           `json:"desc,omitempty"`
	Glyph         string           `json:"glyph,omitempty"`
	Color         RGB              `json:"color"`
	Kind          string           `json:"kind"` // buff | debuff
	Tags          []string         `json:"tags,omitempty"`
	Flags         []string         `json:"flags,omitempty"` // stunned | rooted | asleep | silenced | guarding
	Duration      float64          `json:"duration"`
	Stacking      string           `json:"stacking,omitempty"` // refresh (по умолчанию) | stack | ignore
	MaxStacks     int              `json:"max_stacks,omitempty"`
	Modifiers     []stats.Modifier `json:"modifiers,omitempty"`
	Periodic      *Periodic        `json:"periodic,omitempty"`
	Absorb        int              `json:"absorb,omitempty"`
	Redirect      *Redirect        `json:"redirect,omitempty"`
	Aura          *Aura            `json:"aura,omitempty"`
	BreakOnDamage bool             `json:"break_on_damage,omitempty"`
	ResistStat    string           `json:"resist_stat,omitempty"` // шанс сопротивления наложению
}

// HasFlag — есть ли флаг состояния.
func (e *EffectDef) HasFlag(f string) bool {
	for _, x := range e.Flags {
		if x == f {
			return true
		}
	}
	return false
}

// HasTag — есть ли тег.
func (e *EffectDef) HasTag(t string) bool {
	for _, x := range e.Tags {
		if x == t {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Умения

// Shape — форма области поражения.
type Shape struct {
	Type       string `json:"type"`                   // single | line | cone | radius | arc | self
	Size       int    `json:"size,omitempty"`         // длина/радиус
	HalfAngle  int    `json:"half_angle,omitempty"`   // для конуса: 30 или 60
	StopAtWall bool   `json:"stop_at_wall,omitempty"` // линия обрывается на препятствии
}

// Channel — параметры каналируемого умения.
type Channel struct {
	Duration float64 `json:"duration"`
	Interval float64 `json:"interval"`
	Effect   string  `json:"effect,omitempty"` // эффект на время каналирования
	Ops      []OpDef `json:"ops"`
}

// SkillDef — определение умения (конкретный приказ ссылается на него по ID).
type SkillDef struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Desc           string             `json:"desc,omitempty"`
	Icon           string             `json:"icon,omitempty"`
	Kind           string             `json:"kind"` // для отображения и ИИ
	Color          RGB                `json:"color"`
	APCost         float64            `json:"ap_cost"`
	Costs          map[string]float64 `json:"costs,omitempty"`
	Prep           float64            `json:"prep,omitempty"`
	Recovery       float64            `json:"recovery,omitempty"`
	Cooldown       float64            `json:"cooldown,omitempty"`
	Targeting      string             `json:"targeting"`               // self | unit | hex | direction | auto
	TargetFilter   string             `json:"target_filter,omitempty"` // enemy | ally | ally_or_self | any | empty_hex
	RangeMin       int                `json:"range_min,omitempty"`
	Range          int                `json:"range,omitempty"`
	LOS            bool               `json:"los,omitempty"`
	Shape          Shape              `json:"shape"`
	Affects        string             `json:"affects,omitempty"` // enemies | allies | all | self
	RequiresFacing bool               `json:"requires_facing,omitempty"`
	Attack         bool               `json:"attack,omitempty"`      // бросок попадания по каждой цели
	AttackKind     string             `json:"attack_kind,omitempty"` // melee | ranged | spell
	AccuracyMod    int                `json:"accuracy_mod,omitempty"`
	Unevadable     bool               `json:"unevadable,omitempty"`
	Unblockable    bool               `json:"unblockable,omitempty"`
	Interruptible  bool               `json:"interruptible,omitempty"`
	Dangerous      bool               `json:"dangerous,omitempty"`
	Dash           int                `json:"dash,omitempty"` // умение по маршруту: рывок до N клеток перед ударом
	Ops            []OpDef            `json:"ops,omitempty"`
	Channel        *Channel           `json:"channel,omitempty"`
	NoApproach     bool               `json:"no_approach,omitempty"`
	AIHint         string             `json:"ai,omitempty"` // basic | aoe | control | heal | buff | summon | trap | channel | none
	AIMinTargets   int                `json:"ai_min_targets,omitempty"`
}

// ---------------------------------------------------------------------------
// Пассивные навыки

// Trigger — реакция пассивного навыка на игровое событие.
type Trigger struct {
	Event       string   `json:"event"` // blocked | evaded | damaged | hit_dealt | ally_died | killed
	Chance      int      `json:"chance,omitempty"`
	HPBelowPct  int      `json:"hp_below_pct,omitempty"`
	Radius      int      `json:"radius,omitempty"`
	AttackKinds []string `json:"attack_kinds,omitempty"`
	Cooldown    float64  `json:"cooldown,omitempty"`
	Once        bool     `json:"once,omitempty"`
	Ops         []OpDef  `json:"ops"`
}

// PassiveDef — пассивный навык.
type PassiveDef struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Desc      string           `json:"desc,omitempty"`
	Modifiers []stats.Modifier `json:"modifiers,omitempty"`
	MoveCost  map[string]int   `json:"move_cost,omitempty"` // поверхность → % стоимости (переопределение)
	Tags      []string         `json:"tags,omitempty"`      // например, immune_effect:control
	Triggers  []Trigger        `json:"triggers,omitempty"`
}

// ---------------------------------------------------------------------------
// Объекты мира

// ObjectDef — ловушка, отложенный удар или иной объект.
type ObjectDef struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	Glyph             string  `json:"glyph,omitempty"`
	Color             RGB     `json:"color"`
	Kind              string  `json:"kind"`    // trap | delayed
	Trigger           string  `json:"trigger"` // enemy_enter | any_enter | timer
	Delay             float64 `json:"delay,omitempty"`
	Radius            int     `json:"radius,omitempty"`
	Lifetime          float64 `json:"lifetime,omitempty"`
	HiddenFromEnemies bool    `json:"hidden_from_enemies,omitempty"`
	Dangerous         bool    `json:"dangerous,omitempty"`
	Ops               []OpDef `json:"ops"`
}

// ---------------------------------------------------------------------------
// Юниты

// UnitClassDef — определение класса юнита. Экземпляры хранят только текущие параметры.
type UnitClassDef struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Glyph       string             `json:"glyph"`
	Desc        string             `json:"desc,omitempty"`
	MoveClass   string             `json:"move_class"`
	Important   bool               `json:"important,omitempty"`
	Stats       map[string]float64 `json:"stats"`
	Resources   []string           `json:"resources,omitempty"`
	BasicAttack string             `json:"basic_attack,omitempty"`
	Skills      []string           `json:"skills,omitempty"`
	Passives    []string           `json:"passives,omitempty"`
	Tags        []string           `json:"tags,omitempty"`
	AI          string             `json:"ai,omitempty"` // melee | ranged | caster | support | none
}

// ---------------------------------------------------------------------------
// Сценарии

// SideDef — сторона конфликта.
type SideDef struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Color      RGB    `json:"color"`
	Controller string `json:"controller"` // player | ai
}

// FeatureDef — интерактивный элемент карты (дверь, рычаг).
type FeatureDef struct {
	ID    string   `json:"id"`
	Kind  string   `json:"kind"` // door | lever
	Col   int      `json:"col"`
	Row   int      `json:"row"`
	Open  bool     `json:"open,omitempty"`
	Links []string `json:"links,omitempty"` // рычаг → двери
}

// UnitPlacement — юнит в сценарии.
type UnitPlacement struct {
	Class   string   `json:"class"`
	Name    string   `json:"name"`
	Side    int      `json:"side"`
	Col     int      `json:"col"`
	Row     int      `json:"row"`
	Facing  int      `json:"facing"`
	Effects []string `json:"effects,omitempty"` // начальные эффекты (например, сон)
}

// ScenarioDef — карта и расстановка.
type ScenarioDef struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Desc     string            `json:"desc,omitempty"`
	Seed     uint64            `json:"seed"`
	Legend   map[string]string `json:"legend"`
	Rows     []string          `json:"rows"`
	Heights  []string          `json:"heights,omitempty"`
	Features []FeatureDef      `json:"features,omitempty"`
	Sides    []SideDef         `json:"sides"`
	Units    []UnitPlacement   `json:"units"`
}

// ---------------------------------------------------------------------------
// Единицы измерения

// APScale — внутренних единиц в одном ОД (целочисленные «милли-ОД»).
const APScale = 1000

// AP переводит ОД из данных во внутренние единицы.
func AP(x float64) int { return int(math.Round(x * APScale)) }

// APf переводит внутренние единицы в ОД.
func APf(x int) float64 { return float64(x) / APScale }

// ResScale — внутренних единиц ресурса в одной единице.
const ResScale = 1000
