package sim

import (
	"strings"
	"testing"

	"hextactics/engine/data"
	"hextactics/engine/hex"
)

// ---------------------------------------------------------------------------
// Время

func TestPauseStopsSimulation(t *testing.T) {
	w := newTestWorld(t, flat, []data.UnitPlacement{place("warrior", 0, 1, 2, 0)})
	c := NewClock(0)
	u := w.Units[0]
	mustQueue(t, w, u, OrderSpec{Type: OrderMove, Dest: at(8, 2)})
	if n := c.Advance(w, 5); n != 0 || w.Tick != 0 {
		t.Fatalf("на паузе прошло %d тиков", n)
	}
	// на паузе можно редактировать очередь
	if r := queue(t, w, u, OrderSpec{Type: OrderTurn, Dir: 3}); !r.OK() {
		t.Fatal("редактирование на паузе отклонено")
	}
	c.SetSpeed(1)
	c.Advance(w, 0.5)
	if w.Tick != 10 {
		t.Fatalf("за 0.5 с при 1× ожидалось 10 тиков, прошло %d", w.Tick)
	}
	c.SetSpeed(0)
	pos := u.Pos
	c.Advance(w, 10)
	if w.Tick != 10 || u.Pos != pos {
		t.Fatal("пауза не остановила симуляцию")
	}
}

func TestFrameRateIndependence(t *testing.T) {
	mk := func() (*World, *Unit) {
		w := newTestWorld(t, flat, []data.UnitPlacement{place("ranger", 0, 0, 2, 0)})
		u := w.Units[0]
		mustQueue(t, w, u, OrderSpec{Type: OrderMove, Dest: at(12, 2)})
		return w, u
	}
	w1, u1 := mk()
	w2, u2 := mk()
	c1, c2 := NewClock(0), NewClock(0)
	c1.SetSpeed(1)
	c2.SetSpeed(1)
	for i := 0; i < 30; i++ { // 30 FPS, 1 секунда
		c1.Advance(w1, 1.0/30)
	}
	for i := 0; i < 240; i++ { // 240 FPS, 1 секунда
		c2.Advance(w2, 1.0/240)
	}
	if w1.Tick != w2.Tick || u1.Pos != u2.Pos || u1.AP != u2.AP {
		t.Fatalf("результат зависит от FPS: тики %d/%d, позиции %v/%v", w1.Tick, w2.Tick, u1.Pos, u2.Pos)
	}
}

func TestAccelerationKeepsAllEvents(t *testing.T) {
	count := func(speed int) int {
		w := newTestWorld(t, flat, []data.UnitPlacement{place("ranger", 0, 0, 2, 0)})
		u := w.Units[0]
		mustQueue(t, w, u, OrderSpec{Type: OrderMove, Dest: at(12, 2)})
		c := NewClock(0)
		c.SetSpeed(speed)
		for w.Tick < 200 {
			c.Advance(w, 0.1)
		}
		return len(eventsOf(w, EvUnitMoved))
	}
	if a, b := count(1), count(4); a != b || a != 12 {
		t.Fatalf("события потеряны при ускорении: 1×=%d, 4×=%d", a, b)
	}
}

func TestAutoPauseStopsExactlyAtEventTick(t *testing.T) {
	rows := []string{"....................", "....................", "...................."}
	run := func(speed int) int {
		w := newTestWorld(t, rows, []data.UnitPlacement{place("warrior", 0, 0, 1, 0), place("goblin", 1, 17, 1, 3)})
		mustQueue(t, w, w.Units[0], OrderSpec{Type: OrderMove, Dest: at(12, 1)})
		w.Sides[1].Controller = "player"
		c := NewClock(0)
		c.SetSpeed(speed)
		for i := 0; i < 1000 && !c.Paused(); i++ {
			c.Advance(w, 1.0/60)
		}
		if !strings.Contains(c.Reason, "Замечен противник") {
			t.Fatalf("причина автопаузы: %q", c.Reason)
		}
		return w.Tick
	}
	if a, b := run(1), run(4); a != b {
		t.Fatalf("автопауза сработала на разных тиках: 1×=%d, 4×=%d", a, b)
	}
}

// ---------------------------------------------------------------------------
// Очки действия

