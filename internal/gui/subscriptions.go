package gui

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/missuo/xdl/backend"
	"github.com/missuo/xdl/internal/engine"
	"github.com/missuo/xdl/internal/schedule"
)

type subscriptionForm struct {
	handle   string
	timeline string
	interval string
}

var intervalLabels = []string{"Every hour", "Every 6 hours", "Every 12 hours", "Every day", "Every week"}
var intervalValues = []time.Duration{time.Hour, 6 * time.Hour, 12 * time.Hour, 24 * time.Hour, 7 * 24 * time.Hour}

func intervalOf(label string) time.Duration {
	for i, l := range intervalLabels {
		if l == label {
			return intervalValues[i]
		}
	}
	return schedule.DefaultInterval
}

var subColumns = []ui.TableColumn{
	{Title: "Account"},
	{Title: "Every", Width: 100},
	{Title: "Last check", Width: 120},
	{Title: "Result"},
	{Title: "Next check", Width: 120},
}

func (a *App) loadSubscriptions() {
	subs, err := backend.ListSubscriptions()
	if err != nil {
		return
	}
	a.win.Update(func() {
		a.subs = subs
		if a.subRow >= len(subs) {
			a.subRow = -1
		}
	})
}

func (a *App) subscribe(handle, timeline string, every time.Duration) {
	go func() {
		sub, err := a.sched.Subscribe(handle, timeline, every)
		a.win.Update(func() {
			if err != nil {
				a.toast(err.Error())
				return
			}
			a.toast(fmt.Sprintf("Subscribed to @%s — checking now, then every %s", sub.Username, schedule.Describe(sub.IntervalMins)))
		})
		a.loadSubscriptions()
	}()
}

func (a *App) updateSubscription(id int64, every time.Duration, enabled bool) {
	go func() {
		if _, err := a.sched.Update(id, every, enabled); err != nil {
			a.win.Update(func() { a.toast(err.Error()) })
		}
		a.loadSubscriptions()
	}()
}

