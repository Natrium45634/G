// Package sim — игровая симуляция: мир, фиксированный шаг времени, юниты, ОД,
// очереди приказов и их исполнение, бой, умения, эффекты, пассивные навыки,
// видимость, ИИ, команды и сохранение.
//
// Симуляция ничего не знает об интерфейсе. Внешний мир (игрок, ИИ, тесты)
// изменяет её только через World.Apply(Command) и продвигает время через Step
// (обычно — через Clock). Все расчёты целочисленные или с фиксированным порядком
// операций, поэтому при одинаковом зерне и командах результат воспроизводим.
package sim

import (
	"fmt"
	"sort"

	"hextactics/engine/data"
	"hextactics/engine/gamemap"
	"hextactics/engine/hex"
	"hextactics/engine/rng"
)

// LoggedCommand — команда с номером тика (журнал для воспроизведения).
type LoggedCommand struct {
	Tick int     `json:"tick"`
	Cmd  Command `json:"cmd"`
}

// World — состояние боя.
type World struct {
	C        *data.Content
	Scenario *data.ScenarioDef
	Map      *gamemap.Map
	Sides    []data.SideDef

	Tick        int
	RNG         rng.RNG
	Units       []*Unit
	Objects     []*Object
	NextUnitID  int
	NextOrderID int
	NextObjID   int
	QueueMode   string
	Outcome     string // "", "victory", "defeat"
	Known       map[int]map[int]map[string]bool

	Events     []Event
	eventSeq   uint64
	dispatched uint64
	CommandLog []LoggedCommand

	byID       map[int]*Unit
	occ        map[hex.Hex]int
	resv       map[hex.Hex]int
	OccVersion uint64
	visible    []map[int]bool
	visDirty   bool
	undo       map[int][][]*Order
	planCache  map[int]*planCacheEntry
}

// New создаёт мир по сценарию. seed = 0 — использовать зерно сценария.
func New(c *data.Content, scenarioID string, seed uint64) (*World, error) {
	sc := c.Scenarios[scenarioID]
	if sc == nil {
		return nil, fmt.Errorf("сценарий %q не найден", scenarioID)
	}
	m, err := gamemap.Build(sc, c)
	if err != nil {
		return nil, err
	}
	if seed == 0 {
		seed = sc.Seed
	}
	w := &World{
		C: c, Scenario: sc, Map: m, Sides: sc.Sides,
		RNG: *rng.New(seed), NextUnitID: 1, NextOrderID: 1, NextObjID: 1,
		QueueMode: c.Rules.QueueMode,
		Known:     map[int]map[int]map[string]bool{},
	}
	w.initTransient()
	for _, p := range sc.Units {
		h := hex.FromOffset(p.Col, p.Row)
		u, err := w.spawn(p.Class, p.Name, p.Side, h, hex.Dir(p.Facing).Norm())
		if err != nil {
			return nil, err
		}
		for _, e := range p.Effects {
			w.applyEffect(u, e, -1, 0, nil)
		}
	}
	w.updateVision(false)
	return w, nil
}

func (w *World) initTransient() {
	w.byID = map[int]*Unit{}
	w.occ = map[hex.Hex]int{}
	w.resv = map[hex.Hex]int{}
	w.undo = map[int][][]*Order{}
	w.planCache = map[int]*planCacheEntry{}
	w.visible = make([]map[int]bool, len(w.Sides))
	for i := range w.visible {
		w.visible[i] = map[int]bool{}
	}
}

// spawn создаёт юнит по определению класса.
func (w *World) spawn(classID, name string, side int, at hex.Hex, facing hex.Dir) (*Unit, error) {
	cls := w.C.Classes[classID]
	if cls == nil {
		return nil, fmt.Errorf("класс %q не найден", classID)
	}
	if !w.Map.InBounds(at) {
		return nil, fmt.Errorf("юнит %s вне карты %v", name, at)
	}
	if _, busy := w.occ[at]; busy {
		return nil, fmt.Errorf("клетка %v занята", at)
	}
	if name == "" {
		name = cls.Name
	}
	u := &Unit{ID: w.NextUnitID, Name: name, Class: classID, Side: side, Pos: at, Facing: facing,
		Res: map[string]int{}, Acc: map[string]int{}, Cooldowns: map[string]int{}}
	w.NextUnitID++
	w.rebuildUnitCache(u)
	u.HP = u.HPMax()
	u.AP = u.APMax()
	for _, r := range cls.Resources {
		u.Res[r] = w.ResMax(u, r)
	}
	w.Units = append(w.Units, u)
	w.byID[u.ID] = u
	w.occ[at] = u.ID
	w.OccVersion++
	w.visDirty = true
	return u, nil
}

// Unit — юнит по ID (или nil).
func (w *World) Unit(id int) *Unit { return w.byID[id] }

// UnitAt — живой юнит в клетке (по логической позиции) или nil.
func (w *World) UnitAt(h hex.Hex) *Unit {
	if id, ok := w.occ[h]; ok {
		return w.byID[id]
	}
	return nil
}

// Blocked — занята ли клетка для юнита u (другим юнитом или его резервом).
func (w *World) Blocked(u *Unit, h hex.Hex) bool {
	if id, ok := w.occ[h]; ok && (u == nil || id != u.ID) {
		return true
	}
	if id, ok := w.resv[h]; ok && (u == nil || id != u.ID) {
		return true
	}
	return false
}

func (w *World) blockerFor(u *Unit) gamemap.Blocker {
	return func(h hex.Hex) bool { return w.Blocked(u, h) }
}

