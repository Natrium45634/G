// Package gamemap — карта: клетки, поверхность, высоты, препятствия, двери,
// стоимость перемещения, поиск пути (Dijkstra/A*) и линия видимости.
//
// Карта не знает о юнитах: динамические препятствия (занятые клетки) передаются
// в поиск пути функцией Blocker. Изменения, влияющие на проходимость или обзор
// (например, открытие двери), увеличивают Version — по нему интерфейс сбрасывает кэш.
package gamemap

import (
	"fmt"

	"hextactics/engine/data"
	"hextactics/engine/hex"
)

// Коды причин недоступности.
const (
	CodeOutOfBounds = "OUT_OF_BOUNDS"
	CodeImpassable  = "CELL_IMPASSABLE"
	CodeDoorClosed  = "DOOR_CLOSED"
	CodeCliff       = "CLIFF"
	CodeOccupied    = "CELL_OCCUPIED"
	CodeNoPath      = "NO_PATH"
)

// Feature — интерактивный элемент клетки.
type Feature struct {
	ID    string   `json:"id"`
	Kind  string   `json:"kind"` // door | lever
	Open  bool     `json:"open"`
	Links []string `json:"links,omitempty"`
}

// Cell — клетка карты.
type Cell struct {
	ID      int
	Hex     hex.Hex
	Col     int
	Row     int
	Terrain *data.TerrainDef
	Height  int
	Feature *Feature
}

// IsClosedDoor — закрытая дверь.
func (c *Cell) IsClosedDoor() bool {
	return c.Feature != nil && c.Feature.Kind == "door" && !c.Feature.Open
}

// BlocksLOS — полностью перекрывает обзор.
func (c *Cell) BlocksLOS() bool { return c.Terrain.BlocksLOS || c.IsClosedDoor() }

// Cover — укрытие для стоящего в клетке (от дальних атак), %.
func (c *Cell) Cover() int { return c.Terrain.Cover }

// LOSCover — частичное препятствие на линии выстрела, %.
func (c *Cell) LOSCover() int { return c.Terrain.LOSCover }

// Map — игровая карта.
type Map struct {
	Width   int
	Height  int
	Cells   []*Cell
	index   map[hex.Hex]int
	feats   map[string]*Cell
	Version uint64
	minPct  map[string]int // минимальный % стоимости по классу передвижения (для эвристики A*)
}

// Build строит карту по описанию сценария.
func Build(sc *data.ScenarioDef, c *data.Content) (*Map, error) {
	m := &Map{Height: len(sc.Rows), index: map[hex.Hex]int{}, feats: map[string]*Cell{}, minPct: map[string]int{}}
	for row, line := range sc.Rows {
		runes := []rune(line)
		if len(runes) > m.Width {
			m.Width = len(runes)
		}
		for col, ch := range runes {
			tid := sc.Legend[string(ch)]
			t := c.Terrains[tid]
			if t == nil {
				return nil, fmt.Errorf("сценарий %s: неизвестный символ %q", sc.ID, string(ch))
			}
			h := hex.FromOffset(col, row)
			cell := &Cell{ID: len(m.Cells), Hex: h, Col: col, Row: row, Terrain: t}
			if row < len(sc.Heights) && col < len(sc.Heights[row]) {
				d := sc.Heights[row][col]
				if d >= '0' && d <= '9' {
					cell.Height = int(d - '0')
				}
			}
			m.index[h] = cell.ID
			m.Cells = append(m.Cells, cell)
		}
	}
	for _, f := range sc.Features {
		cell := m.Cell(hex.FromOffset(f.Col, f.Row))
		if cell == nil {
			return nil, fmt.Errorf("сценарий %s: элемент %s вне карты", sc.ID, f.ID)
		}
		cell.Feature = &Feature{ID: f.ID, Kind: f.Kind, Open: f.Open, Links: append([]string(nil), f.Links...)}
		m.feats[f.ID] = cell
	}
	for _, t := range c.Terrains {
		for cls, pct := range t.MoveCost {
			if cur, ok := m.minPct[cls]; !ok || pct < cur {
				m.minPct[cls] = pct
			}
		}
	}
	return m, nil
}

// Cell возвращает клетку или nil вне карты.
func (m *Map) Cell(h hex.Hex) *Cell {
	if i, ok := m.index[h]; ok {
		return m.Cells[i]
	}
	return nil
}

