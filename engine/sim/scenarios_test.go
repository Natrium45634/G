package sim

import (
	"strings"
	"testing"

	"hextactics/engine/combat"
	"hextactics/engine/data"
	"hextactics/engine/hex"
)

var flat = []string{
	"..............",
	"..............",
	"..............",
	"..............",
	"..............",
}

// A. Юнит проходит пять гексов; путь и стоимость рассчитаны правильно.
func TestScenarioA_FiveHexes(t *testing.T) {
	w := newTestWorld(t, flat, []data.UnitPlacement{place("warrior", 0, 1, 2, 0)})
	u := w.Units[0]
	noRegen(u)
	dest := at(6, 2)
	r := mustQueue(t, w, u, OrderSpec{Type: OrderMove, Dest: dest})
	if r.Status != ResAccepted {
		t.Fatalf("статус %s: %s", r.Status, r.Message)
	}
	plan := w.ProjectPlan(u)
	if len(plan.Steps) != 1 || len(plan.Steps[0].Path) != 5 || plan.Total != data.AP(5) {
		t.Fatalf("проекция: шагов пути %d, стоимость %d", len(plan.Steps[0].Path), plan.Total)
	}
	if plan.Free != data.AP(1) {
		t.Fatalf("остаток ОД после очереди: %d", plan.Free)
	}
	apBefore := u.AP
	if !runUntil(w, 400, idle(w, u)) {
		t.Fatal("движение не завершилось")
	}
	if u.Pos != dest {
		t.Fatalf("юнит в %v, ожидалось %v", u.Pos, dest)
	}
	if spent := apBefore - u.AP; spent != data.AP(5) {
		t.Fatalf("потрачено %d милли-ОД, ожидалось 5000", spent)
	}
	// Время: 5 шагов × 0.55 с = 2.75 с ≈ 55 тиков
	if w.Tick < 50 || w.Tick > 62 {
		t.Fatalf("длительность движения %d тиков", w.Tick)
	}
	if u.Facing != 0 {
		t.Fatalf("при обычном движении юнит должен смотреть по направлению движения: %d", u.Facing)
	}
}

// B. Юнит проходит две точки, применяет направленное умение во второй и продолжает очередь.
func TestScenarioB_SkillAtWaypointThenContinue(t *testing.T) {
	w := newTestWorld(t, flat, []data.UnitPlacement{
		place("ranger", 0, 0, 0, 0),
		place("dummy", 1, 8, 2, 3),
		place("dummy", 1, 10, 2, 3),
	})
	u := w.Units[0]
	p1, p2, p3 := at(3, 1), at(5, 2), at(4, 4)
	mustQueue(t, w, u, OrderSpec{Type: OrderMove, Dest: p1})
	mustQueue(t, w, u, OrderSpec{Type: OrderMove, Dest: p2})
	mustQueue(t, w, u, OrderSpec{Type: OrderSkill, Skill: "piercing_arrow", Dir: 0})
	mustQueue(t, w, u, OrderSpec{Type: OrderMove, Dest: p3})
	plan := w.ProjectPlan(u)
	if len(plan.Steps) != 4 {
		t.Fatalf("ожидалось 4 точки, получено %d", len(plan.Steps))
	}
	sk := plan.Steps[2]
	if sk.From != p2 || len(sk.Area) == 0 || sk.Area[0] != p2.Neighbor(0) {
		t.Fatalf("умение должно исполняться во второй точке: from=%v area=%v", sk.From, sk.Area)
	}
	hits := 0
	for _, h := range sk.Area {
		if u2 := w.UnitAt(h); u2 != nil && u2.Class == "dummy" {
			hits++
		}
	}
	if hits != 2 {
		t.Fatalf("линия должна задеть двух чучел, задевает %d", hits)
	}
	if !runUntil(w, 1200, idle(w, u)) {
		t.Fatal("очередь не завершилась")
	}
	if u.Pos != p3 {
		t.Fatalf("очередь не продолжилась после умения: позиция %v", u.Pos)
	}
	// Порядок: умение применено, когда юнит стоял во второй точке
	var resolvedFrom hex.Hex
	for _, e := range w.Events {
		if e.Type == EvSkillResolved && e.Skill == "piercing_arrow" {
			resolvedFrom = e.From
		}
	}
	if resolvedFrom != p2 {
		t.Fatalf("умение применено из %v, ожидалось %v", resolvedFrom, p2)
	}
	if len(eventsOf(w, EvAttack))+len(eventsOf(w, EvMissed))+len(eventsOf(w, EvEvaded)) < 2 {
		t.Fatal("линейная атака не разрешилась для обеих целей")
	}
}

