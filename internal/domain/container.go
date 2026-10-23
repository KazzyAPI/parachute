package domain

const (
	composeProjectLabel = "com.docker.compose.project"
	composeServiceLabel = "com.docker.compose.service"
)

type Container struct {
	ID          string
	Name        string
	State       string
	Labels      map[string]string
	ImageRef    string
	ImageID     string
	ImageLabels map[string]string
	Own         bool
}

func (c Container) IsCompose() bool {
	return c.Labels[composeProjectLabel] != ""
}

func (c Container) Stack() string {
	if c.IsCompose() {
		return c.Labels[composeProjectLabel]
	}
	return c.Name
}

func (c Container) Service() string {
	if service := c.Labels[composeServiceLabel]; c.IsCompose() && service != "" {
		return service
	}
	return c.Name
}
