package sim

import (
	"fmt"

	"hextactics/engine/data"
	"hextactics/engine/hex"
)

// CmdKind — тип команды. Интерфейс и ИИ меняют мир только командами;
// клики не меняют здоровье, координаты или ОД напрямую.
type CmdKind string

const (
	CmdQueue         CmdKind = "queue"          // добавить приказ (точку маршрута) в позицию Index
	CmdEdit          CmdKind = "edit"           // изменить параметры запланированного приказа
	CmdRemove        CmdKind = "remove"         // удалить приказ (текущий — отменить)
	CmdReorder       CmdKind = "reorder"        // переставить приказ
	CmdClear         CmdKind = "clear"          // отменить всю очередь (и текущий приказ)
	CmdUndo          CmdKind = "undo"           // отменить последнее изменение очереди
	CmdCancelCurrent CmdKind = "cancel_current" // прервать текущий приказ
	CmdSetPolicy     CmdKind = "set_policy"     // политика ошибок приказа
	CmdSetQueueMode  CmdKind = "set_queue_mode" // strict | flexible
)

// Command — команда симуляции.
type Command struct {
	Kind     CmdKind    `json:"kind"`
	Side     int        `json:"side"`
	Unit     int        `json:"unit,omitempty"`
	Order    *OrderSpec `json:"order,omitempty"`
	Index    int        `json:"index"`
	OrderID  int        `json:"order_id,omitempty"`
	NewIndex int        `json:"new_index,omitempty"`
	Mode     string     `json:"mode,omitempty"`
	Policy   FailPolicy `json:"policy,omitempty"`
}

// ResultStatus — статус обработки команды.
type ResultStatus string

const (
	ResAccepted    ResultStatus = "accepted"    // принята
	ResRejected    ResultStatus = "rejected"    // отклонена
	ResWaiting     ResultStatus = "waiting"     // принята, ожидает условий (например, ОД)
	ResInterrupted ResultStatus = "interrupted" // прервана
	ResCompleted   ResultStatus = "completed"   // выполнена
	ResCancelled   ResultStatus = "cancelled"   // отменена
)

// ResultStatusName — русское название статуса.
func ResultStatusName(s ResultStatus) string {
	switch s {
	case ResAccepted:
		return "принята"
	case ResRejected:
		return "отклонена"
	case ResWaiting:
		return "ожидает условий"
	case ResInterrupted:
		return "прервана"
	case ResCompleted:
		return "выполнена"
	case ResCancelled:
		return "отменена"
	}
	return string(s)
}

// Result — результат команды.
type Result struct {
	Status   ResultStatus `json:"status"`
	Code     Code         `json:"code,omitempty"`
	Message  string       `json:"message,omitempty"`
	Warnings []Issue      `json:"warnings,omitempty"`
	OrderID  int          `json:"order_id,omitempty"`
}

// OK — команда принята.
func (r Result) OK() bool { return r.Status != ResRejected }

func reject(c Code, detail string) Result {
	is := issue(c, detail)
	return Result{Status: ResRejected, Code: c, Message: is.Text}
}

// Apply выполняет внешнюю команду (игрока) и записывает её в журнал для воспроизведения.
func (w *World) Apply(c Command) Result {
	r := w.apply(c)
	if r.OK() {
		w.CommandLog = append(w.CommandLog, LoggedCommand{Tick: w.Tick, Cmd: c})
	}
	return r
}

const maxUndo = 64

func (w *World) snapshot(u *Unit) {
	cp := make([]*Order, len(u.Queue))
	for i, o := range u.Queue {
		cp[i] = o.clone()
	}
	st := append(w.undo[u.ID], cp)
	if len(st) > maxUndo {
		st = st[len(st)-maxUndo:]
	}
	w.undo[u.ID] = st
}

// CanUndo — есть ли что отменять.
func (w *World) CanUndo(u *Unit) bool { return len(w.undo[u.ID]) > 0 }

func (w *World) touch(u *Unit) {
	u.PlanVer++
	delete(w.planCache, u.ID)
}

