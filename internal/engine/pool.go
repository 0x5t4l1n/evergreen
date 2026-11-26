package engine

// Pool is a simple free-list that recycles values instead of allocating.
type Pool[T any] struct {
	free  []*T
	newFn func() *T
	reset func(*T)
	live  int
}

// NewPool creates a pool; reset may be nil.
func NewPool[T any](newFn func() *T, reset func(*T)) *Pool[T] {
	return &Pool[T]{newFn: newFn, reset: reset}
}

// Get returns a recycled or new value.
func (p *Pool[T]) Get() *T {
	p.live++
	if n := len(p.free); n > 0 {
		v := p.free[n-1]
		p.free = p.free[:n-1]
		return v
	}
	return p.newFn()
}

// Put returns v to the pool.
func (p *Pool[T]) Put(v *T) {
	if v == nil {
		return
	}
	if p.reset != nil {
		p.reset(v)
	}
	p.live--
	if len(p.free) < 4096 {
		p.free = append(p.free, v)
	}
}

// Live is the number of values currently checked out.
func (p *Pool[T]) Live() int { return p.live }
