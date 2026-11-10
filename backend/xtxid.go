package backend

// The X-Client-Transaction-Id header of X's web client: a per-request
// signature derived from the site verification key in x.com's HTML and the
// "loading" animation frames drawn there, as indexed by the ondemand.s
// script. This follows the algorithm documented by the open-source
// XClientTransaction project.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	txEpoch       = 1682924400 // seconds
	txKeyword     = "obfiowerehiring"
	txExtraByte   = 3
	txTotalTime   = 4096.0
	ondemandAsset = "https://abs.twimg.com/responsive-web/client-web/ondemand.s.%sa.js"
)

type txSigner struct {
	keyBytes     []byte
	animationKey string
}

var (
	reMetaTag        = regexp.MustCompile(`<meta\b[^>]*>`)
	reMetaContent    = regexp.MustCompile(`content=["']([^"']+)["']`)
	reOndemandChunk  = regexp.MustCompile(`(\d+):"ondemand\.s"`)
	reOndemandLegacy = regexp.MustCompile(`["']ondemand\.s["']:\s*["']([0-9a-f]+)["']`)
	reIndices        = regexp.MustCompile(`\(\w\[(\d{1,2})\],\s*16\)`)
	reAnimFrame      = regexp.MustCompile(`(?s)id=["']loading-x-anim-[^"']+["'].*?</svg>`)
	rePathD          = regexp.MustCompile(`<path\b[^>]*?\sd=["']([^"']+)["']`)
	reNonDigits      = regexp.MustCompile(`[^\d]+`)
)

func newTxSigner(ctx context.Context, s *xSession) (*txSigner, error) {
	home, err := s.fetchPage(ctx, xHome)
	if err != nil {
		return nil, err
	}
	key, err := verificationKey(home)
	if err != nil {
		return nil, err
	}
	keyBytes, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(keyBytes) < 6 {
		return nil, errors.New("invalid site verification key")
	}
	js, err := signerScript(ctx, s, home)
	if err != nil {
		return nil, err
	}
	rowIndex, keyIndices, err := animationIndices(js)
	if err != nil {
		return nil, err
	}
	frames := reAnimFrame.FindAllString(home, -1)
	animationKey, err := computeAnimationKey(keyBytes, rowIndex, keyIndices, frames)
	if err != nil {
		return nil, err
	}
	return &txSigner{keyBytes: keyBytes, animationKey: animationKey}, nil
}

func verificationKey(html string) (string, error) {
	for _, tag := range reMetaTag.FindAllString(html, -1) {
		if !strings.Contains(tag, "twitter-site-verification") {
			continue
		}
		if m := reMetaContent.FindStringSubmatch(tag); m != nil {
			return m[1], nil
		}
	}
	return "", errors.New("site verification key not found")
}

var (
	reXWebEntry    = regexp.MustCompile(`https://abs\.twimg\.com/x-web/x-web/entry-[A-Za-z0-9_-]+\.js`)
	reXWebChunk    = regexp.MustCompile("[\"`](?:\\./)?assets/([A-Za-z0-9_-]+\\.js)[\"`]")
	reXWebDefault  = regexp.MustCompile("import\\(`\\./([A-Za-z0-9_-]+\\.js)`\\)\\.then\\(\\(?\\w+\\)?=>\\w+\\.default\\(\\)\\)")
	xWebAssetsBase = "https://abs.twimg.com/x-web/x-web/assets/"
)

// signerScript returns the script that holds the animation key indices.
// The classic web client loads it as the ondemand.s chunk; x-web (the
// client x.com serves since late 2026) imports it, obfuscated, from one of
// its entry script's chunks as `import("./<hash>.js").then(e=>e.default())`.
func signerScript(ctx context.Context, s *xSession, home string) (string, error) {
	if u, err := ondemandScriptURL(home); err == nil {
		return s.fetchPage(ctx, u)
	}
	entry := reXWebEntry.FindString(home)
	if entry == "" {
		return "", errors.New("neither ondemand.s nor an x-web entry script found")
	}
	js, err := s.fetchPage(ctx, entry)
	if err != nil {
		return "", err
	}
	seen := map[string]bool{}
	for _, m := range reXWebChunk.FindAllStringSubmatch(js, -1) {
		if seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		chunk, err := s.fetchPage(ctx, xWebAssetsBase+m[1])
		if err != nil {
			continue
		}
		for _, d := range reXWebDefault.FindAllStringSubmatch(chunk, -1) {
			script, err := s.fetchPage(ctx, xWebAssetsBase+d[1])
			if err == nil && len(reIndices.FindAllString(script, 2)) == 2 {
				return script, nil
			}
		}
	}
	return "", errors.New("the x-web signer script was not found")
}

