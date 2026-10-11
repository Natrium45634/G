package sim

import (
	"fmt"

	"hextactics/engine/hex"
)

// OrderType — тип точки маршрута / приказа.
type OrderType string

const (
	OrderMove     OrderType = "move"
	OrderTurn     OrderType = "turn"
	OrderAttack   OrderType = "attack"
	OrderSkill    OrderType = "skill"
	OrderInteract OrderType = "interact"
	OrderWait     OrderType = "wait"
	OrderGuard    OrderType = "guard"
	OrderStop     OrderType = "stop"
)

// OrderTypeName — русское название типа приказа.
func OrderTypeName(t OrderType) string {
	switch t {
	case OrderMove:
		return "Движение"
	case OrderTurn:
		return "Поворот"
	case OrderAttack:
		return "Атака"
	case OrderSkill:
		return "Умение"
	case OrderInteract:
		return "Взаимодействие"
	case OrderWait:
		return "Ожидание"
	case OrderGuard:
		return "Защита"
	case OrderStop:
		return "Стоп"
	}
	return string(t)
}

// OrderStatus — статус приказа.
type OrderStatus string

const (
	StatusPlanned     OrderStatus = "planned"
	StatusActive      OrderStatus = "active"
	StatusWaiting     OrderStatus = "waiting"
	StatusDone        OrderStatus = "done"
	StatusFailed      OrderStatus = "failed"
	StatusInterrupted OrderStatus = "interrupted"
	StatusCancelled   OrderStatus = "cancelled"
	StatusSkipped     OrderStatus = "skipped"
)

// StatusName — русское название статуса.
func StatusName(s OrderStatus) string {
	switch s {
	case StatusPlanned:
		return "запланирован"
	case StatusActive:
		return "исполняется"
	case StatusWaiting:
		return "ожидает условий"
	case StatusDone:
		return "выполнен"
	case StatusFailed:
		return "не выполнен"
	case StatusInterrupted:
		return "прерван"
	case StatusCancelled:
		return "отменён"
	case StatusSkipped:
		return "пропущен"
	}
	return string(s)
}

// FailPolicy — политика обработки ошибки при исполнении.
type FailPolicy string

const (
	PolicyAbort  FailPolicy = "abort"  // прервать всю очередь
	PolicySkip   FailPolicy = "skip"   // пропустить приказ, продолжить очередь
	PolicyRepath FailPolicy = "repath" // перестроить путь; если невозможно — ждать, затем пропустить
	PolicyWait   FailPolicy = "wait"   // ждать выполнения условий (с тайм-аутом), затем пропустить
)

// PolicyName — русское название политики.
func PolicyName(p FailPolicy) string {
	switch p {
	case PolicyAbort:
		return "прервать очередь"
	case PolicySkip:
		return "пропустить"
	case PolicyRepath:
		return "перестроить путь"
	case PolicyWait:
		return "ждать"
	}
	return string(p)
}

// Policies — порядок переключения политик в интерфейсе.
var Policies = []FailPolicy{PolicySkip, PolicyAbort, PolicyRepath, PolicyWait}

// OrderSpec — параметры нового приказа (входные данные команды).
type OrderSpec struct {
	Type       OrderType  `json:"type"`
	Dest       hex.Hex    `json:"dest"`
	Dir        hex.Dir    `json:"dir"`
	TargetUnit int        `json:"target_unit,omitempty"`
	Skill      string     `json:"skill,omitempty"`
	KeepFacing bool       `json:"keep_facing,omitempty"`
	Duration   float64    `json:"duration,omitempty"`
	React      bool       `json:"react,omitempty"`
	NoApproach bool       `json:"no_approach,omitempty"`
	Policy     FailPolicy `json:"policy,omitempty"`
}

// Order — приказ в очереди юнита (точка маршрута или действие в точке).
type Order struct {
	ID int `json:"id"`
	OrderSpec
	Status OrderStatus `json:"status"`
	Issue  *Issue      `json:"issue,omitempty"`

	// Состояние исполнения
	Path      []hex.Hex `json:"path,omitempty"`
	PathIdx   int       `json:"path_idx,omitempty"`
	WaitSince int       `json:"wait_since,omitempty"`
	StartedAt int       `json:"started_at,omitempty"`
	Phase     int       `json:"phase,omitempty"`
	Cancel    bool      `json:"cancel,omitempty"`   // остановиться после текущего шага
	Repathed  bool      `json:"repathed,omitempty"` // маршрут уже перестраивался (для журнала)
}

func (o *Order) clone() *Order {
	c := *o
	c.Path = append([]hex.Hex(nil), o.Path...)
	if o.Issue != nil {
		is := *o.Issue
		c.Issue = &is
	}
	return &c
}

// Describe — краткое описание приказа.
func (w *World) Describe(o *Order) string {
	switch o.Type {
	case OrderMove:
		s := fmt.Sprintf("Движение → %d,%d", o.Dest.Q, o.Dest.R)
		if c := w.Map.Cell(o.Dest); c != nil {
			s = fmt.Sprintf("Движение → [%d:%d]", c.Col, c.Row)
		}
		if o.KeepFacing {
			s += " (без поворота)"
		}
		return s
	case OrderTurn:
		return fmt.Sprintf("Поворот → %s %s", o.Dir.Arrow(), o.Dir.Name())
	case OrderAttack:
		if t := w.Unit(o.TargetUnit); t != nil {
			return "Атака: " + t.Name
		}
		return "Атака"
	case OrderSkill:
		name := o.Skill
		sk := w.C.Skills[o.Skill]
		if sk != nil {
			name = sk.Name
			switch sk.Targeting {
			case "unit":
				if t := w.Unit(o.TargetUnit); t != nil {
					return name + ": " + t.Name
				}
			case "direction":
				return fmt.Sprintf("%s %s %s", name, o.Dir.Arrow(), o.Dir.Name())
			case "hex":
				if c := w.Map.Cell(o.Dest); c != nil {
					return fmt.Sprintf("%s → [%d:%d]", name, c.Col, c.Row)
				}
			case "auto":
				return name + " (авто-цель)"
			}
		}
		return name
	case OrderInteract:
		if c := w.Map.Cell(o.Dest); c != nil && c.Feature != nil {
			if c.Feature.Kind == "lever" {
				return "Дёрнуть рычаг"
			}
			if c.Feature.Open {
				return "Закрыть дверь"
			}
			return "Открыть дверь"
		}
		return "Взаимодействие"
	case OrderWait:
		return fmt.Sprintf("Ожидание %.1f с", o.Duration)
	case OrderGuard:
		s := fmt.Sprintf("Защитная стойка %.0f с", o.Duration)
		if o.React {
			s += " + атака по входящим"
		}
		return s
	case OrderStop:
		return "Стоп (конец очереди)"
	}
	return string(o.Type)
}
