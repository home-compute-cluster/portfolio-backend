package clock

import "time"

type Real struct{}

func (Real) Now() time.Time {
	return time.Now()
}
