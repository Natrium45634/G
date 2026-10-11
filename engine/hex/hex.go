// Package hex реализует геометрию гексагональной сетки в аксиальных координатах (q, r).
//
// Используется раскладка «остриём вверх» (pointy-top). Третья кубическая координата
// вычисляется как s = -q - r, поэтому ограничение q + r + s = 0 выполняется всегда.
// Пакет не зависит от остальных частей движка и не использует float в логике,
// влияющей на симуляцию (направления и секторы считаются в целых числах).
package hex

import (
	"fmt"
	"math"
	"sort"
)

// Hex — клетка в аксиальных координатах.
type Hex struct {
	Q int `json:"q"`
	R int `json:"r"`
}

// New создаёт гекс.
func New(q, r int) Hex { return Hex{Q: q, R: r} }

// S — третья кубическая координата.
func (h Hex) S() int { return -h.Q - h.R }

func (h Hex) Add(o Hex) Hex      { return Hex{h.Q + o.Q, h.R + o.R} }
func (h Hex) Sub(o Hex) Hex      { return Hex{h.Q - o.Q, h.R - o.R} }
func (h Hex) Scale(k int) Hex    { return Hex{h.Q * k, h.R * k} }
func (h Hex) String() string     { return fmt.Sprintf("(%d,%d)", h.Q, h.R) }
func (h Hex) Neighbor(d Dir) Hex { return h.Add(d.Vec()) }

