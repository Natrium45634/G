package gamemap

import (
	"container/heap"

	"hextactics/engine/hex"
)

// Blocker сообщает, занята ли клетка динамическим препятствием (другим юнитом).
type Blocker func(h hex.Hex) bool

// Reach — узел области доступности.
type Reach struct {
	Cost int
	Prev hex.Hex
}

// PathResult — результат поиска пути.
type PathResult struct {
	OK        bool
	Code      string
	Path      []hex.Hex // без стартовой клетки
	StepCosts []int
	Cost      int
}

type pqItem struct {
	h        hex.Hex
	pri, aux int
	seq      int
}

type pq []pqItem

func (q pq) Len() int { return len(q) }
func (q pq) Less(i, j int) bool {
	if q[i].pri != q[j].pri {
		return q[i].pri < q[j].pri
	}
	if q[i].aux != q[j].aux {
		return q[i].aux < q[j].aux
	}
	return q[i].seq < q[j].seq
}
func (q pq) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *pq) Push(x any)   { *q = append(*q, x.(pqItem)) }
func (q *pq) Pop() any {
	old := *q
	it := old[len(old)-1]
	*q = old[:len(old)-1]
	return it
}

// Reachable — область доступности (Dijkstra) в пределах бюджета милли-ОД.
// budget < 0 — без ограничения.
func (m *Map) Reachable(start hex.Hex, p *MoveProfile, blocked Blocker, budget int) map[hex.Hex]Reach {
	out := map[hex.Hex]Reach{start: {Cost: 0, Prev: start}}
	q := &pq{{h: start}}
	seq := 0
	for q.Len() > 0 {
		cur := heap.Pop(q).(pqItem)
		if r, ok := out[cur.h]; ok && r.Cost < cur.pri {
			continue
		}
		for d := hex.Dir(0); d < 6; d++ {
			n := cur.h.Neighbor(d)
			if blocked != nil && blocked(n) {
				continue
			}
			c, code := m.StepCost(cur.h, n, p)
			if code != "" {
				continue
			}
			nc := cur.pri + c
			if budget >= 0 && nc > budget {
				continue
			}
			if r, ok := out[n]; ok && r.Cost <= nc {
				continue
			}
			out[n] = Reach{Cost: nc, Prev: cur.h}
			seq++
			heap.Push(q, pqItem{h: n, pri: nc, seq: seq})
		}
	}
	return out
}

// ReconstructPath восстанавливает путь из области доступности.
func ReconstructPath(field map[hex.Hex]Reach, start, goal hex.Hex) []hex.Hex {
	if _, ok := field[goal]; !ok {
		return nil
	}
	var rev []hex.Hex
	for h := goal; h != start; h = field[h].Prev {
		rev = append(rev, h)
	}
	out := make([]hex.Hex, len(rev))
	for i := range rev {
		out[i] = rev[len(rev)-1-i]
	}
	return out
}

func (m *Map) finish(prev map[hex.Hex]hex.Hex, start, goal hex.Hex, p *MoveProfile) PathResult {
	var rev []hex.Hex
	for h := goal; h != start; h = prev[h] {
		rev = append(rev, h)
	}
	res := PathResult{OK: true}
	cur := start
	for i := len(rev) - 1; i >= 0; i-- {
		c, _ := m.StepCost(cur, rev[i], p)
		res.Path = append(res.Path, rev[i])
		res.StepCosts = append(res.StepCosts, c)
		res.Cost += c
		cur = rev[i]
	}
	return res
}