func (a *App) subscriptionsPage(c *ui.Context) {
	t := c.Theme()
	pageHeader(c, "Subscriptions", "XDL checks these accounts on a schedule and downloads what they post. A check missed while XDL was closed runs when it starts.", nil)

	card(c, "", func() {
		ui.Row(c).Gap(10).AlignItems(ui.End).Children(func() {
			ui.Column(c).Grow(1).Gap(5).Children(func() {
				ui.Text(c, "Account").FontSize(11.5).TextColor(t.TextMuted)
				known := make([]string, 0, len(a.accounts))
				for _, acc := range a.accounts {
					if acc.Username != "bookmarks" {
						known = append(known, "@"+acc.Username)
					}
				}
				if ui.Autocomplete(c, &a.subForm.handle, known).Placeholder("@username or profile URL").Label("Account").Submitted() {
					a.submitSubForm()
				}
			})
			ui.Column(c).Width(140).Gap(5).Children(func() {
				ui.Text(c, "Timeline").FontSize(11.5).TextColor(t.TextMuted)
				ui.Select(c, &a.subForm.timeline, timelineLabels).Label("Timeline")
			})
			ui.Column(c).Width(150).Gap(5).Children(func() {
				ui.Text(c, "Check").FontSize(11.5).TextColor(t.TextMuted)
				ui.Select(c, &a.subForm.interval, intervalLabels).Label("Check interval")
			})
			if ui.PrimaryButton(c, "Subscribe").Clicked() {
				a.submitSubForm()
			}
		})
	}).Shrink(0)

	subs := a.subs
	if len(subs) == 0 {
		ui.Column(c).Grow(1).Center().Gap(6).Children(func() {
			ui.Icon(c, iconSchedule).FontSize(36).TextColor(t.TextMuted.Alpha(0.5))
			ui.Text(c, "No subscriptions yet").FontSize(14).Bold().TextColor(t.TextMuted)
			ui.Text(c, "Subscribe to an account above, or run `xdl sub add <user> 1d`.").FontSize(12).TextColor(t.TextMuted)
		})
		return
	}
	a.subsList.Selected = &a.subRow
	a.subsList.Key = func(i int) any { return subs[i].ID }
	table := ui.Table(c, &a.subsList, subColumns, len(subs), func(row, col int) {
		s := subs[row]
		muted := t.TextMuted
		switch col {
		case 0:
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				label := engine.Job{Username: s.Username, TimelineType: s.TimelineType}.Label()
				ui.Text(c, label).FontSize(12.5).Bold().SingleLine()
				if !s.Enabled {
					ui.Badge(c, "Paused").Background(t.Warning.Alpha(0.15)).TextColor(t.Warning)
				}
			})
		case 1:
			ui.Text(c, intervalLabel(s.IntervalMins)).FontSize(12)
		case 2:
			last := "never"
			if s.LastRun != nil {
				last = ago(*s.LastRun)
			}
			ui.Text(c, last).FontSize(12).TextColor(muted)
		case 3:
			col := muted
			text := s.LastMessage
			switch s.LastStatus {
			case "running":
				text, col = "Checking…", t.Accent
			case string(engine.StateFailed):
				col = t.Danger
			case "":
				text = "—"
			}
			ui.Text(c, text).FontSize(12).TextColor(col).SingleLine().Tooltip(text)
		case 4:
			next := until(s.NextRun)
			if !s.Enabled {
				next = "paused"
			}
			ui.Text(c, next).FontSize(12).TextColor(muted)
		}
	}).Grow(1).Basis(0)

	var sel *backend.Subscription
	if a.subRow >= 0 && a.subRow < len(subs) {
		sel = &subs[a.subRow]
	}
	if table.Submitted() && sel != nil {
		a.checkNow(*sel)
	}
	table.ContextMenu(func(m *ui.Menu) {
		if sel == nil {
			return
		}
		s := *sel
		if m.Item("Check Now").Chosen() {
			a.checkNow(s)
		}
		if s.Enabled {
			if m.Item("Pause").Chosen() {
				a.updateSubscription(s.ID, 0, false)
			}
		} else if m.Item("Resume").Chosen() {
			a.updateSubscription(s.ID, 0, true)
		}
		m.Submenu("Check Every", func(m *ui.Menu) {
			for i, label := range intervalLabels {
				d := intervalValues[i]
				if m.Item(label).Checked(time.Duration(s.IntervalMins)*time.Minute == d).Chosen() {
					a.updateSubscription(s.ID, d, s.Enabled)
				}
			}
		})
		if m.Item("Open Folder").Chosen() {
			a.openPath(backend.GetFolderPath(a.downloadDir(), s.Username))
		}
		m.Separator()
		if m.Item("Unsubscribe…").Danger().Chosen() {
			a.confirmSub = true
		}
	})
	if ui.AlertDialog(c, &a.confirmSub, "Unsubscribe?", "XDL stops checking the account. Its downloads and library entry stay.",
		"Cancel", "Unsubscribe") == 1 && sel != nil {
		id := sel.ID
		go func() {
			if err := a.sched.Unsubscribe(id); err != nil {
				a.win.Update(func() { a.toast(err.Error()) })
			}
			a.loadSubscriptions()
		}()
	}
	ui.Text(c, "Double-click to check now. Right-click to pause, change the interval or unsubscribe.").FontSize(11.5).TextColor(t.TextMuted)
}

func (a *App) submitSubForm() {
	handle := engine.NormalizeHandle(a.subForm.handle)
	timeline := timelineOf(a.subForm.timeline)
	if handle == "" && timeline != "bookmarks" {
		a.toast("Enter a username or a profile URL")
		return
	}
	a.subscribe(handle, timeline, intervalOf(a.subForm.interval))
	a.subForm.handle = ""
}

func (a *App) checkNow(s backend.Subscription) {
	go func() {
		j, err := a.sched.RunNow(s.ID)
		a.win.Update(func() {
			if err != nil {
				a.toast(err.Error())
				return
			}
			a.applyJob(j, false)
			a.toast("Checking " + j.Label())
		})
		a.loadSubscriptions()
	}()
}

func until(at time.Time) string {
	d := time.Until(at)
	switch {
	case d <= time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("in %d min", int(d.Minutes()))
	case sameDay(at, time.Now()):
		return "today " + at.Format("15:04")
	case sameDay(at, time.Now().AddDate(0, 0, 1)):
		return "tomorrow " + at.Format("15:04")
	}
	return at.Format("Jan 2 15:04")
}

func sameDay(a, b time.Time) bool {
	y1, m1, d1 := a.Date()
	y2, m2, d2 := b.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

func intervalLabel(mins int) string {
	for i, d := range intervalValues {
		if time.Duration(mins)*time.Minute == d {
			return intervalLabels[i]
		}
	}
	return "Every " + schedule.Describe(mins)
}
