package puctrl

import "sync"

type SafeMap[K comparable, T any] struct {
	mutex sync.RWMutex
	store map[K]T
}

// Create a new safe map
func NewSafeMap[K comparable, T any]() *SafeMap[K, T] {
	return &SafeMap[K, T]{
		store: make(map[K]T),
	}
}

// Store key and value into map
func (m *SafeMap[K, T]) Set(key K, value T) {
	m.mutex.Lock()
	m.store[key] = value
	m.mutex.Unlock()
}

// Returns value for given key
func (m *SafeMap[K, T]) Get(key K) (T, bool) {
	m.mutex.RLock()
	value, exists := m.store[key]
	m.mutex.RUnlock()
	return value, exists
}

// Delete an elements with key
func (m *SafeMap[K, T]) Delete(key K) {
	m.mutex.Lock()
	delete(m.store, key)
	m.mutex.Unlock()
}