func (w *World) apply(c Command) Result {
	if c.Kind == CmdSetQueueMode {
		if c.Mode != "strict" && c.Mode != "flexible" {
			return reject(ErrInvalidMode, c.Mode)
		}
		w.QueueMode = c.Mode
		return Result{Status: ResAccepted}
	}
	u := w.Unit(c.Unit)
	if u == nil {
		return reject(ErrUnknownUnit, "")
	}
	if u.Side != c.Side {
		return reject(ErrNotOwner, u.Name)
	}
	if u.Dead {
		return reject(ErrUnitDead, u.Name)
	}
	switch c.Kind {
	case CmdQueue:
		return w.cmdQueue(u, c)
	case CmdEdit:
		return w.cmdEdit(u, c)
	case CmdRemove:
		if u.Current != nil && u.Current.ID == c.OrderID {
			return w.cmdCancelCurrent(u)
		}
		i := w.queueIndex(u, c.OrderID)
		if i < 0 {
			return reject(ErrOrderNotFound, "")
		}
		w.snapshot(u)
		u.Queue[i].Status = StatusCancelled
		u.Queue = append(u.Queue[:i], u.Queue[i+1:]...)
		w.touch(u)
		return Result{Status: ResCancelled}
	case CmdReorder:
		i := w.queueIndex(u, c.OrderID)
		if i < 0 {
			return reject(ErrOrderNotFound, "")
		}
		j := c.NewIndex
		if j < 0 {
			j = 0
		}
		if j >= len(u.Queue) {
			j = len(u.Queue) - 1
		}
		if i == j {
			return Result{Status: ResAccepted}
		}
		w.snapshot(u)
		o := u.Queue[i]
		u.Queue = append(u.Queue[:i], u.Queue[i+1:]...)
		u.Queue = append(u.Queue[:j], append([]*Order{o}, u.Queue[j:]...)...)
		w.touch(u)
		return w.planResult(u, o.ID, true)
	case CmdClear:
		if len(u.Queue) == 0 && u.Current == nil {
			return reject(ErrOrderNotFound, "очередь пуста")
		}
		w.snapshot(u)
		for _, o := range u.Queue {
			o.Status = StatusCancelled
		}
		u.Queue = nil
		if u.Current != nil {
			w.cmdCancelCurrent(u)
		}
		w.touch(u)
		return Result{Status: ResCancelled}
	case CmdUndo:
		st := w.undo[u.ID]
		if len(st) == 0 {
			return reject(ErrNothingToUndo, "")
		}
		prev := st[len(st)-1]
		w.undo[u.ID] = st[:len(st)-1]
		var restored []*Order
		for _, o := range prev {
			if o.Status == StatusPlanned && (u.Current == nil || u.Current.ID != o.ID) {
				restored = append(restored, o.clone())
			}
		}
		u.Queue = restored
		w.touch(u)
		return Result{Status: ResAccepted, Message: "Изменение отменено"}
	case CmdCancelCurrent:
		return w.cmdCancelCurrent(u)
	case CmdSetPolicy:
		var o *Order
		if u.Current != nil && u.Current.ID == c.OrderID {
			o = u.Current
		} else if i := w.queueIndex(u, c.OrderID); i >= 0 {
			w.snapshot(u)
			o = u.Queue[i]
		}
		if o == nil {
			return reject(ErrOrderNotFound, "")
		}
		switch c.Policy {
		case PolicyAbort, PolicySkip, PolicyRepath, PolicyWait:
		default:
			return reject(ErrInvalidOrder, string(c.Policy))
		}
		o.Policy = c.Policy
		w.touch(u)
		return Result{Status: ResAccepted}
	}
	return reject(ErrInvalidOrder, string(c.Kind))
}

func (w *World) queueIndex(u *Unit, id int) int {
	for i, o := range u.Queue {
		if o.ID == id {
			return i
		}
	}
	return -1
}

