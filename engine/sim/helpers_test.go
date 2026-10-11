package sim

import (
	"testing"

	"hextactics/engine/data"
	"hextactics/engine/hex"
)

var testLegend = map[string]string{".": "grass", ",": "road", "T": "forest", "#": "wall", "~": "water", "D": "floor", "%": "swamp", "*": "bush", "_": "floor", "L": "grass"}

func loadContent(t testing.TB) *data.Content {
	t.Helper()
	c, err := data.LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// newTestWorld создаёт мир по карте rows и расстановке units (колонка/строка в смещённых координатах).
func newTestWorld(t testing.TB, rows []string, units []data.UnitPlacement, feats ...data.FeatureDef) *World {
	t.Helper()
	c := loadContent(t)
	sc := &data.ScenarioDef{ID: "test", Name: "test", Seed: 12345, Legend: testLegend, Rows: rows, Features: feats,
		Sides: []data.SideDef{{ID: 0, Name: "Игрок", Controller: "player"}, {ID: 1, Name: "Враг", Controller: "ai"}},
		Units: units}
	c.Scenarios["test"] = sc
	w, err := New(c, "test", 0)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func place(class string, side, col, row, facing int) data.UnitPlacement {
	return data.UnitPlacement{Class: class, Name: class, Side: side, Col: col, Row: row, Facing: facing}
}

func at(col, row int) hex.Hex { return hex.FromOffset(col, row) }

func queue(t testing.TB, w *World, u *Unit, spec OrderSpec) Result {
	t.Helper()
	return w.Apply(Command{Kind: CmdQueue, Side: u.Side, Unit: u.ID, Index: -1, Order: &spec})
}

func mustQueue(t testing.TB, w *World, u *Unit, spec OrderSpec) Result {
	t.Helper()
	r := queue(t, w, u, spec)
	if !r.OK() {
		t.Fatalf("команда отклонена: %s (%s)", r.Message, r.Code)
	}
	return r
}

// runUntil исполняет тики, пока cond не станет истинным (или limit тиков).
func runUntil(w *World, limit int, cond func() bool) bool {
	for i := 0; i < limit; i++ {
		if cond() {
			return true
		}
		w.Step()
	}
	return cond()
}

func idle(w *World, u *Unit) func() bool {
	return func() bool { return u.Dead || (u.Current == nil && len(u.Queue) == 0 && u.Action == nil) }
}

func eventsOf(w *World, typ EventType) []Event {
	var out []Event
	for _, e := range w.Events {
		if e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}

func noRegen(u *Unit) { u.stats.SetBase("ap_regen", 0) }
