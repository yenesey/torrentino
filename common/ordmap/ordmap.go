package ordmap

import "iter"

type OrderedMap[T comparable, V any] struct {
	keys  []T
	store map[T]V
}

func New[T comparable, V any]() *OrderedMap[T, V] {
	return &OrderedMap[T, V]{
		keys:  make([]T, 0),
		store: make(map[T]V),
	}
}

func (om *OrderedMap[T, V]) Set(key T, value V) {
	if _, ok := om.store[key]; !ok {
		om.keys = append(om.keys, key)
	}
	om.store[key] = value
}

func (om *OrderedMap[T, V]) Get(key T) (V, bool) {
	val, ok := om.store[key]
	return val, ok
}

func (om *OrderedMap[T, V]) GetUnsafe(key T) V {
	return om.store[key]
}

func (om *OrderedMap[T, V]) Len() int {
	if om != nil {
		return len(om.keys)
	}
	return 0
}

func (om *OrderedMap[T, V]) GetByIndex(i int) (V, bool) {
	val, ok := om.store[om.keys[i]]
	return val, ok
}

func (om *OrderedMap[T, V]) GetByIndexUnsafe(i int) V { // unsafe!
	return om.store[om.keys[i]]
}

func (om *OrderedMap[T, V]) IterKeys() iter.Seq2[int, T] {
	return func(yield func(int, T) bool) {
		if om != nil {
			for i, v := range om.keys {
				if !yield(i, v) {
					return
				}
			}
		}
	}
}

func (om *OrderedMap[T, V]) Iter() iter.Seq2[T, V] {
	return func(yield func(T, V) bool) {
		if om != nil {
			for _, k := range om.keys {
				if !yield(k, om.store[k]) {
					return
				}
			}
		}
	}
}
