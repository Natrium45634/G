package gamemap

import (
	"testing"

	"hextactics/engine/data"
	"hextactics/engine/hex"
)

func content(t *testing.T) *data.Content {
	c, err := data.LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

var legend = map[string]string{".": "grass", ",": "road", "T": "forest", "#": "wall", "~": "water", "D": "floor", "%": "swamp"}

func build(t *testing.T, rows []string, heights []string, feats []data.FeatureDef) *Map {
	sc := &data.ScenarioDef{ID: "t", Legend: legend, Rows: rows, Heights: heights, Features: feats}
	m, err := Build(sc, content(t))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func foot() *MoveProfile {
	return &MoveProfile{Class: "foot", StepCost: 1000, ClimbCost: 500, MaxClimb: 1}
}

func at(col, row int) hex.Hex { return hex.FromOffset(col, row) }

// Сценарий A: юнит проходит пять гексов; путь и стоимость рассчитаны правильно.
func TestScenarioA_FiveHexPathCost(t *testing.T) {
	m := build(t, []string{"........"}, nil, nil)
	res := m.FindPath(at(0, 0), at(5, 0), foot(), nil)
	if !res.OK || len(res.Path) != 5 || res.Cost != 5000 {
		t.Fatalf("ожидался путь из 5 шагов стоимостью 5 ОД: %+v", res)
	}
	heavy := &MoveProfile{Class: "heavy", StepCost: 2000, MaxClimb: 1}
	if r := m.FindPath(at(0, 0), at(5, 0), heavy, nil); r.Cost != 10000 {
		t.Fatalf("тяжёлый воин: ожидалось 10 ОД, получено %d", r.Cost)
	}
	scout := &MoveProfile{Class: "foot", StepCost: 500, MaxClimb: 1}
	if r := m.FindPath(at(0, 0), at(5, 0), scout, nil); r.Cost != 2500 {
		t.Fatalf("разведчик: ожидалось 2.5 ОД, получено %d", r.Cost)
	}
}

func TestPathAvoidsExpensiveTerrainAndWalls(t *testing.T) {
	m := build(t, []string{
		"......",
		".TTTT.",
		"......",
	}, nil, nil)
	// По прямой через лес — 2 ОД за клетку; обход по траве дешевле.
	res := m.FindPath(at(0, 1), at(5, 1), foot(), nil)
	if !res.OK {
		t.Fatal("путь не найден")
	}
	for _, h := range res.Path[:len(res.Path)-1] {
		if m.Cell(h).Terrain.ID == "forest" {
			t.Fatalf("путь проходит через лес, хотя обход дешевле: %v", res.Path)
		}
	}
	if res.Cost >= 5*2000 {
		t.Fatalf("стоимость не оптимальна: %d", res.Cost)
	}
	// Геометрическое расстояние ≠ стоимость пути
	if hex.Distance(at(0, 1), at(5, 1)) != 5 || res.Cost == 5000 {
		t.Fatalf("ожидалась стоимость больше геометрического расстояния: %d", res.Cost)
	}

	w := build(t, []string{
		"..#...",
		"..#...",
		"..#...",
	}, nil, nil)
	if r := w.FindPath(at(0, 1), at(4, 1), foot(), nil); r.OK {
		t.Fatalf("путь сквозь сплошную стену: %v", r.Path)
	} else if r.Code != CodeNoPath {
		t.Fatalf("код ошибки %s", r.Code)
	}
	if r := w.FindPath(at(0, 1), at(2, 1), foot(), nil); r.Code != CodeImpassable {
		t.Fatalf("ожидался CELL_IMPASSABLE, получено %s", r.Code)
	}
}

func TestOccupiedCellsAndMoveClasses(t *testing.T) {
	m := build(t, []string{".%%%."}, nil, nil)
	heavy := &MoveProfile{Class: "heavy", StepCost: 2000, MaxClimb: 1}
	if r := m.FindPath(at(0, 0), at(4, 0), heavy, nil); r.OK {
		t.Fatal("тяжёлый юнит прошёл через болото")
	}
	if r := m.FindPath(at(0, 0), at(4, 0), foot(), nil); !r.OK || r.Cost != 1000+3*3000 {
		t.Fatalf("пеший: %+v", r)
	}
	// Пассивный навык переопределяет стоимость поверхности
	p := foot()
	p.TerrainOverride = map[string]int{"swamp": 150}
	if r := m.FindPath(at(0, 0), at(4, 0), p, nil); r.Cost != 1000+3*1500 {
		t.Fatalf("переопределение стоимости не применилось: %d", r.Cost)
	}
	occ := func(h hex.Hex) bool { return h == at(2, 0) }
	if r := m.FindPath(at(0, 0), at(4, 0), foot(), occ); r.OK {
		t.Fatal("путь через занятую клетку в коридоре")
	}
	if r := m.FindPath(at(0, 0), at(2, 0), foot(), occ); r.Code != CodeOccupied {
		t.Fatalf("ожидался CELL_OCCUPIED, получено %s", r.Code)
	}
}

func TestReachableRespectsBudget(t *testing.T) {
	m := build(t, []string{"........", "........", "........"}, nil, nil)
	field := m.Reachable(at(3, 1), foot(), nil, 2000)
	for h, r := range field {
		if hex.Distance(h, at(3, 1)) > 2 || r.Cost > 2000 {
			t.Fatalf("клетка %v за пределами бюджета (%d)", h, r.Cost)
		}
	}
	if len(field) < 12 {
		t.Fatalf("слишком мало доступных клеток: %d", len(field))
	}
	path := ReconstructPath(field, at(3, 1), at(5, 1))
	if len(path) != 2 {
		t.Fatalf("восстановленный путь: %v", path)
	}
}

func TestHeightsClimbCostAndCliffs(t *testing.T) {
	m := build(t, []string{"...."}, []string{"0120"}, nil)
	r := m.FindPath(at(0, 0), at(2, 0), foot(), nil)
	if !r.OK || r.Cost != 2*1000+2*500 {
		t.Fatalf("подъём: %+v", r)
	}
	if _, code := m.StepCost(at(2, 0), at(3, 0), foot()); code != CodeCliff {
		t.Fatalf("ожидался обрыв, получено %q", code)
	}
}

// Сценарий D (часть карты): изменение проходимости — закрытая дверь.
func TestDoorChangesPassabilityAndLOS(t *testing.T) {
	m := build(t, []string{
		"..#..",
		"..D..",
		"..#..",
	}, nil, []data.FeatureDef{{ID: "d", Kind: "door", Col: 2, Row: 1}})
	if r := m.FindPath(at(0, 1), at(4, 1), foot(), nil); r.OK {
		t.Fatal("путь через закрытую дверь")
	}
	if m.LineOfSight(at(1, 1), at(3, 1)).Clear {
		t.Fatal("закрытая дверь не перекрывает обзор")
	}
	v := m.Version
	if !m.SetDoor("d", true) || m.Version == v {
		t.Fatal("версия карты не изменилась")
	}
	if r := m.FindPath(at(0, 1), at(4, 1), foot(), nil); !r.OK || len(r.Path) != 4 {
		t.Fatalf("путь через открытую дверь: %+v", r)
	}
	if !m.LineOfSight(at(1, 1), at(3, 1)).Clear {
		t.Fatal("открытая дверь перекрывает обзор")
	}
}

func TestLineOfSightWallsSoftCoverHeightAndSymmetry(t *testing.T) {
	m := build(t, []string{
		"........",
		"...#....",
		"..TT....",
	}, []string{"00000000", "00000000", "00000000"}, nil)
	if m.LineOfSight(at(1, 1), at(6, 1)).Clear {
		t.Fatal("стена не перекрыла обзор")
	}
	los := m.LineOfSight(at(0, 2), at(6, 2))
	if !los.Clear || los.SoftCover != 30 {
		t.Fatalf("лес должен давать частичное укрытие 30%%: %+v", los)
	}
	for _, a := range m.Cells {
		for _, b := range m.Cells {
			if m.LineOfSight(a.Hex, b.Hex).Clear != m.LineOfSight(b.Hex, a.Hex).Clear {
				t.Fatalf("видимость несимметрична между %v и %v", a.Hex, b.Hex)
			}
		}
	}
	h := build(t, []string{"....."}, []string{"00200"}, nil)
	if h.LineOfSight(at(0, 0), at(4, 0)).Clear {
		t.Fatal("холм не перекрыл обзор между низинами")
	}
	h2 := build(t, []string{"....."}, []string{"20202"}, nil)
	if !h2.LineOfSight(at(0, 0), at(4, 0)).Clear {
		t.Fatal("с высоты на высоту обзор должен быть открыт")
	}
}

func TestStrafeCost(t *testing.T) {
	m := build(t, []string{"....."}, nil, nil)
	p := foot()
	p.KeepFacing, p.Facing, p.StrafePct = true, 3, 150 // смотрит на запад, идёт на восток (пятится)
	if c, _ := m.StepCost(at(0, 0), at(1, 0), p); c != 1500 {
		t.Fatalf("стоимость шага назад: %d", c)
	}
	p.Facing = 0
	if c, _ := m.StepCost(at(0, 0), at(1, 0), p); c != 1000 {
		t.Fatalf("стоимость шага вперёд: %d", c)
	}
}