func TestAPRegenContinuousAndCycle(t *testing.T) {
	w := newTestWorld(t, flat, []data.UnitPlacement{place("warrior", 0, 1, 2, 0)})
	u := w.Units[0]
	u.AP = 0
	for i := 0; i < 20; i++ {
		w.Step()
	}
	if u.AP != data.AP(1) {
		t.Fatalf("за 1 с восстановлено %d милли-ОД, ожидалось 1000", u.AP)
	}
	// Регенерация 0.9 ОД/с без накопления ошибки округления
	k := newTestWorld(t, flat, []data.UnitPlacement{place("knight", 0, 1, 2, 0)}).Units[0]
	_ = k
	wk := newTestWorld(t, flat, []data.UnitPlacement{place("knight", 0, 1, 2, 0)})
	kn := wk.Units[0]
	kn.AP = 0
	for i := 0; i < 100; i++ {
		wk.Step()
	}
	if kn.AP != data.AP(4.5) {
		t.Fatalf("рыцарь за 5 с: %d, ожидалось 4500", kn.AP)
	}

	wc := newTestWorld(t, flat, []data.UnitPlacement{place("warrior", 0, 1, 2, 0)})
	wc.C.Rules.APModel = "cycle"
	wc.C.Rules.CycleSeconds = 2
	uc := wc.Units[0]
	uc.AP = 0
	for i := 0; i < 39; i++ {
		wc.Step()
	}
	if uc.AP != 0 {
		t.Fatal("в модели «цикл» ОД не должны расти между циклами")
	}
	wc.Step()
	if uc.AP != uc.APMax() {
		t.Fatal("в начале цикла ОД должны восстановиться полностью")
	}
}

func TestQueueCostExampleAndReservation(t *testing.T) {
	// Пример из ТЗ: 6 ОД; движение 2, поворот 1 (2×0.5), выстрел 2 → итого 5, остаток 1.
	w := newTestWorld(t, flat, []data.UnitPlacement{place("warrior", 0, 1, 2, 0), place("dummy", 1, 6, 2, 3)})
	u := w.Units[0]
	noRegen(u)
	mustQueue(t, w, u, OrderSpec{Type: OrderMove, Dest: at(3, 2)})
	mustQueue(t, w, u, OrderSpec{Type: OrderTurn, Dir: 2})
	r := mustQueue(t, w, u, OrderSpec{Type: OrderTurn, Dir: 0})
	if r.Status != ResAccepted {
		t.Fatal(r.Message)
	}
	plan := w.ProjectPlan(u)
	if plan.Total != data.AP(4) || plan.Free != data.AP(2) {
		t.Fatalf("итого %d, остаток %d", plan.Total, plan.Free)
	}
	// Атака сверх бюджета в гибком режиме — принята с ожиданием
	r = mustQueue(t, w, u, OrderSpec{Type: OrderAttack, TargetUnit: w.Units[1].ID})
	if r.Status != ResWaiting {
		t.Fatalf("ожидался статус «ожидает условий», получено %s", r.Status)
	}
	// В строгом режиме — отклонена
	w.Apply(Command{Kind: CmdUndo, Side: 0, Unit: u.ID})
	w.Apply(Command{Kind: CmdSetQueueMode, Mode: "strict"})
	r = queue(t, w, u, OrderSpec{Type: OrderAttack, TargetUnit: w.Units[1].ID})
	if r.OK() || r.Code != ErrInsufficientAP {
		t.Fatalf("строгий режим: %s %s", r.Status, r.Code)
	}
	if !strings.Contains(r.Message, "Недостаточно очков действия") {
		t.Fatalf("нет понятного текста ошибки: %q", r.Message)
	}
}

func TestTurnCostsAPAndNoSplitExploit(t *testing.T) {
	w := newTestWorld(t, flat, []data.UnitPlacement{place("warrior", 0, 1, 2, 0)})
	u := w.Units[0]
	noRegen(u)
	before := u.AP
	mustQueue(t, w, u, OrderSpec{Type: OrderTurn, Dir: 3}) // 180°
	runUntil(w, 100, idle(w, u))
	big := before - u.AP
	if big != data.AP(1.5) || u.Facing != 3 {
		t.Fatalf("разворот на 180° стоит %d (ожидалось 1500)", big)
	}
	before = u.AP
	mustQueue(t, w, u, OrderSpec{Type: OrderTurn, Dir: 4})
	mustQueue(t, w, u, OrderSpec{Type: OrderTurn, Dir: 5})
	mustQueue(t, w, u, OrderSpec{Type: OrderTurn, Dir: 0})
	runUntil(w, 100, idle(w, u))
	if split := before - u.AP; split != big {
		t.Fatalf("поворот мелкими приказами стоит %d вместо %d", split, big)
	}
	// Оглушённый не поворачивается
	w.applyEffect(u, "stunned", 1, 0, nil)
	f := u.Facing
	mustQueue(t, w, u, OrderSpec{Type: OrderTurn, Dir: 2})
	for i := 0; i < 10; i++ {
		w.Step()
	}
	if u.Facing != f || u.Current == nil || u.Current.Status != StatusWaiting {
		t.Fatal("оглушённый юнит повернулся или приказ не ждёт")
	}
	runUntil(w, 100, idle(w, u))
	if u.Facing != 2 {
		t.Fatal("после оглушения поворот не выполнен")
	}
}