// ondemandScriptURL finds the ondemand.s chunk: the webpack runtime maps
// its chunk ID to a name in one table and to a content hash in another.
func ondemandScriptURL(html string) (string, error) {
	if m := reOndemandChunk.FindStringSubmatch(html); m != nil {
		reHash := regexp.MustCompile(`[,{]` + m[1] + `:"([0-9a-f]{4,})"`)
		if h := reHash.FindStringSubmatch(html); h != nil {
			return fmt.Sprintf(ondemandAsset, h[1]), nil
		}
	}
	if m := reOndemandLegacy.FindStringSubmatch(html); m != nil {
		return fmt.Sprintf(ondemandAsset, m[1]), nil
	}
	return "", errors.New("ondemand.s script not found")
}

func animationIndices(js string) (int, []int, error) {
	var all []int
	for _, m := range reIndices.FindAllStringSubmatch(js, -1) {
		n, _ := strconv.Atoi(m[1])
		all = append(all, n)
	}
	if len(all) < 2 {
		return 0, nil, errors.New("animation key indices not found")
	}
	return all[0], all[1:], nil
}

// computeAnimationKey picks a frame and a row of its curve by key bytes and
// renders the animation at a time the key bytes also give.
func computeAnimationKey(keyBytes []byte, rowIndex int, keyIndices []int, frames []string) (string, error) {
	if len(frames) < 4 {
		return "", errors.New("animation frames not found")
	}
	if rowIndex >= len(keyBytes) {
		return "", errors.New("invalid animation row index")
	}
	paths := rePathD.FindAllStringSubmatch(frames[int(keyBytes[5])%4], -1)
	if len(paths) < 2 || len(paths[1][1]) <= 9 {
		return "", errors.New("animation path not found")
	}
	var rows [][]int
	for _, segment := range strings.Split(paths[1][1][9:], "C") {
		var row []int
		for _, f := range strings.Fields(reNonDigits.ReplaceAllString(segment, " ")) {
			n, _ := strconv.Atoi(f)
			row = append(row, n)
		}
		rows = append(rows, row)
	}
	row := int(keyBytes[rowIndex]) % 16
	if row >= len(rows) || len(rows[row]) < 11 {
		return "", errors.New("animation row too short")
	}
	frameTime := 1.0
	for _, i := range keyIndices {
		if i >= len(keyBytes) {
			return "", errors.New("invalid animation key index")
		}
		frameTime *= float64(int(keyBytes[i]) % 16)
	}
	frameTime = jsRound(frameTime/10) * 10
	return animate(rows[row], frameTime/txTotalTime), nil
}

func animate(frame []int, t float64) string {
	from := []float64{float64(frame[0]), float64(frame[1]), float64(frame[2]), 1}
	to := []float64{float64(frame[3]), float64(frame[4]), float64(frame[5]), 1}
	toRotation := solve(float64(frame[6]), 60, 360, true)
	var curves []float64
	for i, v := range frame[7:] {
		curves = append(curves, solve(float64(v), oddness(i), 1, false))
	}
	val := cubicValue(curves, t)

	color := make([]float64, len(from))
	for i := range from {
		color[i] = math.Max(0, from[i]*(1-val)+to[i]*val)
	}
	rotation := toRotation * val
	rad := rotation * math.Pi / 180
	matrix := []float64{math.Cos(rad), -math.Sin(rad), math.Sin(rad), math.Cos(rad)}

	var b strings.Builder
	for _, c := range color[:3] {
		b.WriteString(strconv.FormatInt(int64(math.RoundToEven(c)), 16))
	}
	for _, v := range matrix {
		v = math.Abs(roundTo(v, 2))
		h := floatToHex(v)
		switch {
		case strings.HasPrefix(h, "."):
			b.WriteString("0" + strings.ToLower(h))
		case h == "":
			b.WriteString("0")
		default:
			b.WriteString(strings.ToLower(h))
		}
	}
	b.WriteString("00")
	return strings.NewReplacer(".", "", "-", "").Replace(b.String())
}

