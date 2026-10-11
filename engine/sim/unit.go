package sim

import (
	"hextactics/engine/data"
	"hextactics/engine/gamemap"
	"hextactics/engine/hex"
	"hextactics/engine/stats"
)

// UnitState — наблюдаемое состояние юнита (отдельно от приказа).
type UnitState string

const (
	StateIdle        UnitState = "idle"
	StateMoving      UnitState = "moving"
	StateTurning     UnitState = "turning"
	StatePreparing   UnitState = "preparing"
	StateRecovering  UnitState = "recovering"
	StateChanneling  UnitState = "channeling"
	StateWaiting     UnitState = "waiting"
	StateGuarding    UnitState = "guarding"
	StateInteracting UnitState = "interacting"
	StateStunned     UnitState = "stunned"
	StateAsleep      UnitState = "asleep"
	StateDead        UnitState = "dead"
)

// StateName — русское название состояния.
func StateName(s UnitState) string {
	switch s {
	case StateIdle:
		return "бездействие"
	case StateMoving:
		return "движение"
	case StateTurning:
		return "поворот"
	case StatePreparing:
		return "подготовка умения"
	case StateRecovering:
		return "восстановление после действия"
	case StateChanneling:
		return "каналирование"
	case StateWaiting:
		return "ожидание"
	case StateGuarding:
		return "защитная стойка"
	case StateInteracting:
		return "взаимодействие"
	case StateStunned:
		return "оглушён"
	case StateAsleep:
		return "спит"
	case StateDead:
		return "погиб"
	}
	return string(s)
}

// ActiveEffect — экземпляр эффекта на юните.
type ActiveEffect struct {
	Def       string `json:"def"`
	Source    int    `json:"source,omitempty"`
	Remaining int    `json:"remaining"` // тиков; -1 — бессрочно
	Total     int    `json:"total"`
	Stacks    int    `json:"stacks"`
	NextTick  int    `json:"next_tick"`  // тиков до периодического действия
	AuraTimer int    `json:"aura_timer"` // тиков до обновления ауры
	Absorb    int    `json:"absorb,omitempty"`
	Order     int    `json:"order,omitempty"` // эффект, привязанный к приказу (защитная стойка)
}

// ActionKind — низкоуровневое действие юнита, занимающее время.
type ActionKind string

const (
	ActMove     ActionKind = "move"
	ActTurn     ActionKind = "turn"
	ActPrep     ActionKind = "prep"
	ActRecover  ActionKind = "recover"
	ActChannel  ActionKind = "channel"
	ActWait     ActionKind = "wait"
	ActInteract ActionKind = "interact"
)

// Target — разрешённая цель умения.
type Target struct {
	Unit int     `json:"unit,omitempty"`
	Hex  hex.Hex `json:"hex"`
	Dir  hex.Dir `json:"dir"`
}

// Action — текущее действие (движение на клетку, поворот, подготовка удара и т.п.).
type Action struct {
	Kind          ActionKind `json:"kind"`
	Start         int        `json:"start"`
	End           int        `json:"end"`
	From          hex.Hex    `json:"from"`
	To            hex.Hex    `json:"to"`
	FromDir       hex.Dir    `json:"from_dir"`
	Dir           hex.Dir    `json:"dir"`
	Switched      bool       `json:"switched,omitempty"`
	Skill         string     `json:"skill,omitempty"`
	Target        Target     `json:"target"`
	Interruptible bool       `json:"interruptible,omitempty"`
	NextTick      int        `json:"next_tick,omitempty"`
	Cost          int        `json:"cost,omitempty"`
}

// Progress — доля выполнения действия на тике tick (0..1).
func (a *Action) Progress(tick int) float64 {
	if a.End <= a.Start {
		return 1
	}
	p := float64(tick-a.Start) / float64(a.End-a.Start)
	if p < 0 {
		return 0
	}
	if p > 1 {
		return 1
	}
	return p
}

// Unit — экземпляр юнита. Определение класса хранится в данных (Class — ссылка по ID).
// Экспортируемые поля — сериализуемое состояние; кэш восстанавливается после загрузки.
type Unit struct {
	ID        int                `json:"id"`
	Name      string             `json:"name"`
	Class     string             `json:"class"`
	Side      int                `json:"side"`
	Pos       hex.Hex            `json:"pos"`
	Facing    hex.Dir            `json:"facing"`
	HP        int                `json:"hp"`
	AP        int                `json:"ap"` // милли-ОД
	Res       map[string]int     `json:"res,omitempty"`
	Acc       map[string]int     `json:"acc,omitempty"` // накопители дробной регенерации
	Effects   []*ActiveEffect    `json:"effects,omitempty"`
	Cooldowns map[string]int     `json:"cooldowns,omitempty"` // умение → тик готовности
	Queue     []*Order           `json:"queue,omitempty"`
	Current   *Order             `json:"current,omitempty"`
	Action    *Action            `json:"action,omitempty"`
	Dead      bool               `json:"dead,omitempty"`
	Summoned  bool               `json:"summoned,omitempty"`
	Once      map[string]bool    `json:"once,omitempty"`       // использованные одноразовые реакции
	TrigReady map[string]int     `json:"trig_ready,omitempty"` // перезарядка реакций
	LastIssue *Issue             `json:"last_issue,omitempty"` // причина последней остановки
	LastInfo  string             `json:"last_info,omitempty"`
	PlanVer   int                `json:"-"` // версия очереди для кэша проекции (не игровое состояние)
	Threat    *ThreatMemory      `json:"threat,omitempty"`
	Extra     map[string]float64 `json:"extra,omitempty"` // расширяемые числовые поля

	class    *data.UnitClassDef
	passives []*data.PassiveDef
	stats    *stats.Block
	tags     map[string]bool
}

