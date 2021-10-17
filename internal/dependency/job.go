package dependency

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

type Job struct {
	ID        string
	Start     time.Time
	End       time.Time
	Error     string
	Module    string
	Reference string
}

type JobRequest struct {
	// The location of the version control repository.
	Module string
	// Reference is a version control reference. With Git this could be a
	// branch, tag, or commit. This value is optional.
	Reference string
}

type jobQueue struct {
	jobs map[string]*Job
	lock sync.Mutex
}

func (jq *jobQueue) AddJob(job *Job) {
	jq.lock.Lock()
	defer jq.lock.Unlock()

	for {
		// unique id
		job.ID = fmt.Sprint(rand.Int())
		if _, found := jq.jobs[job.ID]; !found {
			break
		}
	}
	job.Start = time.Now()
	// Remove stale jobs from in-memory
	if len(jq.jobs) > 100 {
		jq.kickOldest()
	}
	jq.jobs[job.ID] = job
}

func (jq *jobQueue) End(id string, err error) {
	jq.lock.Lock()
	defer jq.lock.Unlock()
	job := jq.jobs[id]
	if err != nil {
		job.Error = err.Error()
	}
	job.End = time.Now()
}

func (jq *jobQueue) Lookup(id string) *Job {
	jq.lock.Lock()
	defer jq.lock.Unlock()
	job := jq.jobs[id]
	if job != nil {
		// make copy, a read only record
		v := *job
		return &v
	}
	return nil
}

func (jq *jobQueue) kickOldest() {
	oldest := time.Now()
	id := ""
	for i, j := range jq.jobs {
		if j.End.IsZero() {
			// Skip running jobs
			continue
		}
		if j.Start.Before(oldest) {
			oldest = j.Start
			id = i
		}
	}
	delete(jq.jobs, id)
}

var jq = &jobQueue{jobs: map[string]*Job{}}
