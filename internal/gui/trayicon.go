package gui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"golang.org/x/image/vector"

	"github.com/missuo/xdl/internal/engine"
)

// The menu bar (and notification area) icon. Idle, it is the logo's X: a
// falling bar and a lightning bolt; while a job fetches, an arc spins
// around the bolt; while it downloads, the ring fills clockwise with its
// progress; when it finishes, a check shows for a moment before the idle
// icon returns. macOS
// tints the black template image itself; elsewhere the icon is white on dark
// task bars and black on light ones.

type trayLook int

const (
	lookIdle     trayLook = iota
	lookSpin              // fetching: frame is the spinner's step
	lookProgress          // downloading: frame is the percentage
	lookDone
)

var (
	trayIdleSVG = icon("xdl-tray")
	trayBoltSVG = icon("xdl-bolt")
	trayDoneSVG = icon("circle-check")
)

const (
	traySize   = 36 // 18 points at 2x; Windows scales it to 16 or 32 px
	spinSteps  = 12
	spinEvery  = 90 * time.Millisecond
	doneShowed = 2500 * time.Millisecond
)

func trayInk(dark bool) color.Color {
	if runtime.GOOS != "darwin" && dark {
		return color.White
	}
	return color.Black
}

// trayTemplate reports whether the platform tints the icon itself.
func trayTemplate() bool { return runtime.GOOS == "darwin" }

func trayIcon(look trayLook, frame int, dark bool) []byte {
	ink := trayInk(dark)
	var img image.Image
	switch look {
	case lookIdle:
		img = trayIdleSVG.Image(traySize, traySize, ink)
	case lookDone:
		img = trayDoneSVG.Image(traySize, traySize, ink)
	default:
		img = drawRing(look, frame, ink)
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

// drawRing draws the bolt inside a ring: a faint track under a solid arc,
// the download's share of it or the spinner's quarter.
func drawRing(look trayLook, frame int, ink color.Color) image.Image {
	const (
		c     = traySize / 2.0
		outer = traySize/2.0 - 2
		width = 3.2
	)
	track := vector.NewRasterizer(traySize, traySize)
	arc(track, c, c, outer, outer-width, 0, 2*math.Pi)

	solid := vector.NewRasterizer(traySize, traySize)
	switch look {
	case lookProgress:
		p := math.Max(0.04, math.Min(1, float64(frame)/100))
		arc(solid, c, c, outer, outer-width, 0, p*2*math.Pi)
	case lookSpin:
		start := float64(frame%spinSteps) / spinSteps * 2 * math.Pi
		arc(solid, c, c, outer, outer-width, start, math.Pi/2)
	}
	bounds := image.Rect(0, 0, traySize, traySize)
	trackMask := image.NewAlpha(bounds)
	track.Draw(trackMask, bounds, image.Opaque, image.Point{})
	solidMask := image.NewAlpha(bounds)
	solid.Draw(solidMask, bounds, image.Opaque, image.Point{})

	// The logo's bolt inside the ring: the whole X is too fine at this size.
	const glyph = 20
	mark := image.NewAlpha(bounds)
	small := trayBoltSVG.Image(glyph, glyph, color.Black)
	off := (traySize - glyph) / 2
	for y := range glyph {
		for x := range glyph {
			_, _, _, a := small.At(small.Bounds().Min.X+x, small.Bounds().Min.Y+y).RGBA()
			mark.SetAlpha(off+x, off+y, color.Alpha{A: uint8(a >> 8)})
		}
	}

	r, g, b, _ := ink.RGBA()
	out := image.NewNRGBA(bounds)
	for i := range solidMask.Pix {
		a := max(uint16(trackMask.Pix[i])*35/100, uint16(solidMask.Pix[i]), uint16(mark.Pix[i]))
		out.Pix[i*4+0] = uint8(r >> 8)
		out.Pix[i*4+1] = uint8(g >> 8)
		out.Pix[i*4+2] = uint8(b >> 8)
		out.Pix[i*4+3] = uint8(a)
	}
	return out
}

// arc adds a ring segment starting at start radians, clockwise from the
// top, sweeping sweep radians.
func arc(r *vector.Rasterizer, cx, cy, outer, inner, start, sweep float64) {
	n := max(8, int(sweep/(2*math.Pi)*96))
	point := func(radius, a float64) (float32, float32) {
		return float32(cx + radius*math.Sin(a)), float32(cy - radius*math.Cos(a))
	}
	x, y := point(outer, start)
	r.MoveTo(x, y)
	for i := 1; i <= n; i++ {
		x, y = point(outer, start+sweep*float64(i)/float64(n))
		r.LineTo(x, y)
	}
	for i := n; i >= 0; i-- {
		x, y = point(inner, start+sweep*float64(i)/float64(n))
		r.LineTo(x, y)
	}
	r.ClosePath()
}

// trayAnimator keeps the icon in step with the jobs. It runs on the main
// thread; its ticker only asks the main thread for the next frame.
type trayAnimator struct {
	look      trayLook
	frame     int
	shown     string // the look and frame on screen, to skip redundant icons
	doneUntil time.Time
	spinning  chan struct{}
}

// setIcon shows the icon for the jobs' state: active is the running job,
// if any; justFinished says a job ended since the last call.
func (a *App) setTrayIcon(active *engine.Job, justFinished bool) {
	t := &a.trayAnim
	switch {
	case active != nil && active.State == engine.StateDownloading:
		t.look, t.frame = lookProgress, active.Percent
	case active != nil:
		if t.look != lookSpin {
			t.frame = 0
		}
		t.look = lookSpin
	case justFinished || time.Now().Before(t.doneUntil):
		if t.look != lookDone {
			t.doneUntil = time.Now().Add(doneShowed)
			time.AfterFunc(doneShowed+50*time.Millisecond, func() {
				mygo.RunOnMain(func() { a.setTrayIcon(a.activeJob(), false) })
			})
		}
		t.look = lookDone
	default:
		t.look = lookIdle
	}

	// Spin while fetching.
	if t.look == lookSpin && t.spinning == nil {
		stop := make(chan struct{})
		t.spinning = stop
		go func() {
			tick := time.NewTicker(spinEvery)
			defer tick.Stop()
			for {
				select {
				case <-stop:
					return
				case <-tick.C:
					mygo.RunOnMain(func() {
						if a.trayAnim.look == lookSpin {
							a.trayAnim.frame++
							a.drawTrayIcon()
						}
					})
				}
			}
		}()
	} else if t.look != lookSpin && t.spinning != nil {
		close(t.spinning)
		t.spinning = nil
	}
	a.drawTrayIcon()
}

func (a *App) drawTrayIcon() {
	t := &a.trayAnim
	key := fmt.Sprint(t.look, t.frame, a.tray.Dark())
	if key == t.shown {
		return
	}
	t.shown = key
	_ = a.tray.SetIcon(trayIcon(t.look, t.frame, a.tray.Dark()), trayTemplate())
}

func (a *App) activeJob() *engine.Job {
	for i := range a.jobs {
		if s := a.jobs[i].State; s == engine.StateFetching || s == engine.StateDownloading {
			return &a.jobs[i]
		}
	}
	return nil
}
