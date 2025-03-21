package log

import (
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/log"
)

func TestCQueue(t *testing.T) {
	var r Record
	r.SetBody(log.BoolValue(true))

	t.Run("newQueue", func(t *testing.T) {
		const size = 1
		q := newCQueue(size)
		assert.Equal(t, 0, q.Len())
		assert.Equal(t, size, q.Cap(), "capacity")
		// assert.Equal(t, size, q.read.Len(), "read ring")
		// assert.Same(t, q.read, q.write, "different rings")
	})

	t.Run("Enqueue", func(t *testing.T) {
		const size = 2
		q := newCQueue(size)

		var notR Record
		notR.SetBody(log.IntValue(10))

		assert.Equal(t, 1, q.Enqueue(&notR), "incomplete batch")
		assert.Equal(t, 1, q.Len(), "length")
		assert.Equal(t, size, q.cap, "capacity")

		assert.Equal(t, 2, q.Enqueue(&r), "complete batch")
		assert.Equal(t, 2, q.Len(), "length")
		assert.Equal(t, size, q.cap, "capacity")

		assert.Equal(t, 2, q.Enqueue(&r), "overflow batch")
		assert.Equal(t, 2, q.Len(), "length")
		assert.Equal(t, size, q.Cap(), "capacity")

		assert.Equal(t, []*Record{&r, &r}, q.Flush(), "flushed Records")
	})

	t.Run("Dropped", func(t *testing.T) {
		q := newCQueue(1)

		_ = q.Enqueue(&r)
		_ = q.Enqueue(&r)
		assert.Equal(t, uint64(1), q.Dropped(), "fist")

		_ = q.Enqueue(&r)
		_ = q.Enqueue(&r)
		assert.Equal(t, uint64(2), q.Dropped(), "second")
	})

	t.Run("Flush", func(t *testing.T) {
		const size = 2
		q := newCQueue(size)
		q.Enqueue(&r)

		assert.Equal(t, []*Record{&r}, q.Flush(), "flushed")
	})

	t.Run("TryFlush", func(t *testing.T) {
		const size = 3
		q := newCQueue(size)
		for i := 0; i < size-1; i++ {
			q.Enqueue(&r)
		}

		buf := make([]*Record, 1)
		f := func([]*Record) bool { return false }
		assert.Equal(t, size-1, q.TryDequeue(buf, f), "not flushed")
		require.Equal(t, size-1, q.Len(), "length")
		// require.NotSame(t, q.read, q.write, "read ring advanced")

		var flushed []*Record
		f = func(r []*Record) bool {
			flushed = append(flushed, r...)
			return true
		}
		if assert.Equal(t, size-2, q.TryDequeue(buf, f), "did not flush len(buf)") {
			assert.Equal(t, []*Record{&r}, flushed, "Records")
		}

		buf = slices.Grow(buf, size)
		flushed = flushed[:0]
		if assert.Equal(t, 0, q.TryDequeue(buf, f), "did not flush len(queue)") {
			assert.Equal(t, []*Record{&r}, flushed, "Records")
		}
	})

	t.Run("ConcurrentSafe", func(t *testing.T) {
		const goRoutines = 10

		flushed := make(chan []*Record, goRoutines)
		out := make([]*Record, 0, goRoutines)
		done := make(chan struct{})
		go func() {
			defer close(done)
			for recs := range flushed {
				out = append(out, recs...)
			}
		}()

		var wg sync.WaitGroup
		wg.Add(goRoutines)

		b := newCQueue(goRoutines)
		for i := 0; i < goRoutines; i++ {
			go func() {
				defer wg.Done()
				b.Enqueue(&Record{})
				flushed <- b.Flush()
			}()
		}

		wg.Wait()
		close(flushed)
		<-done

		assert.Len(t, out, goRoutines, "flushed Records")
	})
}