// InBounds — клетка существует.
func (m *Map) InBounds(h hex.Hex) bool { _, ok := m.index[h]; return ok }

// FeatureCell — клетка с элементом по ID.
func (m *Map) FeatureCell(id string) *Cell { return m.feats[id] }

// FeatureCells — все клетки с элементами (стабильный порядок по ID клетки).
func (m *Map) FeatureCells() []*Cell {
	var out []*Cell
	for _, c := range m.Cells {
		if c.Feature != nil {
			out = append(out, c)
		}
	}
	return out
}

// SetDoor открывает или закрывает дверь. Возвращает true, если состояние изменилось.
func (m *Map) SetDoor(id string, open bool) bool {
	c := m.feats[id]
	if c == nil || c.Feature.Kind != "door" || c.Feature.Open == open {
		return false
	}
	c.Feature.Open = open
	m.Version++
	return true
}

// SetTerrain меняет поверхность клетки (для сценариев, тестов и будущих умений).
func (m *Map) SetTerrain(h hex.Hex, t *data.TerrainDef) {
	if c := m.Cell(h); c != nil && c.Terrain != t {
		c.Terrain = t
		m.Version++
	}
}

// ---------------------------------------------------------------------------
// Стоимость перемещения

// MoveProfile — параметры перемещения юнита (рассчитываются из характеристик и пассивных навыков).
type MoveProfile struct {
	Class           string
	StepCost        int            // милли-ОД за клетку со стоимостью 100%
	TerrainOverride map[string]int // поверхность → % (пассивные навыки)
	ClimbCost       int            // милли-ОД за уровень подъёма
	MaxClimb        int
	KeepFacing      bool    // движение без поворота (пятиться/боком)
	Facing          hex.Dir // ориентация при KeepFacing
	StrafePct       int     // множитель стоимости шага не вперёд при KeepFacing
}

// TerrainPct — % стоимости поверхности для профиля; ok=false — непроходимо.
func (p *MoveProfile) TerrainPct(t *data.TerrainDef) (int, bool) {
	if v, ok := p.TerrainOverride[t.ID]; ok {
		return v, true
	}
	v, ok := t.MoveCost[p.Class]
	return v, ok
}

// CanStand — может ли юнит находиться в клетке (без учёта занятости).
func (m *Map) CanStand(h hex.Hex, p *MoveProfile) (bool, string) {
	c := m.Cell(h)
	if c == nil {
		return false, CodeOutOfBounds
	}
	if c.IsClosedDoor() {
		return false, CodeDoorClosed
	}
	if _, ok := p.TerrainPct(c.Terrain); !ok {
		return false, CodeImpassable
	}
	return true, ""
}

// StepCost — стоимость перехода между соседними клетками в милли-ОД.
func (m *Map) StepCost(from, to hex.Hex, p *MoveProfile) (int, string) {
	cf, ct := m.Cell(from), m.Cell(to)
	if ct == nil || cf == nil {
		return 0, CodeOutOfBounds
	}
	if ct.IsClosedDoor() {
		return 0, CodeDoorClosed
	}
	pct, ok := p.TerrainPct(ct.Terrain)
	if !ok {
		return 0, CodeImpassable
	}
	dh := ct.Height - cf.Height
	if dh > p.MaxClimb || -dh > p.MaxClimb {
		return 0, CodeCliff
	}
	cost := p.StepCost * pct / 100
	if p.KeepFacing && p.StrafePct > 0 {
		if d, ok := hex.DirectionTo(from, to); ok && d != p.Facing {
			cost = cost * p.StrafePct / 100
		}
	}
	if dh > 0 {
		cost += dh * p.ClimbCost
	}
	return cost, ""
}

func (m *Map) minStep(p *MoveProfile) int {
	pct, ok := m.minPct[p.Class]
	if !ok {
		pct = 100
	}
	for _, v := range p.TerrainOverride {
		if v < pct {
			pct = v
		}
	}
	return p.StepCost * pct / 100
}

// Describe — текст причины недоступности.
func Describe(code string) string {
	switch code {
	case CodeOutOfBounds:
		return "Вне карты"
	case CodeImpassable:
		return "Непроходимая местность для этого юнита"
	case CodeDoorClosed:
		return "Дверь закрыта"
	case CodeCliff:
		return "Слишком крутой перепад высот"
	case CodeOccupied:
		return "Клетка занята"
	case CodeNoPath:
		return "Нет пути"
	}
	return code
}
