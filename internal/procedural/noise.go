package procedural

import "math"

func fade(t float64) float64 { return t * t * t * (t*(t*6-15) + 10) }

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

var grad2 = [8][2]float64{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {0.7071, 0.7071}, {-0.7071, 0.7071}, {0.7071, -0.7071}, {-0.7071, -0.7071}}

func dot2(seed uint64, ix, iy int32, dx, dy float64) float64 {
	g := grad2[Hash2(seed, ix, iy)&7]
	return g[0]*dx + g[1]*dy
}

// Noise2 is 2D gradient (Perlin) noise in roughly [-1,1].
func Noise2(seed uint64, x, y float64) float64 {
	fx, fy := math.Floor(x), math.Floor(y)
	ix, iy := int32(fx), int32(fy)
	dx, dy := x-fx, y-fy
	n00 := dot2(seed, ix, iy, dx, dy)
	n10 := dot2(seed, ix+1, iy, dx-1, dy)
	n01 := dot2(seed, ix, iy+1, dx, dy-1)
	n11 := dot2(seed, ix+1, iy+1, dx-1, dy-1)
	u, v := fade(dx), fade(dy)
	return lerp(lerp(n00, n10, u), lerp(n01, n11, u), v) * 1.4142
}

// Value3 is smooth 3D value noise in [-1,1]; used for blobby 3D displacement.
func Value3(seed uint64, x, y, z float64) float64 {
	fx, fy, fz := math.Floor(x), math.Floor(y), math.Floor(z)
	ix, iy, iz := int32(fx), int32(fy), int32(fz)
	u, v, w := fade(x-fx), fade(y-fy), fade(z-fz)
	h := func(a, b, c int32) float64 { return float64(Hash3(seed, ix+a, iy+b, iz+c)>>11)/(1<<52) - 1 }
	return lerp(
		lerp(lerp(h(0, 0, 0), h(1, 0, 0), u), lerp(h(0, 1, 0), h(1, 1, 0), u), v),
		lerp(lerp(h(0, 0, 1), h(1, 0, 1), u), lerp(h(0, 1, 1), h(1, 1, 1), u), v), w)
}

// FBM sums octaves of Noise2.
func FBM(seed uint64, x, y float64, octaves int, lacunarity, gain float64) float64 {
	amp, freq, sum, norm := 1.0, 1.0, 0.0, 0.0
	for i := 0; i < octaves; i++ {
		sum += amp * Noise2(seed+uint64(i)*0x9E37, x*freq, y*freq)
		norm += amp
		amp *= gain
		freq *= lacunarity
	}
	return sum / norm
}

// Ridged returns ridged multifractal noise in [0,1]; sharp crests suit mountains.
func Ridged(seed uint64, x, y float64, octaves int) float64 {
	amp, freq, sum, norm, prev := 1.0, 1.0, 0.0, 0.0, 1.0
	for i := 0; i < octaves; i++ {
		n := 1 - math.Abs(Noise2(seed+uint64(i)*0x51ED, x*freq, y*freq))
		n *= n
		sum += n * amp * prev
		norm += amp
		prev = math.Min(1, n*1.6)
		amp *= 0.5
		freq *= 2.03
	}
	return sum / norm
}

// SmoothStep is the classic Hermite interpolation between edges a and b.
func SmoothStep(a, b, x float64) float64 {
	if a == b {
		if x < a {
			return 0
		}
		return 1
	}
	t := math.Min(1, math.Max(0, (x-a)/(b-a)))
	return t * t * (3 - 2*t)
}

// Clamp limits v to [lo,hi].
func Clamp(v, lo, hi float64) float64 { return math.Min(hi, math.Max(lo, v)) }

// Mix linearly interpolates between a and b.
func Mix(a, b, t float64) float64 { return a + (b-a)*t }
