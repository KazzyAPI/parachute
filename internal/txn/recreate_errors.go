package txn

import "fmt"

type ConfigUnreproducibleError struct {
	Field string
}

func (e *ConfigUnreproducibleError) Error() string {
	return fmt.Sprintf("config_unreproducible: %s", e.Field)
}

type PullFailedError struct {
	Repository string
	Approved   string
	Detail     string
}

func (e *PullFailedError) Error() string {
	return fmt.Sprintf("pull_failed: %s@%s: %s", e.Repository, e.Approved, e.Detail)
}

type ImageMismatchError struct {
	Approved string
	Actual   string
}

func (e *ImageMismatchError) Error() string {
	return fmt.Sprintf("image_mismatch: approved %s, got %s", e.Approved, e.Actual)
}
