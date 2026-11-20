package positions

import (
	"bytes"

	"go.opentelemetry.io/otel/trace"

	"github.com/staff768/avito_tripgo_staff768/internal/model"
)

type task struct {
	trip  model.StaleTrip
	scan  trace.SpanContext
	index int
}

type taskHeap []*task

func (q taskHeap) Len() int { return len(q) }

func (q taskHeap) Less(i, j int) bool {
	left, right := q[i].trip, q[j].trip
	if order := left.LastActivityAt.Compare(right.LastActivityAt); order != 0 {
		return order < 0
	}
	return bytes.Compare(left.ID[:], right.ID[:]) < 0
}

func (q taskHeap) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
	q[i].index, q[j].index = i, j
}

func (q *taskHeap) Push(value any) {
	job := value.(*task)
	job.index = len(*q)
	*q = append(*q, job)
}

func (q *taskHeap) Pop() any {
	last := len(*q) - 1
	job := (*q)[last]
	(*q)[last] = nil
	*q = (*q)[:last]
	job.index = -1
	return job
}

func (q taskHeap) freshest() *task {
	index := 0
	for i := 1; i < len(q); i++ {
		if q.Less(index, i) {
			index = i
		}
	}
	return q[index]
}
