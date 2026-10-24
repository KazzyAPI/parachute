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

type ConfigUnreproducibleError struct {
	Field string
}

func (e *ConfigUnreproducibleError) Error() string {
	return fmt.Sprintf("configuration cannot be reproduced: %s", e.Field)
}

type PullFailedError struct {
	Detail string
}

func (e *PullFailedError) Error() string {
	return fmt.Sprintf("pull failed: %s", e.Detail)
}

type ImageMismatchError struct {
	Pulled  string
	Created string
}

func (e *ImageMismatchError) Error() string {
	return fmt.Sprintf("image mismatch: pulled %s, replacement %s", e.Pulled, e.Created)
}
