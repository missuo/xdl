package gui

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/missuo/xdl/internal/engine"
)

var jobColumns = []ui.TableColumn{
	{Title: "Account"},
	{Title: "Mode", Width: 90},
	{Title: "From", Width: 80},
	{Title: "State", Width: 110},
	{Title: "Progress", Width: 260},
	{Title: "Added", Width: 90},
}

var modeNames = map[engine.Mode]string{
	engine.ModeDownload: "Download",
	engine.ModeFetch:    "Fetch",
	engine.ModeLibrary:  "From library",
}

func (a *App) queuePage(c *ui.Context) {
	t := c.Theme()
	finished := 0
	for _, j := range a.jobs {
		if j.State.Finished() {
			finished++
		}
	}
	pageHeader(c, "Queue", "Jobs from the app, the browser extension and the CLI run here one at a time.", func() {
		if ui.Button(c, "Clear Finished").Disabled(finished == 0).Clicked() {
			a.eng.ClearFinished()
		}
	})
	jobs := a.jobs
	if len(jobs) == 0 {
		ui.Column(c).Grow(1).Center().Gap(6).Children(func() {
			ui.Icon(c, iconQueue).FontSize(36).TextColor(t.TextMuted.Alpha(0.5))
			ui.Text(c, "The queue is empty").FontSize(14).Bold().TextColor(t.TextMuted)
			ui.Text(c, "Click the XDL button on an X profile in Chrome, or run `xdl send <user>`.").
				FontSize(12).TextColor(t.TextMuted)
		})
		return
	}
	a.jobsList.Selected = &a.jobRow
	a.jobsList.Key = func(i int) any { return jobs[i].ID }
	table := ui.Table(c, &a.jobsList, jobColumns, len(jobs), func(row, col int) {
		j := jobs[row]
		switch col {
		case 0:
			ui.Text(c, j.Label()).FontSize(12.5).Bold().SingleLine()
		case 1:
			ui.Text(c, modeNames[j.Mode]).FontSize(12)
		case 2:
			ui.Text(c, sourceNames[j.Source]).FontSize(12).TextColor(t.TextMuted)
		case 3:
			stateBadge(c, j.State)
		case 4:
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				switch j.State {
				case engine.StateDownloading:
					ui.Progress(c, float64(j.Percent)/100).Width(80).Label("Progress")
					ui.Textf(c, "%d/%d", j.Current, j.Total).FontSize(12).SingleLine()
				case engine.StateFetching:
					ui.Progress(c, -1).Width(80).Label("Fetching")
					ui.Textf(c, "page %d · %d items", j.Pages, j.Fetched).FontSize(12).SingleLine()
				default:
					text := j.Summary()
					col := t.TextMuted
					if j.State == engine.StateFailed {
						text, col = j.Error, t.Danger
					}
					ui.Text(c, text).FontSize(12).TextColor(col).SingleLine().Tooltip(text)
				}
			})
		case 5:
			ui.Text(c, ago(j.CreatedAt)).FontSize(12).TextColor(t.TextMuted)
		}
	}).Grow(1).Basis(0)
	if table.Submitted() && a.jobRow >= 0 && a.jobRow < len(jobs) {
		a.showJob(jobs[a.jobRow])
	}
	table.ContextMenu(func(m *ui.Menu) {
		if a.jobRow < 0 || a.jobRow >= len(jobs) {
			return
		}
		j := jobs[a.jobRow]
		if m.Item("Show Details").Chosen() {
			a.showJob(j)
		}
		if m.Item("Open Folder").Disabled(j.OutputDir == "").Chosen() {
			a.openPath(a.jobFolder(j))
		}
		m.Separator()
		if m.Item("Cancel").Disabled(j.State.Finished()).Chosen() {
			a.eng.Cancel(j.ID)
		}
		if m.Item("Retry").Disabled(!j.State.Finished()).Chosen() {
			a.submit(engine.Request{Username: j.Username, TimelineType: j.TimelineType, Mode: j.Mode})
		}
		if m.Item("Remove").Disabled(!j.State.Finished()).Chosen() {
			a.eng.Remove(j.ID)
		}
	})
	ui.Text(c, "Double-click a job to see its items. Right-click for more.").FontSize(11.5).TextColor(t.TextMuted)
}

func (a *App) showJob(j engine.Job) {
	a.focusJob = j.ID
	a.items, a.itemsJob = nil, ""
	a.loadItems(j.ID)
	a.page = pageDownload
}

func ago(at time.Time) string {
	d := time.Since(at)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return at.Format("15:04")
	}
	return at.Format("Jan 2")
}