func TestInsufficientAPWaitsInFlexibleMode(t *testing.T) {
	w := newTestWorld(t, flat, []data.UnitPlacement{place("warrior", 0, 1, 2, 0)})
	u := w.Units[0]
	u.AP = 500
	mustQueue(t, w, u, OrderSpec{Type: OrderMove, Dest: at(2, 2)})
	w.Step()
	if u.Current == nil || u.Current.Status != StatusWaiting || u.Current.Issue.Code != ErrInsufficientAP {
		t.Fatal("приказ должен ожидать восстановления ОД")
	}
	runUntil(w, 100, idle(w, u))
	if u.Pos != at(2, 2) {
		t.Fatal("после восстановления ОД приказ не выполнен")
	}
}

func TestCancelDoesNotRefund(t *testing.T) {
	w := newTestWorld(t, flat, []data.UnitPlacement{place("knight", 0, 1, 2, 0), place("dummy", 1, 2, 2, 3)})
	u := w.Units[0]
	noRegen(u)
	before := u.AP
	mustQueue(t, w, u, OrderSpec{Type: OrderAttack, TargetUnit: w.Units[1].ID})
	w.Step()
	if u.Action == nil || u.Action.Kind != ActPrep {
		t.Fatal("удар не начат")
	}
	r := w.Apply(Command{Kind: CmdCancelCurrent, Side: 0, Unit: u.ID})
	if r.Status != ResCancelled {
		t.Fatal(r.Message)
	}
	if u.AP != before-data.AP(2) {
		t.Fatal("ОД возвращены после отмены — возможен обход стоимости")
	}
}

// ---------------------------------------------------------------------------
// Очередь и команды

func TestQueueEditingUndoReorderStop(t *testing.T) {
	w := newTestWorld(t, flat, []data.UnitPlacement{place("warrior", 0, 1, 2, 0)})
	u := w.Units[0]
	a := mustQueue(t, w, u, OrderSpec{Type: OrderMove, Dest: at(3, 2)}).OrderID
	b := mustQueue(t, w, u, OrderSpec{Type: OrderMove, Dest: at(5, 2)}).OrderID
	c := mustQueue(t, w, u, OrderSpec{Type: OrderTurn, Dir: 3}).OrderID
	// Изменить промежуточную точку
	r := w.Apply(Command{Kind: CmdEdit, Side: 0, Unit: u.ID, OrderID: a, Order: &OrderSpec{Dest: at(3, 1)}})
	if !r.OK() || u.Queue[0].Dest != at(3, 1) {
		t.Fatal("изменение точки маршрута не применено")
	}
	// Переставить
	w.Apply(Command{Kind: CmdReorder, Side: 0, Unit: u.ID, OrderID: c, NewIndex: 0})
	if u.Queue[0].ID != c {
		t.Fatal("перестановка не выполнена")
	}
	// Удалить
	w.Apply(Command{Kind: CmdRemove, Side: 0, Unit: u.ID, OrderID: b})
	if len(u.Queue) != 2 {
		t.Fatal("удаление не выполнено")
	}
	// Отменить последние три изменения
	for i := 0; i < 3; i++ {
		if r := w.Apply(Command{Kind: CmdUndo, Side: 0, Unit: u.ID}); !r.OK() {
			t.Fatal(r.Message)
		}
	}
	if len(u.Queue) != 3 || u.Queue[0].ID != a || u.Queue[0].Dest != at(3, 2) {
		t.Fatalf("отмена изменений восстановила неверную очередь: %+v", u.Queue[0])
	}
	// Отмена всей очереди и её отмена
	w.Apply(Command{Kind: CmdClear, Side: 0, Unit: u.ID})
	if len(u.Queue) != 0 {
		t.Fatal("очередь не очищена")
	}
	w.Apply(Command{Kind: CmdUndo, Side: 0, Unit: u.ID})
	if len(u.Queue) != 3 {
		t.Fatal("очистку нельзя отменить")
	}
	// Стоп завершает очередь
	mustQueue(t, w, u, OrderSpec{Type: OrderStop})
	if r := queue(t, w, u, OrderSpec{Type: OrderTurn, Dir: 1}); r.OK() || r.Code != ErrQueueLocked {
		t.Fatal("после «Стоп» нельзя добавлять приказы в конец")
	}
	// Вставка перед «Стоп» разрешена
	if r := w.Apply(Command{Kind: CmdQueue, Side: 0, Unit: u.ID, Index: 1, Order: &OrderSpec{Type: OrderWait, Duration: 0.5}}); !r.OK() {
		t.Fatal("вставка в середину очереди отклонена")
	}
	// Чужим юнитом управлять нельзя
	if r := w.Apply(Command{Kind: CmdClear, Side: 1, Unit: u.ID}); r.OK() || r.Code != ErrNotOwner {
		t.Fatal("команда чужой стороны принята")
	}
	runUntil(w, 500, idle(w, u))
	if u.Facing != 3 {
		t.Fatal("очередь не исполнена до конца")
	}
}