func solve(v, lo, hi float64, floor bool) float64 {
	r := v*(hi-lo)/255 + lo
	if floor {
		return math.Floor(r)
	}
	return roundTo(r, 2)
}

func oddness(i int) float64 {
	if i%2 == 1 {
		return -1
	}
	return 0
}

// cubicValue evaluates the cubic Bézier easing curve (x1, y1, x2, y2) at t.
func cubicValue(c []float64, t float64) float64 {
	if len(c) < 4 {
		return t
	}
	if t <= 0 {
		g := 0.0
		if c[0] > 0 {
			g = c[1] / c[0]
		} else if c[1] == 0 && c[2] > 0 {
			g = c[3] / c[2]
		}
		return g * t
	}
	if t >= 1 {
		g := 0.0
		if c[2] < 1 {
			g = (c[3] - 1) / (c[2] - 1)
		} else if c[2] == 1 && c[0] < 1 {
			g = (c[1] - 1) / (c[0] - 1)
		}
		return 1 + g*(t-1)
	}
	start, end, mid := 0.0, 1.0, 0.0
	for start < end {
		mid = (start + end) / 2
		x := bezier(c[0], c[2], mid)
		if math.Abs(t-x) < 0.00001 {
			return bezier(c[1], c[3], mid)
		}
		if x < t {
			start = mid
		} else {
			end = mid
		}
	}
	return bezier(c[1], c[3], mid)
}

func bezier(a, b, m float64) float64 {
	return 3*a*(1-m)*(1-m)*m + 3*b*(1-m)*m*m + m*m*m
}

// jsRound rounds half up, as JavaScript's Math.round.
func jsRound(x float64) float64 {
	f := math.Floor(x)
	if x-f >= 0.5 {
		f = math.Ceil(x)
	}
	return math.Copysign(f, x)
}

func roundTo(x float64, digits int) float64 {
	p := math.Pow(10, float64(digits))
	return math.RoundToEven(x*p) / p
}

// floatToHex writes x in base 16, fraction included (0.5 → ".8").
func floatToHex(x float64) string {
	var whole []byte
	q := math.Trunc(x)
	fraction := x - q
	for q > 0 {
		next := math.Trunc(x / 16)
		rem := int(x - next*16)
		whole = append([]byte{hexDigit(rem)}, whole...)
		x, q = next, next
	}
	if fraction == 0 {
		return string(whole)
	}
	out := append(whole, '.')
	for i := 0; fraction > 0 && i < 64; i++ {
		fraction *= 16
		d := int(fraction)
		fraction -= float64(d)
		out = append(out, hexDigit(d))
	}
	return string(out)
}

func hexDigit(d int) byte {
	if d > 9 {
		return byte('A' + d - 10)
	}
	return byte('0' + d)
}

func (t *txSigner) id(method, path string, now time.Time) string {
	elapsed := now.Unix() - txEpoch
	if elapsed < 0 {
		elapsed = 0
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s!%s!%d%s%s", method, path, elapsed, txKeyword, t.animationKey)))
	payload := make([]byte, 0, len(t.keyBytes)+4+16+1)
	payload = append(payload, t.keyBytes...)
	payload = append(payload, byte(elapsed), byte(elapsed>>8), byte(elapsed>>16), byte(elapsed>>24))
	payload = append(payload, sum[:16]...)
	payload = append(payload, txExtraByte)

	var r [1]byte
	_, _ = rand.Read(r[:])
	out := make([]byte, 1, len(payload)+1)
	out[0] = r[0]
	for _, b := range payload {
		out = append(out, b^r[0])
	}
	return base64.RawStdEncoding.EncodeToString(out)
}
