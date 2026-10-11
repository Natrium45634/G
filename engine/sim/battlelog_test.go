package sim

import (
	"fmt"
	"os"
	"testing"

	"hextactics/engine/hex"
)

// TestDebugBattleLog печатает журнал полного боя ИИ против ИИ (диагностика баланса):
//
//	BATTLE_LOG=1 go test ./engine/sim -run DebugBattleLog -v
func TestDebugBattleLog(t *testing.T) {
	if os.Getenv("BATTLE_LOG") == "" {
		t.Skip()
	}
	c := loadContent(t)
	w, _ := New(c, "river_crossing", 777)
	dests := []hex.Hex{at(12, 7), at(11, 8), at(11, 5), at(10, 6)}
	for i, u := range w.AliveUnits(0) {
		mustQueue(t, w, u, OrderSpec{Type: OrderMove, Dest: dests[i]})
	}
	for i := range w.Sides {
		w.Sides[i].Controller = "ai"
	}
	for w.Tick < w.Ticks(240) && w.Outcome == "" {
		w.Step()
	}
	for _, e := range w.Events {
		if e.Text != "" {
			fmt.Printf("[%6.2f] %s\n", float64(e.Tick)/20, e.Text)
		}
	}
	for _, u := range w.Units {
		fmt.Printf("%-16s side=%d hp=%d/%d dead=%v pos=%v\n", u.Name, u.Side, u.HP, u.HPMax(), u.Dead, u.Pos)
	}
	fmt.Println("outcome:", w.Outcome, "tick", w.Tick)
}