func TestRejectionReasons(t *testing.T) {
	w := newTestWorld(t, []string{".....#.....", "....~~~....", "..........."}, []data.UnitPlacement{place("warrior", 0, 1, 1, 0), place("goblin", 1, 9, 2, 3)})
	u := w.Units[0]
	w.Sides[1].Controller = "player"
	cases := []struct {
		spec OrderSpec
		code Code
	}{
		{OrderSpec{Type: OrderMove, Dest: at(5, 0)}, ErrImpassable},
		{OrderSpec{Type: OrderMove, Dest: at(40, 0)}, ErrOutOfBounds},
		{OrderSpec{Type: OrderSkill, Skill: "fireball", Dest: at(3, 1)}, ErrSkillNotAvailable},
		{OrderSpec{Type: OrderSkill, Skill: "nope"}, ErrUnknownSkill},
		{OrderSpec{Type: OrderInteract, Dest: at(2, 2)}, ErrNoInteractable},
	}
	for _, c := range cases {
		r := queue(t, w, u, c.spec)
		if r.OK() || r.Code != c.code || r.Message == "" {
			t.Errorf("%+v: ожидался код %s, получено %s %q", c.spec, c.code, r.Code, r.Message)
		}
	}
	// Невидимая цель не может быть выбрана
	w.visible[0] = map[int]bool{}
	if r := queue(t, w, u, OrderSpec{Type: OrderAttack, TargetUnit: w.Units[1].ID}); r.Code != ErrTargetNotVisible {
		t.Fatalf("невидимая цель: %s", r.Code)
	}
}

func TestAttackApproachTurnAndHit(t *testing.T) {
	w := newTestWorld(t, flat, []data.UnitPlacement{place("warrior", 0, 1, 2, 3), place("dummy", 1, 6, 2, 3)})
	u, d := w.Units[0], w.Units[1]
	r := mustQueue(t, w, u, OrderSpec{Type: OrderAttack, TargetUnit: d.ID})
	_ = r
	plan := w.ProjectPlan(u)
	if len(plan.Steps[0].Path) != 4 {
		t.Fatalf("проекция подхода: %v", plan.Steps[0].Path)
	}
	runUntil(w, 400, idle(w, u))
	if hex.Distance(u.Pos, d.Pos) != 1 {
		t.Fatal("юнит не подошёл к цели")
	}
	if len(eventsOf(w, EvAttack))+len(eventsOf(w, EvMissed))+len(eventsOf(w, EvEvaded)) != 1 {
		t.Fatal("атака не состоялась")
	}
}

