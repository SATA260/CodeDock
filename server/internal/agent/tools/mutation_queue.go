package tools

import (
	"path/filepath"
	"sync"
)

type mutationLock struct {
	mutex sync.Mutex
	uses  int
}

var mutationLocks = struct {
	sync.Mutex
	byPath map[string]*mutationLock
}{
	byPath: make(map[string]*mutationLock),
}

func (executor *Executor) withFileMutation(path string, mutate func() (ToolResult, error)) (ToolResult, error) {
	key, err := filepath.Abs(path)
	if err != nil {
		return ToolResult{}, err
	}
	if canonical, canonicalErr := executor.FS.EvalSymlinks(key); canonicalErr == nil {
		key = canonical
	}

	mutationLocks.Lock()
	lock := mutationLocks.byPath[key]
	if lock == nil {
		lock = &mutationLock{}
		mutationLocks.byPath[key] = lock
	}
	lock.uses++
	mutationLocks.Unlock()

	lock.mutex.Lock()
	defer func() {
		lock.mutex.Unlock()
		mutationLocks.Lock()
		lock.uses--
		if lock.uses == 0 {
			delete(mutationLocks.byPath, key)
		}
		mutationLocks.Unlock()
	}()

	return mutate()
}
