package domain

import "time"

type Hold struct {
	Reason string
	Until  *time.Time
	Detail string
}