func (w *World) cmdCancelCurrent(u *Unit) Result {
	o := u.Current
	if o == nil {
		return reject(ErrOrderNotFound, "нет текущего приказа")
	}
	if a := u.Action; a != nil {
		switch a.Kind {
		case ActMove:
			// Шаг уже начат: юнит остановится, закончив его (ОД не возвращаются).
			o.Cancel = true
			w.touch(u)
			return Result{Status: ResCancelled, Message: "Остановка после текущего шага"}
		case ActRecover:
			// Умение уже применено; восстановление продолжится.
		case ActPrep, ActChannel:
			u.Action = nil
			if sk := w.C.Skills[a.Skill]; sk != nil {
				w.endChannelEffect(u, sk)
			}
		default:
			u.Action = nil
		}
	}
	w.cancelOrder(u, o)
	w.touch(u)
	return Result{Status: ResCancelled}
}

// normalizeSpec заполняет значения по умолчанию.
func (w *World) normalizeSpec(u *Unit, s OrderSpec) OrderSpec {
	if s.Policy == "" {
		if p, ok := w.C.Rules.FailPolicies[string(s.Type)]; ok {
			s.Policy = FailPolicy(p)
		} else {
			s.Policy = PolicySkip
		}
	}
	switch s.Type {
	case OrderWait:
		if s.Duration <= 0 {
			s.Duration = 1
		}
	case OrderGuard:
		if s.Duration <= 0 {
			s.Duration = w.C.Rules.GuardSeconds
		}
	}
	s.Dir = s.Dir.Norm()
	return s
}

// ValidateSpec — проверка параметров приказа, не зависящая от позиции в очереди.
func (w *World) ValidateSpec(u *Unit, s OrderSpec) *Issue {
	fail := func(c Code, d string) *Issue { is := issue(c, d); return &is }
	switch s.Type {
	case OrderMove:
		if ok, code := w.Map.CanStand(s.Dest, w.MoveProfile(u, false, 0)); !ok {
			return fail(codeFromMap(code), "")
		}
	case OrderTurn, OrderWait, OrderGuard, OrderStop:
	case OrderAttack, OrderSkill:
		var sk *data.SkillDef
		if s.Type == OrderAttack {
			if u.class.BasicAttack == "" {
				return fail(ErrNoBasicAttack, "")
			}
			sk = w.C.Skills[u.class.BasicAttack]
		} else {
			sk = w.C.Skills[s.Skill]
			if sk == nil {
				return fail(ErrUnknownSkill, s.Skill)
			}
			if !w.UnitHasSkill(u, sk.ID) {
				return fail(ErrSkillNotAvailable, sk.Name)
			}
		}
		switch sk.Targeting {
		case "unit":
			t := w.Unit(s.TargetUnit)
			if t == nil {
				return fail(ErrTargetInvalid, "")
			}
			if t.Dead {
				return fail(ErrTargetDead, t.Name)
			}
			if !w.targetFilterOK(u, t, sk) {
				what := "нужен союзник"
				if sk.TargetFilter == "enemy" {
					what = "нужен противник"
				}
				return fail(ErrTargetInvalid, what)
			}
			if w.Hostile(u.Side, t.Side) && !w.IsVisibleTo(u.Side, t) {
				return fail(ErrTargetNotVisible, t.Name)
			}
		case "hex":
			if !w.Map.InBounds(s.Dest) {
				return fail(ErrOutOfBounds, "")
			}
			if sk.TargetFilter == "empty_hex" {
				if ok, code := w.Map.CanStand(s.Dest, &footStand); !ok {
					return fail(codeFromMap(code), "")
				}
			}
		}
	case OrderInteract:
		if c := w.Map.Cell(s.Dest); c == nil || c.Feature == nil {
			return fail(ErrNoInteractable, "")
		}
	default:
		return fail(ErrInvalidOrder, string(s.Type))
	}
	return nil
}

