package main

import (
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"strconv"
	"time"
)

const (
	alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.- "
)

const (
	LogisticMode = "logistic"
	RampMode     = "ramp"
)

var opts struct {
	peakRate string
	mode     string
	skips    int
	skipProb float64

	duration  time.Duration
	stepSize  time.Duration
	sliceLen  int
	blockSize int

	// logistic flags
	scale int

	// ramp flags
	rampDuration time.Duration
}

func init() {
	flag.StringVar(&opts.peakRate, "rate", "128", "peak character rate in chars/s")
	flag.StringVar(&opts.mode, "mode", LogisticMode, "the operation mode, one of 'logistic' or 'ramp'")

	flag.IntVar(&opts.skips, "skips", 2, "expected number of time steps with no output per slice")
	flag.Float64Var(&opts.skipProb, "skip-probability", 0, "probability that a given slice will contain skips")

	flag.DurationVar(&opts.duration, "duration", 60*time.Second, "duration")
	flag.DurationVar(&opts.stepSize, "step-size", 250*time.Millisecond, "length of each time step")
	flag.IntVar(&opts.sliceLen, "slice-length", 16, "number of time steps per slice")
	flag.IntVar(&opts.blockSize, "block-size", 4096, "maximum number of characters printed in one line/operation")

	// logistic flags
	flag.IntVar(&opts.scale, "scale", 25, "scale factor for the output distribution; only used with -mode=logistic")

	// ramp flags
	flag.DurationVar(&opts.rampDuration, "ramp-duration", 10*time.Second, "time taken to reach the peak rate; only used with -mode=ramp")
}

func main() {
	flag.Parse()

	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	rate, err := parseRate(opts.peakRate)
	if err != nil {
		die(err)
	}
	if opts.stepSize > opts.duration {
		die("invalid step size: must be less than duration")
	}
	if opts.skips > opts.sliceLen {
		die("invalid skips: must be less than slice length")
	}
	if opts.skipProb > 1 || opts.skipProb < 0 {
		die("invalid skip probability: must be in [0.0, 1.0]")
	}

	charsPerStep := float64(rate) * opts.stepSize.Seconds()

	var shaper RateShaper
	switch opts.mode {
	case LogisticMode:
		peakStep := r.Intn(int(opts.duration / opts.stepSize))
		shaper = LogisticShaper{Mu: peakStep, Scale: opts.scale}

	case RampMode:
		peakStep := int(opts.rampDuration / opts.stepSize)
		shaper = RampShaper{PeakStep: peakStep}

	default:
		die("invalid mode: must be one of 'logistic' or 'ramp'")
	}

	out := NewRandomOutput(r, 32, opts.blockSize)

	end := time.After(opts.duration)
	steps := time.Tick(opts.stepSize)

	var skips int
	for step := 0; true; step++ {
		sliceIdx := step % opts.sliceLen
		if sliceIdx == 0 {
			skips = sampleSkips(r, opts.skips, opts.skipProb)
		}

		select {
		case <-steps:
			if sliceIdx < opts.sliceLen-skips {
				n := int(charsPerStep * shaper.Fraction(step))
				out.WriteN(os.Stdout, n)
			}
		case <-end:
			return
		}
	}
}

func die(msg interface{}) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}

func sampleSkips(r *rand.Rand, skips int, skipProb float64) int {
	if skips <= 0 || r.Float64() >= skipProb {
		return 0
	}

	// https://en.wikipedia.org/wiki/Poisson_distribution
	l := math.Exp(-float64(skips))
	k := 0
	p := float64(1)
	for {
		p *= r.Float64()
		if p <= l {
			return k
		}
		k += 1
	}
}

// RateShaper shapes how the output scales by returning a multiple between 0.0
// and 1.0 of the peak rate for each step.
type RateShaper interface {
	Fraction(step int) float64
}

type LogisticShaper struct {
	Mu    int
	Scale int
}

func (s LogisticShaper) Fraction(step int) float64 {
	// https://en.wikipedia.org/wiki/Logistic_distribution
	// Scaled to fit in [0.0, 1.0]
	a := math.Exp(-float64(step-s.Mu) / float64(s.Scale))
	b := float64(s.Scale) * (1 + a) * (1 + a)
	return float64(4*s.Scale) * (a / b)
}

type RampShaper struct {
	PeakStep int
}

func (s RampShaper) Fraction(step int) float64 {
	if step < s.PeakStep {
		return float64(step) / float64(s.PeakStep)
	}
	return 1.0
}

type RandomOutput struct {
	bufs [][]byte
	r    *rand.Rand
}

func NewRandomOutput(r *rand.Rand, n, blockSize int) *RandomOutput {
	bufs := make([][]byte, n)
	for i := range bufs {
		bufs[i] = make([]byte, blockSize)
		for j := range bufs[i] {
			bufs[i][j] = byte(alphabet[r.Intn(len(alphabet))])
		}
		bufs[i][blockSize-1] = '\n'
	}

	return &RandomOutput{
		bufs: bufs,
		r:    r,
	}
}

func (ro *RandomOutput) WriteN(w io.Writer, n int) (err error) {
	for n > 0 {
		buf := ro.pickBuffer()
		var nr int
		if len(buf) > n {
			nr, err = w.Write(buf[len(buf)-n:])
		} else {
			nr, err = w.Write(buf)
		}
		if err != nil {
			return err
		}
		n -= nr
	}
	return nil
}

func (ro *RandomOutput) pickBuffer() []byte {
	return ro.bufs[ro.r.Intn(len(ro.bufs))]
}

func parseRate(rate string) (int64, error) {
	if rate == "" {
		return 0, fmt.Errorf("invalid rate: rate must be non-empty")
	}

	scale := int64(1)
	switch rate[len(rate)-1] {
	case 'k', 'K':
		rate = rate[:len(rate)-1]
		scale = 1000
	case 'm', 'M':
		rate = rate[:len(rate)-1]
		scale = 1000000
	case 'g', 'G':
		rate = rate[:len(rate)-1]
		scale = 1000000000
	}

	base, err := strconv.ParseInt(rate, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid rate: %w", err)
	}
	return scale * base, nil
}