func TestMoveKeepFacingCostsMore(t *testing.T) {
	w := newTestWorld(t, flat, []data.UnitPlacement{place("warrior", 0, 5, 2, 0)})
	u := w.Units[0]
	noRegen(u)
	before := u.AP
	mustQueue(t, w, u, OrderSpec{Type: OrderMove, Dest: at(3, 2), KeepFacing: true}) // пятится на запад
	runUntil(w, 300, idle(w, u))
	if u.Facing != 0 {
		t.Fatal("при движении без поворота ориентация изменилась")
	}
	if spent := before - u.AP; spent != data.AP(3) {
		t.Fatalf("отступление: потрачено %d, ожидалось 3000 (2 шага × 150%%)", spent)
	}
}

// ---------------------------------------------------------------------------
// Эффекты, пассивные навыки, объекты

func TestStunInterruptsPreparationAndBlocksActions(t *testing.T) {
	w := newTestWorld(t, flat, []data.UnitPlacement{place("mage", 0, 1, 2, 0), place("dummy", 1, 6, 2, 3)})
	m := w.Units[0]
	mustQueue(t, w, m, OrderSpec{Type: OrderSkill, Skill: "fireball", Dest: at(6, 2)})
	mustQueue(t, w, m, OrderSpec{Type: OrderTurn, Dir: 3})
	w.Step()
	if m.Action == nil || m.Action.Kind != ActPrep {
		t.Fatal("подготовка не началась")
	}
	w.applyEffect(m, "stunned", 1, 0, nil)
	if m.Action != nil {
		t.Fatal("оглушение не прервало подготовку")
	}
	if len(eventsOf(w, EvActionInterrupted)) != 1 {
		t.Fatal("нет события прерывания")
	}
	// Политика skip: следующий приказ выполняется после окончания оглушения
	runUntil(w, 200, idle(w, m))
	if m.Facing != 3 {
		t.Fatal("очередь не продолжилась после прерывания")
	}
}

func TestDamageInterruptsInterruptibleSkill(t *testing.T) {
	w := newTestWorld(t, flat, []data.UnitPlacement{place("mage", 0, 1, 2, 0), place("dummy", 1, 6, 2, 3)})
	m := w.Units[0]
	manaBefore, apBefore := m.Res["mana"], m.AP
	mustQueue(t, w, m, OrderSpec{Type: OrderSkill, Skill: "fireball", Dest: at(6, 2)})
	w.Step()
	w.dealDamage(m, 5, "physical", nil, DamageOpts{})
	if m.Action != nil || m.Current != nil {
		t.Fatal("урон не прервал подготовку огненного шара")
	}
	if m.Res["mana"] > manaBefore-24*data.ResScale || m.AP > apBefore-data.AP(3) {
		t.Fatal("ресурсы и ОД за прерванное умение должны быть потрачены")
	}
}

func TestEffectsDotAuraAbsorbRedirectResist(t *testing.T) {
	w := newTestWorld(t, flat, []data.UnitPlacement{
		place("knight", 0, 2, 2, 0), place("warrior", 0, 3, 2, 0), place("mage", 0, 6, 0, 0),
	})
	k, wr, m := w.Units[0], w.Units[1], w.Units[2]
	// Горение: 3 урона в секунду 3 секунды
	hp := wr.HP
	w.applyEffect(wr, "burning", 3, 0, nil)
	for i := 0; i < 60; i++ {
		w.Step()
	}
	if hp-wr.HP != 9 {
		t.Fatalf("горение нанесло %d вместо 9", hp-wr.HP)
	}
	// Аура: союзник в радиусе получает эффект
	w.applyEffect(wr, "rally_aura", 5, wr.ID, nil)
	w.Step()
	if w.findEffect(k, "rallied") == nil || w.findEffect(m, "rallied") != nil {
		t.Fatal("аура должна действовать в радиусе 2 и только там")
	}
	if k.stats.Get("accuracy") != 90 {
		t.Fatalf("модификатор ауры не применён: точность %v", k.stats.Get("accuracy"))
	}
	// Поглощение
	w.applyEffect(m, "barrier", 10, m.ID, nil)
	hp = m.HP
	w.dealDamage(m, 30, "fire", nil, DamageOpts{})
	if hp-m.HP != 5 || w.findEffect(m, "barrier") != nil {
		t.Fatalf("барьер: потеряно %d HP", hp-m.HP)
	}
	// Перенаправление: половину урона по наёмнику принимает рыцарь
	w.applyEffect(wr, "protected", 8, k.ID, nil)
	hk, hw := k.HP, wr.HP
	w.dealDamage(wr, 20, "physical", nil, DamageOpts{})
	if hk-k.HP != 10 || hw-wr.HP != 10 {
		t.Fatalf("перенаправление: рыцарь −%d, наёмник −%d", hk-k.HP, hw-wr.HP)
	}
	// Сопротивление эффекту и упорство
	k.stats.SetBase("eres_control", 100)
	if w.applyEffect(k, "stunned", 2, 0, nil) {
		t.Fatal("стойкость 100% не защитила от оглушения")
	}
	k.stats.SetBase("eres_control", 0)
	k.stats.SetBase("tenacity", 50)
	w.applyEffect(k, "rooted", 2, 0, nil)
	if e := w.findEffect(k, "rooted"); e == nil || e.Remaining != w.Ticks(1) {
		t.Fatal("упорство должно сократить длительность вдвое")
	}
	// Иммунитет к эффекту через тег
	wr.tags["immune_effect:control"] = true
	if w.applyEffect(wr, "stunned", 2, 0, nil) {
		t.Fatal("иммунитет к контролю не сработал")
	}
	// Иммунитет к урону
	wr.tags["immune_damage:fire"] = true
	hp = wr.HP
	w.applyOps([]data.OpDef{{Op: "damage", DamageType: "fire", Base: 50}}, &OpContext{Target: wr})
	if wr.HP != hp {
		t.Fatal("иммунитет к огню не сработал")
	}
}

