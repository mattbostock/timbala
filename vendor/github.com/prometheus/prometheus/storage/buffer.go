package storage

import "math"

// BufferedSeriesIterator wraps an iterator with a look-back buffer.
type BufferedSeriesIterator struct {
	it  SeriesIterator
	buf *sampleRing

	lastTime int64
}

// NewBuffer returns a new iterator that buffers the values within the time range
// of the current element and the duration of delta before.
func NewBuffer(it SeriesIterator, delta int64) *BufferedSeriesIterator {
	bit := &BufferedSeriesIterator{
		it:       it,
		buf:      newSampleRing(delta, 16),
		lastTime: math.MinInt64,
	}
	it.Next()

	return bit
}

// PeekBack returns the previous element of the iterator. If there is none buffered,
// ok is false.
func (b *BufferedSeriesIterator) PeekBack() (t int64, v float64, ok bool) {
	return b.buf.last()
}

// Buffer returns an iterator over the buffered data.
func (b *BufferedSeriesIterator) Buffer() SeriesIterator {
	return b.buf.iterator()
}

// Seek advances the iterator to the element at time t or greater.
func (b *BufferedSeriesIterator) Seek(t int64) bool {
	t0 := t - b.buf.delta

	// If the delta would cause us to seek backwards, preserve the buffer
	// and just continue regular advancment while filling the buffer on the way.
	if t0 > b.lastTime {
		b.buf.reset()

		ok := b.it.Seek(t0)
		if !ok {
			return false
		}
		b.lastTime, _ = b.Values()
	}

	if b.lastTime >= t {
		return true
	}
	for b.Next() {
		if b.lastTime >= t {
			return true
		}
	}

	return false
}

// Next advances the iterator to the next element.
func (b *BufferedSeriesIterator) Next() bool {
	// Add current element to buffer before advancing.
	b.buf.add(b.it.At())

	ok := b.it.Next()
	if ok {
		b.lastTime, _ = b.Values()
	}
	return ok
}

// Values returns the current element of the iterator.
func (b *BufferedSeriesIterator) Values() (int64, float64) {
	return b.it.At()
}

// Err returns the last encountered error.
func (b *BufferedSeriesIterator) Err() error {
	return b.it.Err()
}

type sample struct {
	t int64
	v float64
}

type sampleRing struct {
	delta int64

	buf []sample // lookback buffer
	i   int      // position of most recent element in ring buffer
	f   int      // position of first element in ring buffer
	l   int      // number of elements in buffer
}

func newSampleRing(delta int64, sz int) *sampleRing {
	r := &sampleRing{delta: delta, buf: make([]sample, sz)}
	r.reset()

	return r
}

func (r *sampleRing) reset() {
	r.l = 0
	r.i = -1
	r.f = 0
}

func (r *sampleRing) iterator() SeriesIterator {
	return &sampleRingIterator{r: r, i: -1}
}

type sampleRingIterator struct {
	r *sampleRing
	i int
}

func (it *sampleRingIterator) Next() bool {
	it.i++
	return it.i < it.r.l
}

func (it *sampleRingIterator) Seek(int64) bool {
	return false
}

func (it *sampleRingIterator) Err() error {
	return nil
}

func (it *sampleRingIterator) At() (int64, float64) {
	return it.r.at(it.i)
}

func (r *sampleRing) at(i int) (int64, float64) {
	j := (r.f + i) % len(r.buf)
	s := r.buf[j]
	return s.t, s.v
}

// add adds a sample to the ring buffer and frees all samples that fall
// out of the delta range.
func (r *sampleRing) add(t int64, v float64) {
	l := len(r.buf)
	// Grow the ring buffer if it fits no more elements.
	if l == r.l {
		buf := make([]sample, 2*l)
		copy(buf[l+r.f:], r.buf[r.f:])
		copy(buf, r.buf[:r.f])

		r.buf = buf
		r.i = r.f
		r.f += l
	} else {
		r.i++
		if r.i >= l {
			r.i -= l
		}
	}

	r.buf[r.i] = sample{t: t, v: v}
	r.l++

	// Free head of the buffer of samples that just fell out of the range.
	for r.buf[r.f].t < t-r.delta {
		r.f++
		if r.f >= l {
			r.f -= l
		}
		r.l--
	}
}

// last returns the most recent element added to the ring.
func (r *sampleRing) last() (int64, float64, bool) {
	if r.l == 0 {
		return 0, 0, false
	}
	s := r.buf[r.i]
	return s.t, s.v, true
}

func (r *sampleRing) samples() []sample {
	res := make([]sample, r.l)

	var k = r.f + r.l
	var j int
	if k > len(r.buf) {
		k = len(r.buf)
		j = r.l - k + r.f
	}

	n := copy(res, r.buf[r.f:k])
	copy(res[n:], r.buf[:j])

	return res
}