// FindPath — A* от start до goal. Клетка goal сама не должна быть занята.
func (m *Map) FindPath(start, goal hex.Hex, p *MoveProfile, blocked Blocker) PathResult {
	if start == goal {
		return PathResult{OK: true}
	}
	if ok, code := m.CanStand(goal, p); !ok {
		return PathResult{Code: code}
	}
	if blocked != nil && blocked(goal) {
		return PathResult{Code: CodeOccupied}
	}
	minStep := m.minStep(p)
	hfun := func(h hex.Hex) int { return hex.Distance(h, goal) * minStep }
	g := map[hex.Hex]int{start: 0}
	prev := map[hex.Hex]hex.Hex{}
	closed := map[hex.Hex]bool{}
	q := &pq{{h: start, pri: hfun(start), aux: hfun(start)}}
	seq := 0
	for q.Len() > 0 {
		cur := heap.Pop(q).(pqItem)
		if closed[cur.h] {
			continue
		}
		if cur.h == goal {
			return m.finish(prev, start, goal, p)
		}
		closed[cur.h] = true
		for d := hex.Dir(0); d < 6; d++ {
			n := cur.h.Neighbor(d)
			if closed[n] {
				continue
			}
			if blocked != nil && blocked(n) {
				continue
			}
			c, code := m.StepCost(cur.h, n, p)
			if code != "" {
				continue
			}
			ng := g[cur.h] + c
			if old, ok := g[n]; ok && old <= ng {
				continue
			}
			g[n] = ng
			prev[n] = cur.h
			seq++
			h := hfun(n)
			heap.Push(q, pqItem{h: n, pri: ng + h, aux: h, seq: seq})
		}
	}
	return PathResult{Code: CodeNoPath}
}

// FindPathToAny — Dijkstra до ближайшей (по стоимости) клетки, удовлетворяющей goal.
// Если стартовая клетка подходит, возвращается пустой путь.
func (m *Map) FindPathToAny(start hex.Hex, goal func(hex.Hex) bool, p *MoveProfile, blocked Blocker, budget int) PathResult {
	if goal(start) {
		return PathResult{OK: true}
	}
	dist := map[hex.Hex]int{start: 0}
	prev := map[hex.Hex]hex.Hex{}
	q := &pq{{h: start}}
	seq := 0
	for q.Len() > 0 {
		cur := heap.Pop(q).(pqItem)
		if d, ok := dist[cur.h]; ok && d < cur.pri {
			continue
		}
		if cur.h != start && goal(cur.h) {
			return m.finish(prev, start, cur.h, p)
		}
		for d := hex.Dir(0); d < 6; d++ {
			n := cur.h.Neighbor(d)
			if blocked != nil && blocked(n) {
				continue
			}
			c, code := m.StepCost(cur.h, n, p)
			if code != "" {
				continue
			}
			nd := cur.pri + c
			if budget >= 0 && nd > budget {
				continue
			}
			if old, ok := dist[n]; ok && old <= nd {
				continue
			}
			dist[n] = nd
			prev[n] = cur.h
			seq++
			heap.Push(q, pqItem{h: n, pri: nd, seq: seq})
		}
	}
	return PathResult{Code: CodeNoPath}
}

// ---------------------------------------------------------------------------
// Линия видимости

// LOSResult — результат проверки линии видимости.
type LOSResult struct {
	Clear     bool
	Blocker   hex.Hex   // первая перекрывающая клетка
	SoftCover int       // сумма частичных препятствий, %
	Cells     []hex.Hex // клетки линии
}

func lessHex(a, b hex.Hex) bool {
	if a.R != b.R {
		return a.R < b.R
	}
	return a.Q < b.Q
}

// LineOfSight проверяет видимость между центрами клеток. Результат симметричен:
// линия всегда строится в каноническом порядке концов.
// Перекрывают обзор: клетки с BlocksLOS, закрытые двери и промежуточные клетки
// выше обоих концов. Частичные препятствия суммируются в SoftCover.
func (m *Map) LineOfSight(a, b hex.Hex) LOSResult {
	ca, cb := m.Cell(a), m.Cell(b)
	if ca == nil || cb == nil {
		return LOSResult{}
	}
	from, to := a, b
	if lessHex(b, a) {
		from, to = b, a
	}
	line := hex.Line(from, to)
	if from != a {
		for i, j := 0, len(line)-1; i < j; i, j = i+1, j-1 {
			line[i], line[j] = line[j], line[i]
		}
	}
	res := LOSResult{Clear: true, Cells: line}
	top := max(ca.Height, cb.Height)
	for i := 1; i < len(line)-1; i++ {
		c := m.Cell(line[i])
		if c == nil || c.BlocksLOS() || c.Height > top {
			res.Clear = false
			res.Blocker = line[i]
			return res
		}
		res.SoftCover += c.LOSCover()
	}
	return res
}