func TestPassivesReactThroughEvents(t *testing.T) {
	w := newTestWorld(t, flat, []data.UnitPlacement{place("knight", 0, 2, 2, 0), place("goblin", 1, 3, 2, 3), place("warrior", 0, 1, 1, 0)})
	k, g, wr := w.Units[0], w.Units[1], w.Units[2]
	// Ответный удар после блока
	hp := g.HP
	w.emit(Event{Type: EvBlocked, Unit: k.ID, Other: g.ID, Kind: "melee"})
	w.dispatch()
	if g.HP >= hp {
		t.Fatal("«Ответный удар» не сработал на блок")
	}
	hp2 := g.HP
	w.emit(Event{Type: EvBlocked, Unit: k.ID, Other: g.ID, Kind: "melee"})
	w.dispatch()
	if g.HP != hp2 {
		t.Fatal("перезарядка реакции не соблюдается")
	}
	// Второе дыхание — один раз
	wr.HP = 40
	w.dealDamage(wr, 15, "physical", nil, DamageOpts{})
	w.dispatch()
	if wr.HP != 45 {
		t.Fatalf("«Второе дыхание»: HP %d", wr.HP)
	}
	w.dealDamage(wr, 30, "physical", nil, DamageOpts{})
	w.dispatch()
	if wr.HP != 15 {
		t.Fatal("одноразовая реакция сработала повторно")
	}
	// Месть при гибели союзника рядом
	w.kill(k, g)
	w.dispatch()
	if w.findEffect(wr, "enraged") == nil {
		t.Fatal("«Месть» не сработала")
	}
}

func TestSleepingUnitWakesAndIsSurprised(t *testing.T) {
	w := newTestWorld(t, flat, []data.UnitPlacement{place("warrior", 0, 2, 2, 0), place("goblin", 1, 3, 2, 3)})
	wr, g := w.Units[0], w.Units[1]
	w.applyEffect(g, "asleep", -1, 0, nil)
	p := w.AttackPreview(wr, g, w.C.Skills["strike"], wr.Pos)
	if p.SurpriseChance != 100 {
		t.Fatal("атака по спящему должна быть неожиданной")
	}
	w.dealDamage(g, 1, "physical", wr, DamageOpts{})
	if w.HasFlag(g, "asleep") {
		t.Fatal("урон не разбудил")
	}
}

