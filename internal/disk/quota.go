// Package disk implements conservative in-process admission for a single worker owner.
// Filesystem usage must include partial files and pending upload backlog.
package disk

import (
 "errors"
 "sync"
)

var ErrCapacity = errors.New("insufficient staging capacity")
var ErrSize = errors.New("unknown or invalid file size")

type Usage struct { Staged, Free int64 }
type Measure func() (Usage,error)

type Manager struct {
 mu sync.Mutex
 quota,reserve,promised int64
 measure Measure
}

func New(quota,reserve int64, measure Measure) (*Manager,error) {
 if quota <= 0 || reserve < 0 || measure == nil { return nil, errors.New("invalid quota configuration") }
 return &Manager{quota:quota,reserve:reserve,measure:measure},nil
}

// Available intentionally double-counts bytes already written by active reservations.
// This sacrifices utilization rather than allowing concurrent jobs to overcommit.
func (m *Manager) availableLocked() (int64,error) {
 u,err:=m.measure()
 if err!=nil {return 0,err}
 if u.Staged<0 || u.Free<0 {return 0,errors.New("invalid disk measurement")}
 q:=max(int64(0),m.quota-u.Staged)
 f:=max(int64(0),u.Free-m.reserve)
 return max(int64(0),min(q,f)-m.promised),nil
}
func(m *Manager) Available()(int64,error){
 m.mu.Lock();defer m.mu.Unlock()
 return m.availableLocked()
}

// Reserve returns an idempotent release function. Release only when the writer
// has stopped and all its staged bytes are visible to Measure.
func(m *Manager) Reserve(size int64)(func(),error){
 if size<=0 {return nil,ErrSize}
 m.mu.Lock()
 defer m.mu.Unlock()
 available,err:=m.availableLocked()
 if err!=nil {return nil,err}
 if size>available{return nil,ErrCapacity}
 m.promised+=size
 var once sync.Once
 return func(){once.Do(func(){m.mu.Lock();m.promised-=size;m.mu.Unlock()})},nil
}
