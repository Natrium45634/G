package stats

import (
	"math"
	"testing"
)

func f(v float64) *float64 { return &v }

func testReg(t *testing.T) *Registry {
	r, err := NewRegistry([]Def{
		{ID: "attack", Name: "Атака", Type: "int", Base: 10},
		{ID: "evasion", Name: "Уклонение", Type: "percent", Base: 0, Min: f(0), Max: f(75)},
		{ID: "luck", Name: "Удача (новая характеристика)", Type: "float", Base: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestOrderOfModifiers(t *testing.T) {
	b := NewBlock(testReg(t), map[string]float64{"attack": 10})
	b.SetPermanent([]Modifier{
		{Stat: "attack", Op: OpPct, Value: 50, Source: "пассив"},  // постоянный %
		{Stat: "attack", Op: OpFlat, Value: 2, Source: "меч"},     // постоянный плоский
		{Stat: "attack", Op: OpMult, Value: 2, Source: "класс"},   // постоянный множитель
		{Stat: "attack", Op: OpPct, Value: 50, Source: "пассив2"}, // проценты слоя суммируются
	})
	b.SetTemporary([]Modifier{
		{Stat: "attack", Op: OpFlat, Value: 3, Source: "ярость"},
		{Stat: "attack", Op: OpPct, Value: -50, Source: "слабость"},
	})
	// (10 + 2) × (1+100%) × 2 = 48; + 3 = 51; × 0.5 = 25.5
	if got := b.Get("attack"); !near(got, 25.5) {
		t.Fatalf("ожидалось 25.5, получено %v", got)
	}
	bd := b.Explain("attack", nil)
	if bd.Final != 25.5 || len(bd.Steps) < 6 || bd.Steps[0].Stage != "base" || bd.Steps[1].Stage != "perm_flat" {
		t.Fatalf("объяснение неверно: %+v", bd)
	}
	if len(bd.Lines()) == 0 {
		t.Fatal("нет текстового объяснения")
	}
}

func TestClampAndCache(t *testing.T) {
	b := NewBlock(testReg(t), map[string]float64{"evasion": 60})
	b.SetTemporary([]Modifier{{Stat: "evasion", Op: OpFlat, Value: 40, Source: "туман"}})
	if b.Get("evasion") != 75 {
		t.Fatalf("ограничение максимума не сработало: %v", b.Get("evasion"))
	}
	b.SetTemporary(nil)
	if b.Get("evasion") != 60 {
		t.Fatal("кэш не сброшен после смены модификаторов")
	}
	b.SetTemporary([]Modifier{{Stat: "evasion", Op: OpFlat, Value: -100, Source: "оглушение"}})
	if b.Get("evasion") != 0 {
		t.Fatal("ограничение минимума не сработало")
	}
}

func TestSituationalModifiers(t *testing.T) {
	b := NewBlock(testReg(t), map[string]float64{"attack": 10})
	b.SetPermanent([]Modifier{{Stat: "attack", Op: OpFlat, Value: 5, Source: "удар в спину",
		When: &Condition{Role: "attacker", Sectors: []string{"rear"}}}})
	if b.Get("attack") != 10 {
		t.Fatal("ситуативный модификатор применился без контекста")
	}
	if b.GetCtx("attack", &Context{Role: "attacker", Sector: "front"}) != 10 {
		t.Fatal("ситуативный модификатор применился в неверном секторе")
	}
	if b.GetCtx("attack", &Context{Role: "attacker", Sector: "rear"}) != 15 {
		t.Fatal("ситуативный модификатор не применился")
	}
}

func TestNewStatWithoutCodeChanges(t *testing.T) {
	b := NewBlock(testReg(t), nil)
	if b.Get("luck") != 1 {
		t.Fatal("значение по умолчанию из определения не применилось")
	}
	b.SetPermanent([]Modifier{{Stat: "luck", Op: OpMult, Value: 3, Source: "амулет"}})
	if b.Get("luck") != 3 {
		t.Fatal("модификатор новой характеристики не работает")
	}
}

func TestDuplicateDefRejected(t *testing.T) {
	if _, err := NewRegistry([]Def{{ID: "a"}, {ID: "a"}}); err == nil {
		t.Fatal("дубликат не отклонён")
	}
}
