package gui

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

func (a *App) logsPage(c *ui.Context) {
	t := c.Theme()
	pageHeader(c, "Logs", "What XDL has been doing since it started.", func() {
		if ui.Button(c, "Copy").Disabled(len(a.logs) == 0).Clicked() {
			c.WriteClipboard(strings.Join(a.logs, "\n"))
			a.toast("Copied the log")
		}
	})
	lines := a.logs
	logList.FollowEnd = true
	ui.List(c, &logList, len(lines), func(i int) {
		ui.Text(c, lines[i]).Font("monospace").FontSize(11.5).Padding(2, 10).Selectable()
	}).Grow(1).Basis(0).Radius(8).Background(t.Surface).Border(1, t.Border).Children(func() {
		if len(lines) == 0 {
			ui.Text(c, "Nothing yet.").TextColor(t.TextMuted).Padding(12)
		}
	})
}

var logList ui.ListState