func TestTrapsDelayedAndSummon(t *testing.T) {
	w := newTestWorld(t, flat, []data.UnitPlacement{place("ranger", 0, 1, 2, 0), place("goblin", 1, 8, 2, 3), place("mage", 0, 1, 4, 0)})
	r, g, m := w.Units[0], w.Units[1], w.Units[2]
	w.Sides[1].Controller = "player"
	mustQueue(t, w, r, OrderSpec{Type: OrderSkill, Skill: "snare_trap", Dest: at(3, 2)})
	runUntil(w, 100, idle(w, r))
	if len(w.Objects) != 1 {
		t.Fatal("ловушка не установлена")
	}
	if w.ObjectVisibleTo(w.Objects[0], 1) {
		t.Fatal("ловушка должна быть невидима противнику")
	}
	queue(t, w, g, OrderSpec{Type: OrderMove, Dest: at(2, 2)})
	runUntil(w, 300, func() bool { return len(eventsOf(w, EvObjectTriggered)) == 1 })
	if g.Pos != at(3, 2) || !w.HasFlag(g, "rooted") || g.HP >= g.HPMax() {
		t.Fatalf("ловушка не сработала: позиция %v", g.Pos)
	}
	for i := 0; i < 20; i++ {
		w.Step()
	}
	if g.Pos != at(3, 2) || g.Current == nil || g.Current.Issue == nil || g.Current.Issue.Code != ErrRooted {
		t.Fatal("обездвиженный юнит должен стоять и ждать")
	}
	runUntil(w, 300, idle(w, g))
	if g.Pos != at(2, 2) {
		t.Fatal("после окончания обездвиживания приказ не продолжен")
	}
	// Отложенный удар: метеор взрывается через 3 с
	mustQueue(t, w, m, OrderSpec{Type: OrderSkill, Skill: "meteor", Dest: g.Pos})
	runUntil(w, 40, func() bool { return len(w.Objects) == 1 })
	placedAt := w.Tick
	hp := g.HP
	runUntil(w, 100, func() bool { return len(eventsOf(w, EvObjectTriggered)) == 2 })
	if g.HP >= hp && !g.Dead {
		t.Fatal("метеор не нанёс урон")
	}
	if dt := w.Tick - placedAt; dt < w.Ticks(2.9) || dt > w.Ticks(3.2) {
		t.Fatalf("метеор сработал через %d тиков", dt)
	}
	// Призыв
	ctx := &OpContext{Source: g, Hex: at(10, 2)}
	n := len(w.Units)
	w.summon("wolf", 1, ctx)
	if len(w.Units) != n+1 || !w.Units[n].Summoned || w.Units[n].Side != 1 {
		t.Fatal("призыв не создал юнита нужной стороны")
	}
}

func TestChannelRestoresAndCanBeInterrupted(t *testing.T) {
	w := newTestWorld(t, flat, []data.UnitPlacement{place("mage", 0, 1, 2, 0)})
	m := w.Units[0]
	m.Res["mana"] = 0
	mustQueue(t, w, m, OrderSpec{Type: OrderSkill, Skill: "meditate"})
	for i := 0; i < 50; i++ {
		w.Step()
	}
	if m.Action == nil || m.Action.Kind != ActChannel {
		t.Fatal("каналирование не идёт")
	}
	if m.Res["mana"] < 16*data.ResScale {
		t.Fatalf("мана не восстанавливается: %d", m.Res["mana"])
	}
	if w.findEffect(m, "meditating") == nil {
		t.Fatal("эффект каналирования не наложен")
	}
	w.dealDamage(m, 3, "physical", nil, DamageOpts{})
	if m.Action != nil || w.findEffect(m, "meditating") != nil {
		t.Fatal("урон не прервал каналирование")
	}
}

func TestGuardReactsToEnemyEnteringZone(t *testing.T) {
	w := newTestWorld(t, flat, []data.UnitPlacement{place("warrior", 0, 3, 2, 0), place("goblin", 1, 8, 2, 3)})
	wr, g := w.Units[0], w.Units[1]
	w.Sides[1].Controller = "player"
	mustQueue(t, w, wr, OrderSpec{Type: OrderGuard, Duration: 10, React: true})
	mustQueue(t, w, wr, OrderSpec{Type: OrderTurn, Dir: 3})
	w.Step()
	if !w.HasFlag(wr, "guarding") {
		t.Fatal("защитная стойка не включилась")
	}
	queue(t, w, g, OrderSpec{Type: OrderMove, Dest: at(4, 2)})
	runUntil(w, 300, idle(w, wr))
	attacked := false
	for _, e := range w.Events {
		if (e.Type == EvAttack || e.Type == EvMissed || e.Type == EvEvaded) && (e.Unit == wr.ID || e.Other == wr.ID) && e.Skill == "strike" {
			attacked = true
		}
	}
	if !attacked || wr.Facing != 3 || w.HasFlag(wr, "guarding") {
		t.Fatalf("реакция стойки: атака=%v, очередь продолжена=%v", attacked, wr.Facing == 3)
	}
}

