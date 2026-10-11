package sim

import (
	"fmt"

	"hextactics/engine/data"
	"hextactics/engine/hex"
)

// Object — объект мира: ловушка, отложенный удар и т.п. (экземпляр ObjectDef).
type Object struct {
	ID      int     `json:"id"`
	Def     string  `json:"def"`
	Hex     hex.Hex `json:"hex"`
	Side    int     `json:"side"`
	Owner   int     `json:"owner"`
	Created int     `json:"created"`
	Trigger int     `json:"trigger,omitempty"` // тик срабатывания (для timer)
	Expires int     `json:"expires,omitempty"` // тик исчезновения (0 — никогда)
	Done    bool    `json:"done,omitempty"`
}

// ObjectDef — определение объекта.
func (w *World) ObjectDef(o *Object) *data.ObjectDef { return w.C.Objects[o.Def] }

// ObjectVisibleTo — видит ли сторона объект (скрытые ловушки видны только владельцу).
func (w *World) ObjectVisibleTo(o *Object, side int) bool {
	d := w.ObjectDef(o)
	return d == nil || !d.HiddenFromEnemies || o.Side == side
}

func (w *World) placeObject(def string, ctx *OpContext) {
	d := w.C.Objects[def]
	if d == nil {
		return
	}
	side, owner := ctx.Side, 0
	if ctx.Source != nil {
		side, owner = ctx.Source.Side, ctx.Source.ID
	}
	o := &Object{ID: w.NextObjID, Def: def, Hex: ctx.Hex, Side: side, Owner: owner, Created: w.Tick}
	w.NextObjID++
	if d.Trigger == "timer" {
		o.Trigger = w.Tick + w.Ticks(d.Delay)
	}
	if d.Lifetime > 0 {
		o.Expires = w.Tick + w.Ticks(d.Lifetime)
	}
	w.Objects = append(w.Objects, o)
	ownerName := "?"
	if ctx.Source != nil {
		ownerName = ctx.Source.Name
	}
	text := fmt.Sprintf("%s: «%s» установлен", ownerName, d.Name)
	if d.Trigger == "timer" {
		text = fmt.Sprintf("%s: «%s» — удар через %.1f с", ownerName, d.Name, d.Delay)
	}
	ev := Event{Type: EvObjectPlaced, Unit: owner, Hex: ctx.Hex, Skill: def, Dangerous: d.Dangerous, Text: text, Amount: o.ID}
	if d.Radius > 0 {
		ev.Area = hex.Range(ctx.Hex, d.Radius)
	} else {
		ev.Area = []hex.Hex{ctx.Hex}
	}
	e := w.emit(ev)
	if d.HiddenFromEnemies {
		e.Visible = 1 << uint(side)
	} else {
		e.Visible = ^uint32(0)
	}
}

// updateObjects — срабатывание таймеров и истечение срока объектов.
func (w *World) updateObjects() {
	for _, o := range w.Objects {
		if o.Done {
			continue
		}
		if o.Trigger > 0 && w.Tick >= o.Trigger {
			w.triggerObject(o, nil)
			continue
		}
		if o.Expires > 0 && w.Tick >= o.Expires {
			o.Done = true
		}
	}
	// Удаляем отработавшие объекты (стабильный порядок сохраняется)
	kept := w.Objects[:0]
	for _, o := range w.Objects {
		if !o.Done {
			kept = append(kept, o)
		}
	}
	w.Objects = kept
}

// checkTraps — юнит вошёл в клетку: срабатывание ловушек.
func (w *World) checkTraps(u *Unit) {
	for _, o := range w.Objects {
		if o.Done || o.Hex != u.Pos {
			continue
		}
		d := w.ObjectDef(o)
		if d == nil {
			continue
		}
		if d.Trigger == "any_enter" || (d.Trigger == "enemy_enter" && w.Hostile(o.Side, u.Side)) {
			w.triggerObject(o, u)
			if u.Dead {
				return
			}
		}
	}
}

func (w *World) triggerObject(o *Object, victim *Unit) {
	d := w.ObjectDef(o)
	o.Done = true
	if d == nil {
		return
	}
	area := []hex.Hex{o.Hex}
	if d.Radius > 0 {
		area = nil
		for _, h := range hex.Range(o.Hex, d.Radius) {
			if c := w.Map.Cell(h); c != nil && !c.BlocksLOS() && (h == o.Hex || w.Map.LineOfSight(o.Hex, h).Clear) {
				area = append(area, h)
			}
		}
	}
	text := fmt.Sprintf("Срабатывает «%s»", d.Name)
	if victim != nil {
		text = fmt.Sprintf("%s попадает в «%s»", victim.Name, d.Name)
	}
	ev := w.emit(Event{Type: EvObjectTriggered, Unit: idOf(victim), Hex: o.Hex, Skill: o.Def, Area: area, Text: text, Amount: o.ID})
	ev.Visible = ^uint32(0)
	src := w.Unit(o.Owner)
	var targets []*Unit
	for _, h := range area {
		if t := w.UnitAt(h); t != nil && !t.Dead {
			targets = append(targets, t)
		}
	}
	if victim != nil && d.Radius == 0 {
		targets = []*Unit{victim}
	}
	for _, t := range targets {
		w.applyOps(d.Ops, &OpContext{Source: src, Self: src, Target: t, Label: d.Name, Hex: o.Hex, Side: o.Side})
	}
}

// summon — призыв юнитов на свободные клетки рядом с точкой.
func (w *World) summon(class string, count int, ctx *OpContext) {
	side := ctx.Side
	if ctx.Source != nil {
		side = ctx.Source.Side
	}
	cand := []hex.Hex{ctx.Hex}
	for r := 1; r <= 2; r++ {
		cand = append(cand, hex.Ring(ctx.Hex, r)...)
	}
	prof := footStand
	if cls := w.C.Classes[class]; cls != nil {
		prof.Class = cls.MoveClass
	}
	placed := 0
	for _, h := range cand {
		if placed >= count {
			break
		}
		if ok, _ := w.Map.CanStand(h, &prof); !ok || w.Blocked(nil, h) {
			continue
		}
		facing := hex.Dir(0)
		if ctx.Source != nil {
			facing = ctx.Source.Facing
		}
		u, err := w.spawn(class, "", side, h, facing)
		if err != nil {
			continue
		}
		u.Summoned = true
		u.AP = 0
		placed++
		owner := "?"
		if ctx.Source != nil {
			owner = ctx.Source.Name
		}
		w.emit(Event{Type: EvUnitSummoned, Unit: u.ID, Other: idOf(ctx.Source), Hex: h,
			Text: fmt.Sprintf("%s призывает: %s", owner, u.Name)})
	}
}