// C. Атаки с разных направлений дают правильный сектор и модификаторы.
func TestScenarioC_DirectionalSectors(t *testing.T) {
	w := newTestWorld(t, flat, []data.UnitPlacement{
		place("dummy", 1, 6, 2, 0), // смотрит на восток
		place("warrior", 0, 1, 0, 0),
	})
	def, att := w.Units[0], w.Units[1]
	sk := w.C.Skills["strike"]
	want := map[hex.Dir]combat.Sector{0: combat.Front, 1: combat.NearFlank, 5: combat.NearFlank, 2: combat.FarFlank, 4: combat.FarFlank, 3: combat.Rear}
	for d, sec := range want {
		p := w.AttackPreview(att, def, sk, def.Pos.Neighbor(d))
		if p.Sector != sec {
			t.Fatalf("атака с направления %d: сектор %v, ожидался %v", d, p.Sector, sec)
		}
		r := w.C.Rules.Sectors
		unseen := 0
		if !w.IsVisibleTo(def.Side, att) {
			unseen = w.C.Rules.Surprise.UnseenAttackerBonus // чучело ничего не видит (обзор 0)
		}
		if p.SurpriseChance != min(100, max(0, r.Surprise[sec]+unseen+int(att.stats.Get("stealth"))-int(def.stats.Get("awareness")))) {
			t.Fatalf("сектор %v: неожиданность %d", sec, p.SurpriseChance)
		}
		if p.BlockChance != 40*r.FrontBlock[sec]/100 {
			t.Fatalf("сектор %v: блок %d", sec, p.BlockChance)
		}
		if p.EvadeChance != 20*r.Evasion[sec]/100 {
			t.Fatalf("сектор %v: уклонение %d", sec, p.EvadeChance)
		}
	}
	// Реальная атака сзади: журнал объясняет сектор
	att.Pos = def.Pos.Neighbor(3)
	w.occ = map[hex.Hex]int{def.Pos: def.ID, att.Pos: att.ID}
	att.Facing = 0
	mustQueue(t, w, att, OrderSpec{Type: OrderAttack, TargetUnit: def.ID})
	runUntil(w, 200, idle(w, att))
	found := false
	for _, e := range w.Events {
		if (e.Type == EvAttack || e.Type == EvMissed || e.Type == EvEvaded) && e.Unit == att.ID || (e.Type == EvEvaded && e.Other == att.ID) {
			if len(e.Details) > 0 && strings.Contains(e.Details[0], "тыл") {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("в журнале нет объяснения атаки с тыла")
	}
}

// D. Изменение проходимости или уход цели корректно обрабатываются политикой очереди.
func TestScenarioD_PassabilityChangeAndTargetLeaves(t *testing.T) {
	rows := []string{
		"....#....",
		"....D....",
		"....#....",
		"....#....",
	}
	door := data.FeatureDef{ID: "door", Kind: "door", Col: 4, Row: 1, Open: true}
	w := newTestWorld(t, rows, []data.UnitPlacement{place("warrior", 0, 1, 1, 0)}, door)
	u := w.Units[0]
	dest := at(7, 1)
	mustQueue(t, w, u, OrderSpec{Type: OrderMove, Dest: dest, Policy: PolicyRepath})
	mustQueue(t, w, u, OrderSpec{Type: OrderTurn, Dir: 3})
	w.Step()
	w.Map.SetDoor("door", false) // дверь закрылась во время движения
	runUntil(w, 400, idle(w, u))
	if u.Pos == dest {
		t.Fatal("юнит прошёл через закрытую дверь")
	}
	fails := eventsOf(w, EvOrderFailed)
	if len(fails) == 0 || !strings.Contains(fails[0].Text, "Переход к следующему") {
		t.Fatalf("ожидалась неудача движения с переходом к следующему приказу: %+v", fails)
	}
	if u.Facing != 3 {
		t.Fatal("следующий приказ (поворот) не выполнен после пропуска")
	}

	// Политика abort прерывает всю очередь
	w2 := newTestWorld(t, rows, []data.UnitPlacement{place("warrior", 0, 1, 1, 0)}, door)
	u2 := w2.Units[0]
	mustQueue(t, w2, u2, OrderSpec{Type: OrderMove, Dest: dest, Policy: PolicyAbort})
	mustQueue(t, w2, u2, OrderSpec{Type: OrderTurn, Dir: 3})
	w2.Step()
	w2.Map.SetDoor("door", false)
	runUntil(w2, 400, idle(w2, u2))
	if u2.Facing == 3 {
		t.Fatal("политика abort должна была отменить следующий приказ")
	}

	// Цель уходит из зоны во время замаха → атака не достигает цели, очередь продолжается
	w3 := newTestWorld(t, flat, []data.UnitPlacement{place("knight", 0, 3, 2, 0), place("goblin", 1, 4, 2, 3)})
	k, g := w3.Units[0], w3.Units[1]
	w3.AIControlled(1)
	w3.Sides[1].Controller = "player"   // гоблином управляет тест
	k.stats.SetBase("action_speed", 25) // замах длится 1 с — цель успевает уйти
	mustQueue(t, w3, k, OrderSpec{Type: OrderAttack, TargetUnit: g.ID, NoApproach: true})
	mustQueue(t, w3, k, OrderSpec{Type: OrderTurn, Dir: 2})
	w3.Step() // рыцарь начал замах
	if k.Action == nil || k.Action.Kind != ActPrep {
		t.Fatalf("ожидалась подготовка удара, действие %+v", k.Action)
	}
	queue(t, w3, g, OrderSpec{Type: OrderMove, Dest: at(7, 2)})
	runUntil(w3, 300, idle(w3, k))
	fizzled := false
	for _, e := range w3.Events {
		if e.Type == EvInfo && strings.Contains(e.Text, "не достигло цели") {
			fizzled = true
		}
	}
	if !fizzled || k.Facing != 2 {
		t.Fatalf("уход цели: промах по уходящей цели=%v, очередь продолжилась=%v", fizzled, k.Facing == 2)
	}
}

// E. Бой при 1× и 4× с одинаковыми случайными исходами даёт одинаковый результат.
func TestScenarioE_SpeedIndependence(t *testing.T) {
	run := func(speed int, frame func(i int) float64) (string, int, int) {
		c := loadContent(t)
		w, err := New(c, "river_crossing", 777)
		if err != nil {
			t.Fatal(err)
		}
		// Отряд выдвигается к мосту командами игрока, дальше обе стороны под ИИ.
		dests := []hex.Hex{at(12, 7), at(11, 8), at(11, 5), at(10, 6)}
		for i, u := range w.AliveUnits(0) {
			mustQueue(t, w, u, OrderSpec{Type: OrderMove, Dest: dests[i]})
		}
		for i := range w.Sides {
			w.Sides[i].Controller = "ai"
		}
		clock := NewClock(0)
		clock.AutoPause = AutoPause{}
		clock.SetSpeed(speed)
		target := w.Ticks(90)
		for i := 0; w.Tick < target; i++ {
			dt := frame(i)
			// не выходим за целевой тик (последний кадр укорачиваем)
			if left := float64(target-w.Tick) / float64(w.C.Rules.TicksPerSecond) / float64(speed); dt > left {
				dt = left + 1e-9
			}
			clock.Advance(w, dt)
		}
		attacks := len(eventsOf(w, EvAttack)) + len(eventsOf(w, EvMissed)) + len(eventsOf(w, EvEvaded))
		if attacks < 10 {
			t.Fatalf("бой не состоялся: атак %d", attacks)
		}
		return w.StateHash(), w.Tick, int(w.LastSeq())
	}
	h1, t1, e1 := run(1, func(int) float64 { return 1.0 / 60 })
	h4, t4, e4 := run(4, func(i int) float64 { return []float64{1.0 / 30, 1.0 / 144, 0.05, 1.0 / 60}[i%4] })
	if t1 != t4 {
		t.Fatalf("разное число тиков: %d и %d", t1, t4)
	}
	if h1 != h4 || e1 != e4 {
		t.Fatalf("результат зависит от скорости: %s/%d vs %s/%d", h1, e1, h4, e4)
	}
}

// F. Новое умение использует имеющиеся системы ОД, цели, направления, урона и эффектов без переписывания ядра.
func TestScenarioF_NewSkillFromDataOnly(t *testing.T) {
	w := newTestWorld(t, flat, []data.UnitPlacement{place("warrior", 0, 3, 2, 0), place("dummy", 1, 4, 2, 3), place("dummy", 1, 4, 3, 3)})
	// Новое умение «Ледяной вихрь»: дуга 120° длиной 2, урон магией и обездвиживание.
	w.C.Skills["frost_whirl"] = &data.SkillDef{
		ID: "frost_whirl", Name: "Ледяной вихрь", Kind: "cone", APCost: 2.5, Costs: map[string]float64{"stamina": 5},
		Prep: 0.3, Recovery: 0.2, Cooldown: 5, Targeting: "direction", Range: 2,
		Shape: data.Shape{Type: "cone", Size: 2, HalfAngle: 60}, Affects: "enemies", RequiresFacing: true,
		Attack: true, AttackKind: "spell", Unblockable: true, AccuracyMod: 100,
		Ops: []data.OpDef{{Op: "damage", DamageType: "arcane", Base: 10}, {Op: "apply_effect", Effect: "rooted", Duration: 2}},
	}
	cls := *w.C.Classes["warrior"]
	cls.Skills = append(append([]string{}, cls.Skills...), "frost_whirl")
	w.C.Classes["warrior"] = &cls
	u := w.Units[0]
	w.rebuildUnitCache(u)
	noRegen(u)
	apBefore := u.AP
	mustQueue(t, w, u, OrderSpec{Type: OrderSkill, Skill: "frost_whirl", Dir: 5})
	runUntil(w, 200, idle(w, u))
	if u.Facing != 5 {
		t.Fatal("направленное умение не развернуло юнита")
	}
	if u.AP > apBefore-data.AP(2.5) {
		t.Fatal("ОД за новое умение не списаны")
	}
	attacks := 0
	for _, e := range w.Events {
		if e.Skill == "frost_whirl" && (e.Type == EvAttack || e.Type == EvMissed || e.Type == EvEvaded) {
			attacks++
		}
	}
	if attacks != 2 {
		t.Fatalf("новое умение должно атаковать обе цели в конусе, атак %d", attacks)
	}
	if u.Cooldowns["frost_whirl"] <= w.Tick-w.Ticks(1) {
		t.Fatal("перезарядка нового умения не запущена")
	}
}
