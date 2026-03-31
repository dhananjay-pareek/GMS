package jsonrepo

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/gosom/google-maps-scraper/web"
)

type repo struct {
	mu   sync.RWMutex
	path string
	jobs map[string]web.Job
}

func New(path string) (web.JobRepository, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}

	r := &repo{
		path: path,
		jobs: make(map[string]web.Job),
	}

	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	if len(b) > 0 {
		var jlist []web.Job
		if err := json.Unmarshal(b, &jlist); err != nil {
			return nil, err
		}
		for _, j := range jlist {
			r.jobs[j.ID] = j
		}
	}

	return r, nil
}

func (r *repo) save() error {
	var jlist []web.Job
	for _, j := range r.jobs {
		jlist = append(jlist, j)
	}

	// sort so the output is consistent
	sort.Slice(jlist, func(i, j int) bool {
		return jlist[i].Date.After(jlist[j].Date)
	})

	b, err := json.MarshalIndent(jlist, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(r.path, b, 0644)
}

func (r *repo) Get(ctx context.Context, id string) (web.Job, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	j, ok := r.jobs[id]
	if !ok {
		return web.Job{}, errors.New("job not found")
	}

	return j, nil
}

func (r *repo) Create(ctx context.Context, job *web.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.jobs[job.ID]; ok {
		return errors.New("job already exists")
	}

	r.jobs[job.ID] = *job

	return r.save()
}

func (r *repo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.jobs, id)

	return r.save()
}

func (r *repo) Select(ctx context.Context, params web.SelectParams) ([]web.Job, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var ans []web.Job
	for _, j := range r.jobs {
		if params.Status != "" && j.Status != params.Status {
			continue
		}
		ans = append(ans, j)
	}

	sort.Slice(ans, func(i, j int) bool {
		return ans[i].Date.After(ans[j].Date)
	})

	if params.Limit > 0 && len(ans) > params.Limit {
		ans = ans[:params.Limit]
	}

	return ans, nil
}

func (r *repo) Update(ctx context.Context, job *web.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.jobs[job.ID]; !ok {
		return errors.New("job not found")
	}

	r.jobs[job.ID] = *job

	return r.save()
}
