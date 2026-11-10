package gui

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/missuo/xdl/backend"
	"github.com/missuo/xdl/internal/engine"
)

// updateTray refreshes the menu bar icon, its tooltip and its menu.
func (a *App) updateTray() {
	if a.tray == nil {
		return
	}
	active := a.activeJob()
	queued := 0
	for _, j := range a.jobs {
		if j.State == engine.StateQueued {
			queued++
		}
	}
	finished := a.trayFinished
	a.trayFinished = false
	a.setTrayIcon(active, finished && active == nil && queued == 0)

	status := "XDL is idle"
	if active != nil {
		status = active.Label() + " — " + active.Summary()
	}
	queue := ""
	if queued > 0 {
		queue = fmt.Sprintf("%d more queued", queued)
	}
	key := status + "|" + queue + "|" + a.apiErr
	if key == a.trayKey {
		return
	}
	a.trayKey = key
	a.tray.SetToolTip("XDL — " + status)

	items := []*mygo.MenuItem{{Label: truncateLabel(status, 60), Disabled: true}}
	if queue != "" {
		items = append(items, &mygo.MenuItem{Label: queue, Disabled: true})
	}
	if a.apiErr != "" {
		items = append(items, &mygo.MenuItem{Label: "Browser bridge unavailable", Disabled: true, ToolTip: a.apiErr})
	}
	if active != nil {
		id := active.ID
		items = append(items, &mygo.MenuItem{Label: "Stop " + active.Label(), Click: func(*mygo.MenuItem, *mygo.Window) {
			a.eng.Cancel(id)
		}})
	}
	items = append(items,
		mygo.Separator(),
		&mygo.MenuItem{Label: "Download Profile from Clipboard", Click: func(*mygo.MenuItem, *mygo.Window) {
			a.downloadClipboard()
		}},
		&mygo.MenuItem{Label: "Open XDL", Click: func(*mygo.MenuItem, *mygo.Window) { a.show() }},
		&mygo.MenuItem{Label: "Open Download Folder", Click: func(*mygo.MenuItem, *mygo.Window) {
			a.openPath(a.downloadDir())
		}},
		mygo.Separator(),
		&mygo.MenuItem{Label: "Quit XDL", Accelerator: "CmdOrCtrl+Q", Click: func(*mygo.MenuItem, *mygo.Window) {
			mygo.App.Quit()
		}},
	)
	a.tray.SetMenu(mygo.NewMenu(items))
}

// downloadClipboard queues the profile whose URL or @handle is on the
// clipboard.
func (a *App) downloadClipboard() {
	text := strings.TrimSpace(mygo.Clipboard.ReadText())
	req := engine.Request{Username: text}
	if text == "" || req.Validate() != nil || strings.ContainsAny(text, " \n") {
		a.notify("clipboard", "XDL", "The clipboard holds no X profile URL or @handle.")
		return
	}
	a.cfg = a.store.Get()
	j, err := a.eng.Submit(engine.Request{Username: text, Source: "clipboard"})
	if err != nil && err != engine.ErrDuplicate {
		a.notify("clipboard", "XDL", err.Error())
		return
	}
	a.notify(j.ID+"-queued", "XDL", "Queued "+j.Label())
}

func truncateLabel(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// fetchAvatar downloads an account picture at 96 px, or returns nil.
func fetchAvatar(url, proxy string) *ui.Bitmap {
	url = strings.Replace(url, "_normal.", "_bigger.", 1)
	client, err := backend.CreateHTTPClient(proxy, 15*time.Second)
	if err != nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil
	}
	if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err != nil {
		return nil
	}
	bm, err := ui.DecodeBitmap(data)
	if err != nil {
		log.Printf("avatar %s: %v", url, err)
		return nil
	}
	return bm
}
