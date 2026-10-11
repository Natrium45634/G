package ui

import (
	"hextactics/engine/hex"
	"hextactics/engine/sim"
)

// DebugSetup готовит сцену для автоматических снимков экрана (флаг -setup).
// Использует только командный интерфейс симуляции.
func DebugSetup(g *Game, kind string) {
	b, ok := g.cur.(*Battle)
	if !ok {
		return
	}
	us := b.w.AliveUnits(b.player)
	at := hex.FromOffset
	q := func(u *sim.Unit, spec sim.OrderSpec) {
		b.w.Apply(sim.Command{Kind: sim.CmdQueue, Side: b.player, Unit: u.ID, Index: -1, Order: &spec})
	}
	switch kind {
	case "plan":
		if len(us) < 4 {
			return
		}
		k, wr, r, m := us[0], us[1], us[2], us[3]
		q(k, sim.OrderSpec{Type: sim.OrderMove, Dest: at(7, 4)})
		q(k, sim.OrderSpec{Type: sim.OrderTurn, Dir: 1})
		q(wr, sim.OrderSpec{Type: sim.OrderMove, Dest: at(6, 6)})
		q(r, sim.OrderSpec{Type: sim.OrderMove, Dest: at(4, 1)})
		q(r, sim.OrderSpec{Type: sim.OrderMove, Dest: at(6, 1)})
		q(r, sim.OrderSpec{Type: sim.OrderSkill, Skill: "piercing_arrow", Dir: 0})
		q(r, sim.OrderSpec{Type: sim.OrderMove, Dest: at(5, 3)})
		q(m, sim.OrderSpec{Type: sim.OrderMove, Dest: at(4, 5)})
		b.sel = r.ID
		b.setMode(ModeSkill, "fire_cone")
		b.sel = m.ID
	case "combat":
		dests := []hex.Hex{at(12, 7), at(11, 8), at(11, 5), at(10, 6)}
		for i, u := range us {
			if i < len(dests) {
				q(u, sim.OrderSpec{Type: sim.OrderMove, Dest: dests[i]})
			}
		}
		for i := range b.w.Sides {
			b.w.Sides[i].Controller = "ai"
		}
		for i := 0; i < b.w.Ticks(19); i++ {
			b.w.Step()
		}
		for i := range b.w.Sides {
			if b.w.Sides[i].ID == b.player {
				b.w.Sides[i].Controller = "player"
			}
		}
		b.consumeEvents()
		b.logDetails = false
		if len(us) > 0 {
			b.sel = us[0].ID
		}
		b.centerOn(b.partyCenter())
		b.zoom = 1.2
	}
}