// Neighbors возвращает шесть соседей в порядке направлений 0..5.
func (h Hex) Neighbors() [6]Hex {
	var out [6]Hex
	for d := Dir(0); d < 6; d++ {
		out[d] = h.Neighbor(d)
	}
	return out
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// Distance — расстояние в клетках: (|dq| + |dr| + |ds|) / 2.
func Distance(a, b Hex) int {
	d := a.Sub(b)
	return (abs(d.Q) + abs(d.R) + abs(d.S())) / 2
}

// Length — расстояние от начала координат.
func (h Hex) Length() int { return Distance(h, Hex{}) }

// ---------------------------------------------------------------------------
// Направления

// Dir — одно из шести направлений. Нумерация против часовой стрелки на экране,
// начиная с востока: 0=В, 1=СВ, 2=СЗ, 3=З, 4=ЮЗ, 5=ЮВ (угол = 60°·d).
type Dir int

const NumDirs = 6

var dirVecs = [6]Hex{{1, 0}, {1, -1}, {0, -1}, {-1, 0}, {-1, 1}, {0, 1}}

var dirNames = [6]string{"В", "СВ", "СЗ", "З", "ЮЗ", "ЮВ"}
var dirArrows = [6]string{"→", "↗", "↖", "←", "↙", "↘"}

// Vec — смещение соседней клетки в данном направлении.
func (d Dir) Vec() Hex { return dirVecs[d.Norm()] }

// Norm приводит направление к диапазону 0..5.
func (d Dir) Norm() Dir { return Dir(((int(d) % 6) + 6) % 6) }

// Rotate поворачивает на steps шагов по 60° (положительное — против часовой).
func (d Dir) Rotate(steps int) Dir { return Dir(int(d) + steps).Norm() }

// Opposite — противоположное направление.
func (d Dir) Opposite() Dir { return d.Rotate(3) }

// Name — короткое русское имя направления.
func (d Dir) Name() string { return dirNames[d.Norm()] }

// Arrow — стрелка для текстового вывода.
func (d Dir) Arrow() string { return dirArrows[d.Norm()] }

// AngleDeg — экранный угол направления (против часовой от востока).
func (d Dir) AngleDeg() float64 { return float64(d.Norm()) * 60 }

// Delta — минимальная разница между направлениями: min(|f-a|, 6-|f-a|), 0..3.
func Delta(f, a Dir) int {
	x := abs(int(f.Norm()) - int(a.Norm()))
	if 6-x < x {
		return 6 - x
	}
	return x
}

// SignedSteps — кратчайший поворот от from к to: от -2 до +3 (положительное — против часовой).
func SignedSteps(from, to Dir) int {
	x := (int(to.Norm()) - int(from.Norm()) + 6) % 6
	if x > 3 {
		return x - 6
	}
	return x
}

// dot4 — учетверённое скалярное произведение пиксельных векторов v и направления d.
// Пиксельный вектор (pointy-top, размер 1): (√3(q + r/2), 1.5r).
// 4·dot = 3(2qv+rv)(2qd+rd) + 9·rv·rd — целое число.
func dot4(v Hex, d Dir) int {
	dv := d.Vec()
	return 3*(2*v.Q+v.R)*(2*dv.Q+dv.R) + 9*v.R*dv.R
}

// len4sq — учетверённый квадрат длины пиксельного вектора: 3(2q+r)² + 9r².
func len4sq(v Hex) int {
	return 3*(2*v.Q+v.R)*(2*v.Q+v.R) + 9*v.R*v.R
}

// DirectionsTo возвращает ближайшее по углу направление от from к to.
// Если цель лежит ровно между двумя направлениями (±30°), tie=true и alt — второе направление
// (primary всегда равно alt-1 по модулю 6, т.е. выбор детерминирован).
// ok=false, если from == to.
func DirectionsTo(from, to Hex) (primary Dir, alt Dir, tie bool, ok bool) {
	v := to.Sub(from)
	if v.Q == 0 && v.R == 0 {
		return 0, 0, false, false
	}
	best := -1 << 62
	var bestDirs []Dir
	for d := Dir(0); d < 6; d++ {
		x := dot4(v, d)
		if x > best {
			best = x
			bestDirs = bestDirs[:0]
			bestDirs = append(bestDirs, d)
		} else if x == best {
			bestDirs = append(bestDirs, d)
		}
	}
	if len(bestDirs) == 1 {
		return bestDirs[0], bestDirs[0], false, true
	}
	a, b := bestDirs[0], bestDirs[1]
	// Пара соседних направлений; для пары (0,5) первичным считаем 5 (5 → 0 против часовой).
	if a == 0 && b == 5 {
		return 5, 0, true, true
	}
	return a, b, true, true
}

// DirectionTo — направление от from к to (при равенстве — первое по часовой из двух).
func DirectionTo(from, to Hex) (Dir, bool) {
	p, _, _, ok := DirectionsTo(from, to)
	return p, ok
}

// WithinAngle проверяет, что вектор from→to лежит в пределах ±halfDeg градусов от направления d.
// Поддерживаются halfDeg = 30, 60, 90 (точные целочисленные сравнения, границы включаются).
func WithinAngle(from, to Hex, d Dir, halfDeg int) bool {
	v := to.Sub(from)
	if v.Q == 0 && v.R == 0 {
		return false
	}
	D := dot4(v, d)
	V := len4sq(v)
	switch halfDeg {
	case 90:
		return D >= 0
	case 60:
		return D >= 0 && D*D >= 3*V
	case 30:
		return D >= 0 && D*D >= 9*V
	default:
		// произвольный угол — через float (используется только для визуализации)
		ang := math.Acos(float64(D) / math.Sqrt(float64(V)*12))
		return ang*180/math.Pi <= float64(halfDeg)+1e-9
	}
}

// ---------------------------------------------------------------------------
// Области

// Range возвращает все клетки на расстоянии ≤ radius от center (детерминированный порядок).
func Range(center Hex, radius int) []Hex {
	var out []Hex
	for q := -radius; q <= radius; q++ {
		r1 := max(-radius, -q-radius)
		r2 := min(radius, -q+radius)
		for r := r1; r <= r2; r++ {
			out = append(out, center.Add(Hex{q, r}))
		}
	}
	return out
}

// Ring возвращает кольцо клеток на расстоянии ровно radius.
func Ring(center Hex, radius int) []Hex {
	if radius <= 0 {
		return []Hex{center}
	}
	var out []Hex
	h := center.Add(Dir(4).Vec().Scale(radius))
	for side := 0; side < 6; side++ {
		for i := 0; i < radius; i++ {
			out = append(out, h)
			h = h.Neighbor(Dir(side))
		}
	}
	return out
}

// LineDir — луч из origin в направлении d длиной length (без самой origin).
func LineDir(origin Hex, d Dir, length int) []Hex {
	out := make([]Hex, 0, length)
	for i := 1; i <= length; i++ {
		out = append(out, origin.Add(d.Vec().Scale(i)))
	}
	return out
}

// Cone — конус из origin в направлении d: клетки на расстоянии 1..length в пределах ±halfDeg.
func Cone(origin Hex, d Dir, length int, halfDeg int) []Hex {
	var out []Hex
	for _, h := range Range(origin, length) {
		if h == origin {
			continue
		}
		if WithinAngle(origin, h, d, halfDeg) {
			out = append(out, h)
		}
	}
	SortHexes(out, origin)
	return out
}

// Arc — дуга из трёх соседних клеток: фронт и два ближних фланга направления d.
func Arc(origin Hex, d Dir) []Hex {
	return []Hex{origin.Neighbor(d.Rotate(-1)), origin.Neighbor(d), origin.Neighbor(d.Rotate(1))}
}

// SortHexes сортирует клетки по расстоянию от origin, затем по r, затем по q (стабильный порядок).
func SortHexes(hs []Hex, origin Hex) {
	sort.Slice(hs, func(i, j int) bool {
		di, dj := Distance(origin, hs[i]), Distance(origin, hs[j])
		if di != dj {
			return di < dj
		}
		if hs[i].R != hs[j].R {
			return hs[i].R < hs[j].R
		}
		return hs[i].Q < hs[j].Q
	})
}

// ---------------------------------------------------------------------------
// Линия (для линии видимости)

type fcube struct{ x, y, z float64 }

func cubeRound(c fcube) Hex {
	rx, ry, rz := math.Round(c.x), math.Round(c.y), math.Round(c.z)
	dx, dy, dz := math.Abs(rx-c.x), math.Abs(ry-c.y), math.Abs(rz-c.z)
	if dx > dy && dx > dz {
		rx = -ry - rz
	} else if dy > dz {
		ry = -rx - rz
	}
	return Hex{int(rx), int(ry)}
}

// Line возвращает клетки отрезка от a до b включительно (кубическая интерполяция
// с фиксированным малым смещением, чтобы границы разрешались детерминированно).
func Line(a, b Hex) []Hex {
	n := Distance(a, b)
	out := make([]Hex, 0, n+1)
	if n == 0 {
		return append(out, a)
	}
	const e1, e2 = 1e-6, 2e-6
	ax, ay, az := float64(a.Q)+e1, float64(a.R)+e2, float64(a.S())-e1-e2
	bx, by, bz := float64(b.Q)+e1, float64(b.R)+e2, float64(b.S())-e1-e2
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		out = append(out, cubeRound(fcube{ax + (bx-ax)*t, ay + (by-ay)*t, az + (bz-az)*t}))
	}
	return out
}

