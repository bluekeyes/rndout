# rndout

Generate random output for testing. Originally written to simulate output from
builds while testing a CI system. May be useful for other things that just need
throughput and not a specific data format.

## Usage

```
$ rndout -help

  -block-size int
        maximum number of characters printed in one line/operation (default 4096)
  -duration duration
        duration (default 1m0s)
  -mode string
        the operation mode, one of 'logistic' or 'ramp' (default "logistic")
  -ramp-duration duration
        time taken to reach the peak rate; only used with -mode=ramp (default 10s)
  -rate string
        peak character rate in chars/s (default "128")
  -scale int
        scale factor for the output distribution; only used with -mode=logistic (default 25)
  -skip-probability float
        probability that a given slice will contain skips
  -skips int
        expected number of time steps with no output per slice (default 2)
  -slice-length int
        number of time steps per slice (default 16)
  -step-size duration
        length of each time step (default 250ms)
```

Output is written to `stdout`.

## Algorithm

### `ramp` mode

Linearly increase the output rate on each step until reaching the peak output
rate after the ramp duration. Remain at that output rate for the remaining
time.

### `logistic` mode

1. Divide the duration by the step size
2. Select a random step at which to reach the peak output rate
3. For each step, print random ASCII characters such that the output rate
   follows a logistic distribution with scale `scale` centered at the peak step
4. Every `slice-length` steps, randomly sample a Poisson distribution to
   determine how many steps to skip printing output. This reduces the actual
   output rate but can add more realistic pauses and gaps in the output.

## License

MIT
