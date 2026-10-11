package sim

import (
	"fmt"

	"hextactics/engine/hex"
)

// AutoPause — настройки автоматической паузы (какие события останавливают время).
type AutoPause struct {
	EnemySpotted    bool `json:"enemy_spotted"`
	UnitDied        bool `json:"unit_died"`        // гибель своего юнита
	ImportantDied   bool `json:"important_died"`   // гибель важного юнита (любой стороны)
	DangerousAttack bool `json:"dangerous_attack"` // опасная атака/подготовка опасного умения по своим
	Interrupted     bool `json:"interrupted"`      // прерывание действия своего юнита
	OrderFailed     bool `json:"order_failed"`     // приказ не выполнен
	QueueFinished   bool `json:"queue_finished"`   // юнит выполнил очередь
}

// DefaultAutoPause — настройки по умолчанию.
func DefaultAutoPause() AutoPause {
	return AutoPause{EnemySpotted: true, UnitDied: true, ImportantDied: true, DangerousAttack: true, Interrupted: true, OrderFailed: false, QueueFinished: false}
}

// Clock — игровые часы. Различаются:
//   - реальное время (секунды кадров, RealElapsed);
//   - игровое время (World.Seconds = тики / ticks_per_second; на паузе не растёт);
//   - время симуляции (номер тика World.Tick);
//   - длительность действия (в тиках, Action.End-Action.Start).
//
// Advance переводит реальное время в игровое (× скорость) и исполняет целые тики
// фиксированной длительности. Поэтому результат не зависит от FPS и скорости:
// при 1× и 4× выполняются те же тики в том же порядке, меняется только их число за кадр.
type Clock struct {
	Speed       int       `json:"speed"` // 0 — пауза, 1, 2, 4
	LastSpeed   int       `json:"last_speed"`
	AutoPause   AutoPause `json:"auto_pause"`
	PlayerSide  int       `json:"player_side"`
	Reason      string    `json:"reason"` // причина последней автопаузы
	ReasonHex   hex.Hex   `json:"reason_hex"`
	ReasonAt    bool      `json:"reason_at"` // есть ли место события
	RealElapsed float64   `json:"real_elapsed"`
	MaxTicks    int       `json:"max_ticks"` // предел тиков за один кадр (защита от «спирали»)
	acc         float64
	watchSeq    uint64
}

// Speeds — доступные скорости.
var Speeds = []int{1, 2, 4}

// NewClock создаёт часы на паузе.
func NewClock(playerSide int) *Clock {
	return &Clock{Speed: 0, LastSpeed: 1, AutoPause: DefaultAutoPause(), PlayerSide: playerSide, MaxTicks: 200}
}

// SetSpeed устанавливает скорость (0 — пауза).
func (c *Clock) SetSpeed(s int) {
	if s > 0 {
		c.LastSpeed = s
		c.Reason = ""
	}
	if s == 0 {
		c.acc = 0
	}
	c.Speed = s
}

// TogglePause — пауза/продолжение с последней скоростью.
func (c *Clock) TogglePause() {
	if c.Speed == 0 {
		c.SetSpeed(c.LastSpeed)
	} else {
		c.SetSpeed(0)
	}
}

// Alpha — доля следующего тика, накопленная в аккумуляторе (0..1) — для плавной анимации.
func (c *Clock) Alpha(w *World) float64 {
	if c.Speed == 0 {
		return 0
	}
	a := c.acc * float64(w.C.Rules.TicksPerSecond)
	if a > 1 {
		a = 1
	}
	return a
}

// Paused — время остановлено.
func (c *Clock) Paused() bool { return c.Speed == 0 }

// Advance продвигает мир на realDt секунд реального времени. Возвращает число тиков.
// Если событие требует автопаузы, симуляция останавливается сразу после тика,
// в котором оно произошло, — независимо от скорости.
func (c *Clock) Advance(w *World, realDt float64) int {
	c.RealElapsed += realDt
	if c.watchSeq == 0 {
		c.watchSeq = w.LastSeq()
	}
	if c.Speed == 0 {
		c.watchSeq = w.LastSeq()
		return 0
	}
	tickDur := 1.0 / float64(w.C.Rules.TicksPerSecond)
	c.acc += realDt * float64(c.Speed)
	n := 0
	for c.acc >= tickDur && n < c.MaxTicks {
		w.Step()
		c.acc -= tickDur
		n++
		if reason := c.checkAutoPause(w); reason != "" {
			c.SetSpeed(0)
			c.Reason = reason
			return n
		}
	}
	if c.acc > tickDur*float64(c.MaxTicks) {
		// Компьютер не успевает: игровое время замедляется, но тики не пропускаются.
		c.acc = tickDur * float64(c.MaxTicks)
	}
	return n
}

// RunTicks исполняет ровно n тиков (для тестов и пошагового режима), с автопаузой.
func (c *Clock) RunTicks(w *World, n int) int {
	done := 0
	for i := 0; i < n; i++ {
		w.Step()
		done++
		if reason := c.checkAutoPause(w); reason != "" {
			c.SetSpeed(0)
			c.Reason = reason
			break
		}
	}
	return done
}

func (c *Clock) checkAutoPause(w *World) string {
	evs := w.EventsSince(c.watchSeq)
	c.watchSeq = w.LastSeq()
	ap := c.AutoPause
	side := c.PlayerSide
	for i := range evs {
		e := &evs[i]
		u := w.Unit(e.Unit)
		mine := u != nil && u.Side == side
		c.ReasonHex, c.ReasonAt = e.Hex, true
		switch e.Type {
		case EvEnemySpotted:
			if ap.EnemySpotted && e.VisibleTo(side) && u != nil && u.Side != side {
				return "Замечен противник: " + u.Name
			}
		case EvUnitDied:
			if ap.UnitDied && mine {
				return "Погиб юнит: " + u.Name
			}
			if ap.ImportantDied && e.Dangerous && e.VisibleTo(side) {
				return "Погиб важный юнит: " + u.Name
			}
		case EvDamage:
			if ap.DangerousAttack && mine && e.Dangerous {
				return fmt.Sprintf("Опасная атака: %s получил %d урона", u.Name, e.Amount)
			}
		case EvSkillPrepStarted, EvObjectPlaced:
			if ap.DangerousAttack && e.Dangerous && e.VisibleTo(side) {
				src := w.Unit(e.Unit)
				if src != nil && src.Side != side {
					for _, h := range e.Area {
						if t := w.UnitAt(h); t != nil && t.Side == side {
							return fmt.Sprintf("Опасное умение «%s» нацелено на ваш отряд", w.skillOrObjectName(e.Skill))
						}
					}
				}
			}
		case EvActionInterrupted:
			if ap.Interrupted && mine {
				return "Прервано действие: " + u.Name
			}
		case EvOrderFailed:
			if ap.OrderFailed && mine {
				return "Приказ не выполнен: " + u.Name
			}
		case EvQueueFinished:
			if ap.QueueFinished && mine {
				return "Очередь выполнена: " + u.Name
			}
		case EvBattleEnded:
			return e.Text
		}
	}
	return ""
}

func (w *World) skillOrObjectName(id string) string {
	if s := w.C.Skills[id]; s != nil {
		return s.Name
	}
	if o := w.C.Objects[id]; o != nil {
		return o.Name
	}
	return id
}
