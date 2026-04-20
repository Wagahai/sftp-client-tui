package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
)

type TransferJob struct {
	ID        string
	Filename  string
	Direction string // "upload" | "download"
	Sent      int64
	Total     int64
	Done      bool
	Err       error
	bar       progress.Model
}

type ProgressModel struct {
	jobs  map[string]*TransferJob
	order []string
	width int
}

func NewProgressModel() ProgressModel {
	return ProgressModel{
		jobs: make(map[string]*TransferJob),
	}
}

func (m *ProgressModel) SetWidth(w int) {
	m.width = w
}

func (m *ProgressModel) AddJob(id, filename, direction string, total int64) {
	bar := progress.New(progress.WithDefaultGradient())
	bar.Width = m.width - 30
	if bar.Width < 10 {
		bar.Width = 10
	}
	m.jobs[id] = &TransferJob{
		ID:        id,
		Filename:  filename,
		Direction: direction,
		Total:     total,
		bar:       bar,
	}
	m.order = append(m.order, id)
}

func (m *ProgressModel) UpdateJob(id string, sent, total int64, done bool, err error) {
	if j, ok := m.jobs[id]; ok {
		j.Sent = sent
		j.Total = total
		j.Done = done
		j.Err = err
	}
}

func (m *ProgressModel) RemoveDone() {
	newOrder := m.order[:0]
	for _, id := range m.order {
		if j, ok := m.jobs[id]; ok && !j.Done {
			newOrder = append(newOrder, id)
		} else {
			delete(m.jobs, id)
		}
	}
	m.order = newOrder
}

func (m ProgressModel) ActiveCount() int {
	return len(m.order)
}

func (m ProgressModel) Update(msg tea.Msg) (ProgressModel, tea.Cmd) {
	var cmds []tea.Cmd
	for _, j := range m.jobs {
		updated, cmd := j.bar.Update(msg)
		if bar, ok := updated.(progress.Model); ok {
			j.bar = bar
		}
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return m, tea.Batch(cmds...)
}

func (m ProgressModel) View() string {
	if len(m.order) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, StyleLabel.Render("Transfers:"))

	for _, id := range m.order {
		j := m.jobs[id]
		if j == nil {
			continue
		}

		pct := 0.0
		if j.Total > 0 {
			pct = float64(j.Sent) / float64(j.Total)
		}

		arrow := "↑"
		if j.Direction == "download" {
			arrow = "↓"
		}

		status := j.bar.ViewAs(pct)
		if j.Done && j.Err != nil {
			status = StyleError.Render("ERROR: " + j.Err.Error())
		} else if j.Done {
			status = StyleSuccess.Render("done")
		}

		sizeStr := ""
		if j.Total > 0 {
			sizeStr = fmt.Sprintf(" %s/%s", formatSize(j.Sent), formatSize(j.Total))
		}

		name := TruncateStr(j.Filename, 20)
		line := fmt.Sprintf("  %s %s%s %s", arrow, name, sizeStr, status)
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}
