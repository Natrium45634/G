package sim

import "hextactics/engine/hex"

// EventType — тип игрового события.
type EventType string

const (
	EvOrderStarted      EventType = "order_started"
	EvOrderCompleted    EventType = "order_completed"
	EvOrderFailed       EventType = "order_failed"
	EvOrderWaiting      EventType = "order_waiting"
	EvOrderCancelled    EventType = "order_cancelled"
	EvQueueFinished     EventType = "queue_finished"
	EvUnitMoved         EventType = "unit_moved"
	EvUnitTurned        EventType = "unit_turned"
	EvUnitDashed        EventType = "unit_dashed"
	EvSkillPrepStarted  EventType = "skill_prep_started"
	EvSkillResolved     EventType = "skill_resolved"
	EvAttack            EventType = "attack"
	EvMissed            EventType = "missed"
	EvEvaded            EventType = "evaded"
	EvBlocked           EventType = "blocked"
	EvHitDealt          EventType = "hit_dealt"
	EvDamage            EventType = "damage"
	EvHeal              EventType = "heal"
	EvEffectApplied     EventType = "effect_applied"
	EvEffectResisted    EventType = "effect_resisted"
	EvEffectExpired     EventType = "effect_expired"
	EvActionInterrupted EventType = "action_interrupted"
	EvUnitDied          EventType = "unit_died"
	EvEnemySpotted      EventType = "enemy_spotted"
	EvEnemyLost         EventType = "enemy_lost"
	EvObjectPlaced      EventType = "object_placed"
	EvObjectTriggered   EventType = "object_triggered"
	EvMapChanged        EventType = "map_changed"
	EvUnitSummoned      EventType = "unit_summoned"
	EvPassiveTriggered  EventType = "passive_triggered"
	EvAPCycle           EventType = "ap_cycle"
	EvBattleEnded       EventType = "battle_ended"
	EvInfo              EventType = "info"
)

// Event — событие симуляции. События получают возрастающий номер Seq и
// обрабатываются строго в порядке появления; при ускорении ни одно не теряется,
// потому что каждый тик исполняется полностью.
type Event struct {
	Seq       uint64    `json:"seq"`
	Tick      int       `json:"tick"`
	Type      EventType `json:"type"`
	Unit      int       `json:"unit,omitempty"`  // основной участник
	Other     int       `json:"other,omitempty"` // второй участник (источник/цель)
	Hex       hex.Hex   `json:"hex"`
	From      hex.Hex   `json:"from"`
	Amount    int       `json:"amount,omitempty"`
	Skill     string    `json:"skill,omitempty"`
	Effect    string    `json:"effect,omitempty"`
	Kind      string    `json:"kind,omitempty"` // melee/ranged/spell, тип урона и т.п.
	Code      Code      `json:"code,omitempty"`
	Order     int       `json:"order,omitempty"`
	Dangerous bool      `json:"dangerous,omitempty"`
	Crit      bool      `json:"crit,omitempty"`
	Area      []hex.Hex `json:"area,omitempty"`
	Text      string    `json:"text,omitempty"`    // строка журнала (пусто — не показывать)
	Details   []string  `json:"details,omitempty"` // объяснение расчёта
	Visible   uint32    `json:"visible"`           // битовая маска сторон, которым видно событие
}

// VisibleTo — видно ли событие стороне.
func (e *Event) VisibleTo(side int) bool { return e.Visible&(1<<uint(side)) != 0 }

// emit добавляет событие. Видимость: стороны участников и стороны, видящие участников.
func (w *World) emit(e Event) *Event {
	w.eventSeq++
	e.Seq = w.eventSeq
	e.Tick = w.Tick
	for _, id := range []int{e.Unit, e.Other} {
		if id == 0 {
			continue
		}
		if u := w.Unit(id); u != nil {
			e.Visible |= 1 << uint(u.Side)
			for s := range w.Sides {
				if w.IsVisibleTo(s, u) {
					e.Visible |= 1 << uint(s)
				}
			}
		}
	}
	if e.Unit == 0 && e.Other == 0 && e.Visible == 0 {
		e.Visible = ^uint32(0)
	}
	w.Events = append(w.Events, e)
	if len(w.Events) > maxStoredEvents {
		drop := len(w.Events) - maxStoredEvents/2
		w.Events = append([]Event(nil), w.Events[drop:]...)
	}
	return &w.Events[len(w.Events)-1]
}

const maxStoredEvents = 40000

// EventsSince возвращает события с номером > seq (для интерфейса и журнала).
func (w *World) EventsSince(seq uint64) []Event {
	if len(w.Events) == 0 {
		return nil
	}
	first := w.Events[0].Seq
	if seq+1 < first {
		return w.Events
	}
	idx := int(seq + 1 - first)
	if idx >= len(w.Events) {
		return nil
	}
	return w.Events[idx:]
}

// LastSeq — номер последнего события.
func (w *World) LastSeq() uint64 { return w.eventSeq }