func (w *World) cmdQueue(u *Unit, c Command) Result {
	if c.Order == nil {
		return reject(ErrInvalidOrder, "нет параметров")
	}
	if w.Outcome != "" && u.Side == w.PlayerSide() {
		return reject(ErrBattleOver, "")
	}
	spec := w.normalizeSpec(u, *c.Order)
	if is := w.ValidateSpec(u, spec); is != nil {
		return Result{Status: ResRejected, Code: is.Code, Message: is.Text}
	}
	idx := c.Index
	if idx < 0 || idx > len(u.Queue) {
		idx = len(u.Queue)
	}
	if len(u.Queue) > 0 && idx == len(u.Queue) && u.Queue[len(u.Queue)-1].Type == OrderStop {
		return reject(ErrQueueLocked, "")
	}
	if len(u.Queue) >= 40 {
		return reject(ErrNotEnoughPlanningRoom, "")
	}
	o := &Order{ID: w.NextOrderID, OrderSpec: spec, Status: StatusPlanned}
	w.snapshot(u)
	saved := append([]*Order(nil), u.Queue...)
	u.Queue = append(u.Queue[:idx], append([]*Order{o}, u.Queue[idx:]...)...)
	w.touch(u)
	r := w.planResult(u, o.ID, false)
	if !r.OK() {
		u.Queue = saved
		st := w.undo[u.ID]
		w.undo[u.ID] = st[:len(st)-1]
		w.touch(u)
		return r
	}
	w.NextOrderID++
	return r
}

func (w *World) cmdEdit(u *Unit, c Command) Result {
	if c.Order == nil {
		return reject(ErrInvalidOrder, "нет параметров")
	}
	i := w.queueIndex(u, c.OrderID)
	if i < 0 {
		if u.Current != nil && u.Current.ID == c.OrderID {
			return reject(ErrOrderActive, "")
		}
		return reject(ErrOrderNotFound, "")
	}
	old := u.Queue[i]
	spec := *c.Order
	spec.Type = old.Type
	if spec.Policy == "" {
		spec.Policy = old.Policy
	}
	spec = w.normalizeSpec(u, spec)
	if is := w.ValidateSpec(u, spec); is != nil {
		return Result{Status: ResRejected, Code: is.Code, Message: is.Text}
	}
	w.snapshot(u)
	n := old.clone()
	n.OrderSpec = spec
	n.Path, n.PathIdx = nil, 0
	u.Queue[i] = n
	w.touch(u)
	r := w.planResult(u, n.ID, false)
	if !r.OK() {
		u.Queue[i] = old
		st := w.undo[u.ID]
		w.undo[u.ID] = st[:len(st)-1]
		w.touch(u)
	}
	return r
}

// planResult проверяет очередь после изменения: ошибки нового приказа и строгий режим ОД.
func (w *World) planResult(u *Unit, orderID int, lenient bool) Result {
	plan := w.ProjectPlan(u)
	var step *PlanStep
	for i := range plan.Steps {
		if plan.Steps[i].OrderID == orderID {
			step = &plan.Steps[i]
		}
	}
	r := Result{Status: ResAccepted, OrderID: orderID}
	if step != nil {
		if step.Error != nil && !lenient {
			return Result{Status: ResRejected, Code: step.Error.Code, Message: step.Error.Text}
		}
		for _, is := range step.Issues {
			if is.Code != CodeOK {
				r.Warnings = append(r.Warnings, is)
			}
		}
	}
	if w.QueueMode == "strict" && plan.Total > u.AP {
		return reject(ErrInsufficientAP, fmt.Sprintf("очередь требует %.2f ОД, доступно %.2f (строгое резервирование)",
			data.APf(plan.Total), data.APf(u.AP)))
	}
	if plan.Shortfall > 0 {
		r.Status = ResWaiting
		r.Message = fmt.Sprintf("Не хватает %.2f ОД — очередь подождёт восстановления (~%.1f с)", data.APf(plan.Shortfall), plan.WaitSec)
	}
	return r
}

// QueueMove — упрощённый помощник для тестов и ИИ.
func (w *World) QueueMove(u *Unit, dest hex.Hex, keepFacing bool) Result {
	return w.Apply(Command{Kind: CmdQueue, Side: u.Side, Unit: u.ID, Index: -1,
		Order: &OrderSpec{Type: OrderMove, Dest: dest, KeepFacing: keepFacing}})
}