// ---------------------------------------------------------------------------
// Пиксели (только для интерфейса)

// ToPixel — центр клетки в пикселях для раскладки pointy-top с радиусом size.
func ToPixel(h Hex, size float64) (x, y float64) {
	x = size * math.Sqrt(3) * (float64(h.Q) + float64(h.R)/2)
	y = size * 1.5 * float64(h.R)
	return
}

// FromPixel — клетка, содержащая точку.
func FromPixel(x, y, size float64) Hex {
	q := (math.Sqrt(3)/3*x - y/3) / size
	r := (2.0 / 3 * y) / size
	return cubeRound(fcube{q, r, -q - r})
}

// Corner — вершина i (0..5) шестиугольника относительно центра.
func Corner(size float64, i int) (x, y float64) {
	ang := math.Pi / 180 * (60*float64(i) - 30)
	return size * math.Cos(ang), size * math.Sin(ang)
}

// ---------------------------------------------------------------------------
// Смещённые координаты (odd-r) — для описания прямоугольных карт в данных.

// FromOffset переводит (col,row) в аксиальные координаты (нечётные строки сдвинуты вправо).
func FromOffset(col, row int) Hex {
	return Hex{Q: col - (row-(row&1))/2, R: row}
}

// ToOffset — обратное преобразование.
func ToOffset(h Hex) (col, row int) {
	return h.Q + (h.R-(h.R&1))/2, h.R
}
