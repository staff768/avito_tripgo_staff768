package positions

import "time"

type realTicker struct{ *time.Ticker }

func NewTicker(interval time.Duration) Ticker {
	return realTicker{Ticker: time.NewTicker(interval)}
}

func (t realTicker) Ticks() <-chan time.Time { return t.C }
