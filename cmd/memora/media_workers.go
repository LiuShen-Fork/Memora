package main

import (
	"errors"
	"fmt"
	"time"
)

// How many tasks the queue runs at once. The setting is the queue's own, so an
// administrator can change it while the process is running; the ceiling keeps a
// SQLite-backed queue from opening more writers than it can feed, and the floor
// keeps at least one task moving.
const (
	defaultQueueWorkers = 5
	minQueueWorkers     = 1
	maxQueueWorkers     = 8
)

// queueDrainTimeout is how long a shutdown waits for the workers to put down
// what they are holding. A task that is still running afterwards is left for
// the next start, which resets in-progress rows to pending.
const queueDrainTimeout = 30 * time.Second

// startWorkers brings the pool up at the concurrency this process should run
// with, and clears the tasks a previous process left in progress.
func (a *App) startWorkers() {
	_, _ = a.db.Exec(`UPDATE pipeline_queue SET status='pending',status_stage=NULL WHERE status='in-stages'`)
	a.workersMu.Lock()
	defer a.workersMu.Unlock()
	a.workersStarted = true
	a.resizeWorkers(a.initialQueueWorkers())
}

// resizeQueue brings the running pool in line with the setting. A process that
// never started a pool — a test, or a settings write before the queue came up —
// has nothing to resize, and the next start reads the setting on its own.
func (a *App) resizeQueue() {
	a.workersMu.Lock()
	defer a.workersMu.Unlock()
	if !a.workersStarted {
		return
	}
	a.resizeWorkers(a.storedQueueWorkers())
}

// resizeWorkers grows or shrinks the pool to n workers.
//
// A worker that is asked to leave finishes the task it is holding before it
// goes, so shrinking the pool drains work instead of dropping it. Growing it
// takes effect immediately, and the new workers are woken rather than waiting
// out their idle poll.
func (a *App) resizeWorkers(n int) {
	n = clampQueueWorkers(n)
	for len(a.workerRetire) > n {
		close(a.workerRetire[0])
		a.workerRetire = a.workerRetire[1:]
	}
	for len(a.workerRetire) < n {
		retire := make(chan struct{})
		a.workerRetire = append(a.workerRetire, retire)
		a.workerSeq++
		a.wg.Add(1)
		go a.worker(a.workerSeq, retire)
	}
	a.wakeQueue()
}

// workerCount is how many workers the pool runs right now, which is not
// necessarily what the setting says: an operator's environment can start the
// process at a different number.
func (a *App) workerCount() int {
	a.workersMu.Lock()
	defer a.workersMu.Unlock()
	return len(a.workerRetire)
}

// stopWorkers asks every worker to leave and gives them queueDrainTimeout to
// finish. Whatever is still running after that is abandoned deliberately: a
// media task can take minutes, and a shutdown that never completes is worse for
// the operator than a task that is processed again on the next start.
func (a *App) stopWorkers() {
	close(a.stop)
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(queueDrainTimeout):
		a.logs.Add("queue", fmt.Sprintf("workers still busy after %s, leaving the rest of the queue for the next start", queueDrainTimeout))
	}
}

func (a *App) worker(id int, retire <-chan struct{}) {
	defer a.wg.Done()
	for {
		select {
		case <-a.stop:
			return
		case <-retire:
			return
		default:
		}
		task, err := a.claimTask()
		if err != nil {
			a.logs.Add("queue", fmt.Sprintf("worker %d: %v", id, err))
			time.Sleep(time.Second)
			continue
		}
		if task == nil {
			select {
			case <-a.stop:
				return
			case <-retire:
				return
			case <-a.queueWake:
			case <-time.After(2 * time.Second):
			}
			continue
		}
		if err := a.processTask(task); err != nil {
			// A provider-confirmed missing object cannot become available by
			// retrying. Keep the task visible as failed, but avoid hammering
			// remote storage three times for the same deleted file.
			if errors.Is(err, ErrStorageNotFound) {
				a.failTaskPermanent(task.ID, err.Error())
			} else {
				a.failTask(task.ID, err.Error())
			}
		} else {
			a.completeTask(task.ID)
		}
	}
}

// initialQueueWorkers is the concurrency the process starts with. An operator
// who set CFRAME_WORKERS gets what they asked for; otherwise the setting the
// dashboard writes decides.
func (a *App) initialQueueWorkers() int {
	if a.cfg.WorkerCount > 0 {
		return clampQueueWorkers(a.cfg.WorkerCount)
	}
	return a.storedQueueWorkers()
}

// storedQueueWorkers reads the concurrency the dashboard asked for. A value
// outside the range is clamped rather than refused, so a database edited by
// hand still starts a working queue.
func (a *App) storedQueueWorkers() int {
	var value any
	if a.readSetting("system", "queueConcurrency", &value) {
		if size, ok := value.(float64); ok {
			return clampQueueWorkers(int(size))
		}
	}
	return defaultQueueWorkers
}

func clampQueueWorkers(n int) int {
	if n < minQueueWorkers {
		return minQueueWorkers
	}
	if n > maxQueueWorkers {
		return maxQueueWorkers
	}
	return n
}
