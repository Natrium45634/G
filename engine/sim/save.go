package sim

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"hextactics/engine/data"
	"hextactics/engine/gamemap"
	hx "hextactics/engine/hex"
	"hextactics/engine/rng"
)

// SaveVersion — версия формата сохранения.
const SaveVersion = 1

// FeatureState — состояние элемента карты.
type FeatureState struct {
	ID   string `json:"id"`
	Open bool   `json:"open"`
}

// TerrainChange — изменённая поверхность клетки.
type TerrainChange struct {
	Hex     hx.Hex `json:"hex"`
	Terrain string `json:"terrain"`
}

// Snapshot — сериализуемое состояние боя. Данные симуляции отделены от интерфейса:
// сохраняются карта (изменения), юниты, часы, эффекты, ресурсы, очереди,
// перезарядки, объекты и состояние генератора случайных чисел.
type Snapshot struct {
	Version     int                             `json:"version"`
	Scenario    string                          `json:"scenario"`
	Tick        int                             `json:"tick"`
	RNG         rng.RNG                         `json:"rng"`
	Units       []*Unit                         `json:"units"`
	Objects     []*Object                       `json:"objects"`
	NextUnitID  int                             `json:"next_unit_id"`
	NextOrderID int                             `json:"next_order_id"`
	NextObjID   int                             `json:"next_obj_id"`
	QueueMode   string                          `json:"queue_mode"`
	Outcome     string                          `json:"outcome"`
	Known       map[int]map[int]map[string]bool `json:"known"`
	Features    []FeatureState                  `json:"features"`
	Terrain     []TerrainChange                 `json:"terrain,omitempty"`
	EventSeq    uint64                          `json:"event_seq"`
	Clock       *Clock                          `json:"clock,omitempty"`
}

// Snapshot формирует снимок состояния.
func (w *World) Snapshot() *Snapshot {
	s := &Snapshot{Version: SaveVersion, Scenario: w.Scenario.ID, Tick: w.Tick, RNG: w.RNG, Units: w.Units, Objects: w.Objects,
		NextUnitID: w.NextUnitID, NextOrderID: w.NextOrderID, NextObjID: w.NextObjID, QueueMode: w.QueueMode,
		Outcome: w.Outcome, Known: w.Known, EventSeq: w.eventSeq}
	if s.Objects == nil {
		s.Objects = []*Object{}
	}
	for _, c := range w.Map.FeatureCells() {
		s.Features = append(s.Features, FeatureState{ID: c.Feature.ID, Open: c.Feature.Open})
	}
	// Поверхность, отличающаяся от сценария
	if orig, err := gamemap.Build(w.Scenario, w.C); err == nil {
		for _, c := range w.Map.Cells {
			if oc := orig.Cell(c.Hex); oc != nil && oc.Terrain.ID != c.Terrain.ID {
				s.Terrain = append(s.Terrain, TerrainChange{Hex: c.Hex, Terrain: c.Terrain.ID})
			}
		}
	}
	return s
}

// Save сериализует мир (и, при наличии, часы) в JSON.
func (w *World) Save(clock *Clock) ([]byte, error) {
	s := w.Snapshot()
	s.Clock = clock
	return json.MarshalIndent(s, "", " ")
}

// Load восстанавливает мир из JSON.
func Load(c *data.Content, b []byte) (*World, *Clock, error) {
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, nil, err
	}
	if s.Version != SaveVersion {
		return nil, nil, fmt.Errorf("неподдерживаемая версия сохранения %d", s.Version)
	}
	w, err := New(c, s.Scenario, 1)
	if err != nil {
		return nil, nil, err
	}
	w.Tick, w.RNG = s.Tick, s.RNG
	w.Units, w.Objects = s.Units, s.Objects
	w.NextUnitID, w.NextOrderID, w.NextObjID = s.NextUnitID, s.NextOrderID, s.NextObjID
	w.QueueMode, w.Outcome, w.Known = s.QueueMode, s.Outcome, s.Known
	if w.Known == nil {
		w.Known = map[int]map[int]map[string]bool{}
	}
	if w.Objects == nil {
		w.Objects = []*Object{}
	}
	for _, f := range s.Features {
		if cell := w.Map.FeatureCell(f.ID); cell != nil {
			cell.Feature.Open = f.Open
		}
	}
	for _, t := range s.Terrain {
		if td := c.Terrains[t.Terrain]; td != nil {
			w.Map.SetTerrain(t.Hex, td)
		}
	}
	w.Map.Version++
	w.Events = nil
	w.eventSeq = s.EventSeq
	w.dispatched = s.EventSeq
	w.initTransient()
	for _, u := range w.Units {
		if c.Classes[u.Class] == nil {
			return nil, nil, fmt.Errorf("класс %q из сохранения не найден", u.Class)
		}
		if u.Res == nil {
			u.Res = map[string]int{}
		}
		if u.Acc == nil {
			u.Acc = map[string]int{}
		}
		if u.Cooldowns == nil {
			u.Cooldowns = map[string]int{}
		}
		w.rebuildUnitCache(u)
		w.byID[u.ID] = u
		if u.Dead {
			continue
		}
		w.occ[u.Pos] = u.ID
		if a := u.Action; a != nil && a.Kind == ActMove {
			if a.Switched {
				w.resv[a.From] = u.ID
			} else {
				w.resv[a.To] = u.ID
			}
		}
	}
	w.updateVision(false)
	clock := s.Clock
	if clock != nil {
		clock.watchSeq = w.LastSeq()
	}
	return w, clock, nil
}

// StateHash — хэш игрового состояния (для проверки детерминизма).
func (w *World) StateHash() string {
	s := w.Snapshot()
	s.EventSeq = 0
	b, _ := json.Marshal(s)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:8])
}
