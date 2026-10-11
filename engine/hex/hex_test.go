package hex

import "testing"

func TestNeighborsAreSixDistinctAtDistanceOne(t *testing.T) {
	for _, c := range []Hex{{0, 0}, {3, -2}, {-5, 7}} {
		seen := map[Hex]bool{}
		for i, n := range c.Neighbors() {
			if Distance(c, n) != 1 {
				t.Fatalf("сосед %d клетки %v на расстоянии %d", i, c, Distance(c, n))
			}
			if seen[n] {
				t.Fatalf("повтор соседа %v", n)
			}
			seen[n] = true
			if n.Q+n.R+n.S() != 0 {
				t.Fatalf("нарушено q+r+s=0 для %v", n)
			}
		}
		if len(seen) != 6 {
			t.Fatalf("ожидалось 6 соседей, получено %d", len(seen))
		}
	}
}

func TestDistance(t *testing.T) {
	cases := []struct {
		a, b Hex
		d    int
	}{
		{Hex{0, 0}, Hex{0, 0}, 0},
		{Hex{0, 0}, Hex{1, 0}, 1},
		{Hex{0, 0}, Hex{2, -1}, 2},
		{Hex{0, 0}, Hex{3, -3}, 3},
		{Hex{-2, 1}, Hex{2, -1}, 4},
		{Hex{1, 2}, Hex{-1, -2}, 6},
		{Hex{5, 0}, Hex{0, 5}, 5},
	}
	for _, c := range cases {
		if got := Distance(c.a, c.b); got != c.d {
			t.Errorf("Distance(%v,%v)=%d, ожидалось %d", c.a, c.b, got, c.d)
		}
		if Distance(c.a, c.b) != Distance(c.b, c.a) {
			t.Errorf("расстояние несимметрично")
		}
	}
}

func TestDirectionRoundTrip(t *testing.T) {
	for d := Dir(0); d < 6; d++ {
		got, ok := DirectionTo(Hex{}, d.Vec())
		if !ok || got != d {
			t.Errorf("направление к соседу %d определено как %d", d, got)
		}
		if d.Opposite().Opposite() != d || Delta(d, d.Opposite()) != 3 {
			t.Errorf("противоположное направление для %d неверно", d)
		}
		far := d.Vec().Scale(5)
		if got, _ := DirectionTo(Hex{}, far); got != d {
			t.Errorf("направление к дальней клетке %v: %d, ожидалось %d", far, got, d)
		}
	}
}

func TestDelta(t *testing.T) {
	cases := [][3]int{{0, 0, 0}, {0, 1, 1}, {0, 5, 1}, {0, 2, 2}, {0, 4, 2}, {0, 3, 3}, {5, 2, 3}, {4, 1, 3}, {1, 5, 2}}
	for _, c := range cases {
		if got := Delta(Dir(c[0]), Dir(c[1])); got != c[2] {
			t.Errorf("Delta(%d,%d)=%d, ожидалось %d", c[0], c[1], got, c[2])
		}
	}
}

func TestDirectionTieIsReported(t *testing.T) {
	// (1,1) лежит ровно между В(0) и ЮВ(5)
	p, a, tie, ok := DirectionsTo(Hex{}, Hex{1, 1})
	if !ok || !tie || p != 5 || a != 0 {
		t.Fatalf("ожидалась ничья 5/0, получено p=%d a=%d tie=%v", p, a, tie)
	}
	// (2,-1) лежит между В(0) и СВ(1)
	p, a, tie, _ = DirectionsTo(Hex{}, Hex{2, -1})
	if !tie || p != 0 || a != 1 {
		t.Fatalf("ожидалась ничья 0/1, получено p=%d a=%d tie=%v", p, a, tie)
	}
}

func TestLineIsContinuous(t *testing.T) {
	for _, pair := range [][2]Hex{{{0, 0}, {5, -2}}, {{-3, 4}, {4, -4}}, {{0, 0}, {0, 0}}, {{2, 2}, {-6, 1}}} {
		l := Line(pair[0], pair[1])
		if l[0] != pair[0] || l[len(l)-1] != pair[1] {
			t.Fatalf("линия не начинается/заканчивается в концах: %v", l)
		}
		if len(l) != Distance(pair[0], pair[1])+1 {
			t.Fatalf("длина линии %d", len(l))
		}
		for i := 1; i < len(l); i++ {
			if Distance(l[i-1], l[i]) != 1 {
				t.Fatalf("разрыв линии: %v", l)
			}
		}
	}
}

func TestRangeAndRingCounts(t *testing.T) {
	for r := 0; r <= 4; r++ {
		if got, want := len(Range(Hex{}, r)), 1+3*r*(r+1); got != want {
			t.Errorf("Range(%d)=%d клеток, ожидалось %d", r, got, want)
		}
		ring := Ring(Hex{2, -1}, r)
		want := 6 * r
		if r == 0 {
			want = 1
		}
		if len(ring) != want {
			t.Errorf("Ring(%d)=%d клеток, ожидалось %d", r, len(ring), want)
		}
		for _, h := range ring {
			if Distance(h, Hex{2, -1}) != r {
				t.Errorf("клетка кольца %v на неверном расстоянии", h)
			}
		}
	}
}

func TestConeShapes(t *testing.T) {
	// Конус 60° (±30°) длины 3: 1 + 3 + 3 клетки
	c := Cone(Hex{}, 0, 3, 30)
	if len(c) != 7 {
		t.Fatalf("конус 60° длины 3 содержит %d клеток: %v", len(c), c)
	}
	for d := Dir(0); d < 6; d++ {
		if n := len(Cone(Hex{}, d, 3, 30)); n != 7 {
			t.Fatalf("конус в направлении %d: %d клеток", d, n)
		}
		// 120° конус длины 2: 3 клетки рядом + 6 на расстоянии 2 (включая границы ±60°)
		if n := len(Cone(Hex{}, d, 2, 60)); n != 3+5 {
			t.Fatalf("конус 120° в направлении %d: %d клеток", d, n)
		}
	}
	arc := Arc(Hex{}, 0)
	if arc[1] != (Hex{1, 0}) || arc[0] != (Hex{0, 1}) || arc[2] != (Hex{1, -1}) {
		t.Fatalf("дуга неверна: %v", arc)
	}
}

func TestOffsetRoundTrip(t *testing.T) {
	for row := -3; row < 6; row++ {
		for col := -3; col < 6; col++ {
			h := FromOffset(col, row)
			c, r := ToOffset(h)
			if c != col || r != row {
				t.Fatalf("offset (%d,%d) → %v → (%d,%d)", col, row, h, c, r)
			}
		}
	}
}

func TestPixelRoundTrip(t *testing.T) {
	for _, h := range Range(Hex{1, -2}, 4) {
		x, y := ToPixel(h, 30)
		if got := FromPixel(x+3, y-2, 30); got != h {
			t.Fatalf("FromPixel(ToPixel(%v))=%v", h, got)
		}
	}
}

func TestSignedSteps(t *testing.T) {
	if SignedSteps(0, 1) != 1 || SignedSteps(1, 0) != -1 || SignedSteps(0, 5) != -1 || SignedSteps(0, 3) != 3 || SignedSteps(4, 1) != 3 {
		t.Fatal("SignedSteps неверен")
	}
}
