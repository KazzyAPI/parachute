package recreate

import "fmt"

type Failure struct {
	State  string
	Reason string
	Detail string
}

func (f Failure) Error() string {
	if f.Detail == "" {
		return f.Reason
	}
	return fmt.Sprintf("%s: %s", f.Reason, f.Detail)
}