func TestInteractLeverOpensDoor(t *testing.T) {
	rows := []string{"....#....", "....D....", "....#...."}
	w := newTestWorld(t, rows, []data.UnitPlacement{place("warrior", 0, 1, 1, 0)},
		data.FeatureDef{ID: "d", Kind: "door", Col: 4, Row: 1},
		data.FeatureDef{ID: "l", Kind: "lever", Col: 1, Row: 2, Links: []string{"d"}})
	u := w.Units[0]
	if r := queue(t, w, u, OrderSpec{Type: OrderMove, Dest: at(7, 1)}); r.OK() {
		t.Fatal("путь через закрытую дверь принят")
	}
	mustQueue(t, w, u, OrderSpec{Type: OrderInteract, Dest: at(1, 2)})
	runUntil(w, 100, idle(w, u))
	if !w.Map.FeatureCell("d").Feature.Open {
		t.Fatal("рычаг не открыл дверь")
	}
	mustQueue(t, w, u, OrderSpec{Type: OrderMove, Dest: at(7, 1)})
	runUntil(w, 300, idle(w, u))
	if u.Pos != at(7, 1) {
		t.Fatal("не прошёл через открытую дверь")
	}
}

func TestChargeDashesAlongRoute(t *testing.T) {
	w := newTestWorld(t, flat, []data.UnitPlacement{place("warrior", 0, 1, 2, 0), place("dummy", 1, 6, 2, 3)})
	u, d := w.Units[0], w.Units[1]
	mustQueue(t, w, u, OrderSpec{Type: OrderSkill, Skill: "charge", Dir: 0})
	plan := w.ProjectPlan(u)
	if len(plan.Steps[0].Area) != 1 || plan.Steps[0].Area[0] != at(5, 2) {
		t.Fatalf("область натиска после рывка: %v", plan.Steps[0].Area)
	}
	runUntil(w, 100, idle(w, u))
	if u.Pos != at(4, 2) {
		t.Fatalf("рывок закончился в %v", u.Pos)
	}
	_ = d
	if len(eventsOf(w, EvUnitDashed)) != 1 {
		t.Fatal("нет события рывка")
	}
}

// ---------------------------------------------------------------------------
// Сохранение и воспроизведение

func TestSaveLoadContinuesIdentically(t *testing.T) {
	c := loadContent(t)
	w, _ := New(c, "river_crossing", 99)
	dests := []hex.Hex{at(12, 7), at(11, 8), at(11, 5), at(10, 6)}
	for i, u := range w.AliveUnits(0) {
		mustQueue(t, w, u, OrderSpec{Type: OrderMove, Dest: dests[i]})
	}
	for i := range w.Sides {
		w.Sides[i].Controller = "ai"
	}
	for i := 0; i < w.Ticks(15); i++ {
		w.Step()
	}
	b, err := w.Save(NewClock(0))
	if err != nil {
		t.Fatal(err)
	}
	w2, clk, err := Load(c, b)
	if err != nil {
		t.Fatal(err)
	}
	if clk == nil {
		t.Fatal("часы не сохранены")
	}
	for i := range w2.Sides {
		w2.Sides[i].Controller = "ai"
	}
	if w.StateHash() != w2.StateHash() {
		t.Fatal("состояние после загрузки отличается")
	}
	for i := 0; i < w.Ticks(30); i++ {
		w.Step()
		w2.Step()
	}
	if w.StateHash() != w2.StateHash() {
		t.Fatal("после загрузки симуляция разошлась")
	}
}

func TestReplayFromCommandLog(t *testing.T) {
	c := loadContent(t)
	w, _ := New(c, "training", 5)
	u := w.AliveUnits(0)[0]
	for i := 0; i < 10; i++ {
		w.Step()
	}
	mustQueue(t, w, u, OrderSpec{Type: OrderMove, Dest: at(6, 4)})
	for i := 0; i < 30; i++ {
		w.Step()
	}
	mustQueue(t, w, u, OrderSpec{Type: OrderTurn, Dir: 2})
	for i := 0; i < 60; i++ {
		w.Step()
	}
	r, _ := New(c, "training", 5)
	log := w.CommandLog
	li := 0
	for r.Tick < w.Tick {
		for li < len(log) && log[li].Tick == r.Tick {
			r.Apply(log[li].Cmd)
			li++
		}
		r.Step()
	}
	if r.StateHash() != w.StateHash() {
		t.Fatal("воспроизведение журнала команд дало другой результат")
	}
}