// SideOf — описание стороны.
func (w *World) SideOf(side int) data.SideDef {
	for _, s := range w.Sides {
		if s.ID == side {
			return s
		}
	}
	return data.SideDef{ID: side, Name: fmt.Sprintf("Сторона %d", side)}
}

// Hostile — враждебны ли стороны.
func (w *World) Hostile(a, b int) bool { return a != b }

// Ticks — перевод секунд в тики.
func (w *World) Ticks(sec float64) int { return w.C.Ticks(sec) }

// Seconds — игровое время в секундах.
func (w *World) Seconds() float64 { return w.C.Seconds(w.Tick) }

// AliveUnits — живые юниты стороны (side < 0 — все), по возрастанию ID.
func (w *World) AliveUnits(side int) []*Unit {
	var out []*Unit
	for _, u := range w.Units {
		if !u.Dead && (side < 0 || u.Side == side) {
			out = append(out, u)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Шаг симуляции

// Step продвигает симуляцию на один фиксированный тик. Порядок обновления стабилен:
//
//  1. тик++, модель ОД «цикл» (если выбрана);
//  2. мышление ИИ (с периодом ai_think_seconds) — выдаёт команды через Apply;
//  3. регенерация ресурсов и эффекты — по возрастанию ID юнита;
//  4. действия и приказы — по возрастанию ID юнита;
//  5. объекты мира (ловушки, отложенные удары);
//  6. видимость (с периодом vision_update_ticks или при изменениях);
//  7. рассылка событий пассивным навыкам (FIFO, включая порождённые события);
//  8. проверка исхода боя.
func (w *World) Step() {
	w.Tick++
	r := &w.C.Rules
	if r.APModel == "cycle" {
		cyc := w.Ticks(r.CycleSeconds)
		if cyc > 0 && w.Tick%cyc == 0 {
			for _, u := range w.AliveUnits(-1) {
				u.AP = u.APMax()
			}
			w.emit(Event{Type: EvAPCycle, Text: "Новый тактический цикл: ОД восстановлены"})
		}
	}
	if think := w.Ticks(r.AIThinkSeconds); think > 0 && w.Tick%think == 0 {
		w.runAI()
	}
	n := len(w.Units)
	for i := 0; i < n; i++ {
		u := w.Units[i]
		if u.Dead {
			continue
		}
		w.regen(u)
		w.updateEffects(u)
	}
	for i := 0; i < n; i++ {
		u := w.Units[i]
		if u.Dead {
			continue
		}
		w.updateUnit(u)
	}
	w.updateObjects()
	if vt := r.VisionUpdateTicks; w.visDirty || vt <= 1 || w.Tick%vt == 0 {
		w.updateVision(true)
	}
	w.dispatch()
	w.checkOutcome()
}

func (w *World) regen(u *Unit) {
	tps := w.C.Rules.TicksPerSecond
	add := func(key string, perSecMilli int, cur *int, maxv int) {
		if *cur >= maxv || perSecMilli <= 0 {
			if *cur > maxv {
				*cur = maxv
			}
			u.Acc[key] = 0
			return
		}
		acc := u.Acc[key] + perSecMilli
		*cur += acc / tps
		u.Acc[key] = acc % tps
		if *cur > maxv {
			*cur = maxv
		}
	}
	if w.C.Rules.APModel == "continuous" {
		add("ap", data.AP(u.stats.Get("ap_regen")), &u.AP, u.APMax())
	} else if u.AP > u.APMax() {
		u.AP = u.APMax()
	}
	for _, rid := range w.C.ResourceOrder {
		if _, has := u.Res[rid]; !has {
			continue
		}
		rd := w.C.Resources[rid]
		v := u.Res[rid]
		add("res:"+rid, int(u.stats.Get(rd.RegenStat)*data.ResScale+0.5), &v, w.ResMax(u, rid))
		u.Res[rid] = v
	}
	if hr := u.stats.Get("hp_regen"); hr > 0 {
		milli := u.HP * 1000
		add("hp", int(hr*1000+0.5), &milli, u.HPMax()*1000)
		u.HP = milli / 1000
	}
}

func (w *World) checkOutcome() {
	if w.Outcome != "" {
		return
	}
	player := w.PlayerSide()
	alivePlayer, aliveEnemy, hostiles := 0, 0, 0
	for _, u := range w.Units {
		if u.Side == player {
			if !u.Dead {
				alivePlayer++
			}
			continue
		}
		// Неактивные цели (чучела, ai=none) не участвуют в условии победы
		if w.C.Classes[u.Class].AI == "none" {
			continue
		}
		hostiles++
		if !u.Dead {
			aliveEnemy++
		}
	}
	switch {
	case alivePlayer == 0:
		w.Outcome = "defeat"
		w.emit(Event{Type: EvBattleEnded, Kind: "defeat", Text: "Поражение: отряд погиб"})
	case hostiles > 0 && aliveEnemy == 0:
		w.Outcome = "victory"
		w.emit(Event{Type: EvBattleEnded, Kind: "victory", Text: "Победа: все противники повержены"})
	}
}

// PlayerSide — сторона, управляемая игроком.
func (w *World) PlayerSide() int {
	for _, s := range w.Sides {
		if s.Controller == "player" {
			return s.ID
		}
	}
	return 0
}

// AIControlled — управляется ли сторона ИИ.
func (w *World) AIControlled(side int) bool {
	for _, s := range w.Sides {
		if s.ID == side {
			return s.Controller == "ai"
		}
	}
	return false
}

// sortedUnits — копия списка юнитов, отсортированная по ID.
func (w *World) sortedUnits() []*Unit {
	out := append([]*Unit(nil), w.Units...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
