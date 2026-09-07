package codex

import (
	"sync"

	pkg "codedock/pkg/codex"
)

const ringSize = 256

type eventRing struct {
	mu     sync.Mutex
	seq    int64
	items  []pkg.Event
	oldest int64
}

func newRing() *eventRing {
	return &eventRing{oldest: 1}
}

func (r *eventRing) append(ev pkg.Event) pkg.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	ev.Seq = r.seq
	r.items = append(r.items, ev)
	if len(r.items) > ringSize {
		r.items = r.items[len(r.items)-ringSize:]
		r.oldest = r.items[0].Seq
	}
	return ev
}

func (r *eventRing) after(seq int64) (events []pkg.Event, reset bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if seq < 0 {
		seq = 0
	}
	if len(r.items) == 0 {
		return nil, false
	}
	if seq+1 < r.oldest {
		return append([]pkg.Event(nil), r.items...), true
	}
	out := make([]pkg.Event, 0, len(r.items))
	for _, ev := range r.items {
		if ev.Seq > seq {
			out = append(out, ev)
		}
	}
	return out, false
}

func (r *eventRing) last() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.seq
}
