package jobprinter

import (
	"fmt"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func New() *JobPrinter {
	return &JobPrinter{
		events: make(chan Job, 100),
		start:  time.Now(),
	}
}

type JobPrinter struct {
	ready   bool
	width   int
	program *tea.Program
	jobs    []Job
	start   time.Time
	events  chan Job
	wg      sync.WaitGroup
	stopped time.Time
}

func (jp *JobPrinter) Start() error {
	jp.program = tea.NewProgram(jp)
	jp.wg.Add(1)
	defer jp.wg.Done()
	return jp.program.Start()
}
func (jp *JobPrinter) Stop()           { jp.stopped = time.Now(); jp.wg.Wait() }
func (jp *JobPrinter) EndJob(job *Job) { job.end = time.Now(); jp.events <- *job }
func (jp *JobPrinter) StartJob(name string) *Job {
	job := newJob(name)
	jp.events <- job
	return &job
}

func newJob(name string) Job {
	return Job{name: name, start: time.Now()}
}

type Job struct {
	name      string
	start     time.Time
	end       time.Time
	replaceTS string
	// TODO: Color or group of something?
}

func (j *Job) ReplaceTS(s string) { j.replaceTS = s }

type tickMsg time.Time

func (jp *JobPrinter) Init() tea.Cmd { return jp.tickCmd() }

func (jp *JobPrinter) tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (jp *JobPrinter) emptyEvents() (jobs []Job) {
	for {
		select {
		case j := <-jp.events:
			jobs = append(jobs, j)
		default:
			// Channel is empty
			return jobs
		}
	}
}

func (jp *JobPrinter) processEvent(job Job) {
	if job.end.IsZero() {
		jp.jobs = append(jp.jobs, job)
		return
	}
	for i, j := range jp.jobs {
		if j.name == job.name && j.start.Equal(j.start) {
			jp.jobs[i] = job
		}
	}
}

func (jp *JobPrinter) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return jp, nil
	case tickMsg:
		jobs := jp.emptyEvents()
		for _, job := range jobs {
			jp.processEvent(job)
		}
		if !jp.stopped.IsZero() {
			return jp, tea.Quit
		}
		return jp, jp.tickCmd()
	case tea.WindowSizeMsg:
		if !jp.ready {
			jp.ready = true
		}

		jp.width = msg.Width
		return jp, nil
	}
	panic("unreachable")
}

var appStyle = lipgloss.NewStyle().Margin(0, 0, 0, 0)

func (jp *JobPrinter) timeString(job Job) string {
	if job.replaceTS != "" {
		return job.replaceTS
	}
	var ts float64
	if job.end.IsZero() {
		ts = time.Since(job.start).Seconds()
	} else {
		ts = job.end.Sub(job.start).Seconds()
	}
	return fmt.Sprintf("%.1fs", ts)
}

func (jp *JobPrinter) View() string {
	if !jp.ready {
		return "\n  Initializing..."
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Building %.1fs\n", time.Since(jp.start).Seconds())
	w := lipgloss.Width

	for _, job := range jp.jobs {
		name := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Render(job.name)
		time := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Render(jp.timeString(job))
		paddingWidth := jp.width - w(name) - w(time)
		// TODO: ensure time is shown when name is longer than window width-time width
		padding := lipgloss.NewStyle().
			Width(paddingWidth).
			Foreground(lipgloss.Color("241")).
			Render(strings.Repeat(".", floor(paddingWidth)))

		sb.WriteString(lipgloss.NewStyle().Render(name + padding + time + "\n"))
	}
	if !jp.stopped.IsZero() {
		fmt.Fprintf(&sb, "All jobs complete in %.2fs\n\n", jp.stopped.Sub(jp.start).Seconds())
	}
	return appStyle.MaxWidth(jp.width).Foreground(lipgloss.Color("#ffffff")).Render(sb.String())
}

func floor(i int) int {
	if i < 0 {
		return 0
	}
	return i
}