// ThreatMemory — последний атаковавший (для реакции ИИ на невидимого врага).
type ThreatMemory struct {
	From hex.Hex `json:"from"`
	Tick int     `json:"tick"`
}

// ClassDef — определение класса.
func (u *Unit) ClassDef() *data.UnitClassDef { return u.class }

// Passives — пассивные навыки.
func (u *Unit) Passives() []*data.PassiveDef { return u.passives }

// Stats — блок характеристик.
func (u *Unit) Stats() *stats.Block { return u.stats }

// HasTag — есть ли тег (из класса или пассивных навыков).
func (u *Unit) HasTag(t string) bool { return u.tags[t] }

// Alive — юнит жив.
func (u *Unit) Alive() bool { return !u.Dead }

// Stat — значение характеристики без ситуативных модификаторов.
func (u *Unit) Stat(id string) float64 { return u.stats.Get(id) }

// HPMax — максимальное здоровье.
func (u *Unit) HPMax() int { return int(u.stats.Get("hp_max") + 0.5) }

// APMax — максимум ОД (милли).
func (u *Unit) APMax() int { return data.AP(u.stats.Get("ap_max")) }

// ResMax — максимум ресурса (милли).
func (w *World) ResMax(u *Unit, res string) int {
	rd := w.C.Resources[res]
	if rd == nil {
		return 0
	}
	return int(u.stats.Get(rd.MaxStat)*data.ResScale + 0.5)
}

// HasFlag — есть ли у юнита состояние-флаг от эффектов (stunned, rooted, asleep, guarding).
func (w *World) HasFlag(u *Unit, flag string) bool {
	for _, e := range u.Effects {
		if d := w.C.Effects[e.Def]; d != nil && d.HasFlag(flag) {
			return true
		}
	}
	return false
}

// Incapacitated — оглушён или спит.
func (w *World) Incapacitated(u *Unit) (bool, Code) {
	if w.HasFlag(u, "stunned") {
		return true, ErrStunned
	}
	if w.HasFlag(u, "asleep") {
		return true, ErrAsleep
	}
	return false, CodeOK
}

// State — наблюдаемое состояние юнита.
func (w *World) State(u *Unit) UnitState {
	if u.Dead {
		return StateDead
	}
	if w.HasFlag(u, "asleep") {
		return StateAsleep
	}
	if w.HasFlag(u, "stunned") {
		return StateStunned
	}
	if u.Action != nil {
		switch u.Action.Kind {
		case ActMove:
			return StateMoving
		case ActTurn:
			return StateTurning
		case ActPrep:
			return StatePreparing
		case ActRecover:
			return StateRecovering
		case ActChannel:
			return StateChanneling
		case ActWait:
			return StateWaiting
		case ActInteract:
			return StateInteracting
		}
	}
	if u.Current != nil {
		if u.Current.Type == OrderGuard {
			return StateGuarding
		}
		if u.Current.Status == StatusWaiting {
			return StateWaiting
		}
	}
	return StateIdle
}

// MoveProfile — профиль перемещения юнита (стоимость шага, класс, переопределения пассивок).
func (w *World) MoveProfile(u *Unit, keepFacing bool, facing hex.Dir) *gamemap.MoveProfile {
	p := &gamemap.MoveProfile{
		Class:     u.class.MoveClass,
		StepCost:  data.AP(u.stats.Get("move_cost")),
		ClimbCost: data.AP(w.C.Rules.ClimbCostAP),
		MaxClimb:  w.C.Rules.MaxClimb,
	}
	for _, ps := range u.passives {
		for t, v := range ps.MoveCost {
			if p.TerrainOverride == nil {
				p.TerrainOverride = map[string]int{}
			}
			p.TerrainOverride[t] = v
		}
	}
	if keepFacing {
		p.KeepFacing = true
		p.Facing = facing
		p.StrafePct = w.C.Rules.StrafeCostPct
	}
	return p
}

// rebuildUnitCache восстанавливает ссылки на определения и характеристики.
func (w *World) rebuildUnitCache(u *Unit) {
	u.class = w.C.Classes[u.Class]
	u.passives = nil
	u.tags = map[string]bool{}
	for _, t := range u.class.Tags {
		u.tags[t] = true
	}
	var perm []stats.Modifier
	for _, pid := range u.class.Passives {
		p := w.C.Passives[pid]
		u.passives = append(u.passives, p)
		for _, m := range p.Modifiers {
			m.Source = "пассивный: " + p.Name
			perm = append(perm, m)
		}
		for _, t := range p.Tags {
			u.tags[t] = true
		}
	}
	u.stats = stats.NewBlock(w.C.Stats, u.class.Stats)
	u.stats.SetPermanent(perm)
	w.refreshEffectMods(u)
}
