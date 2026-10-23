package drift

import (
	"context"

	"github.com/KazzyAPI/parachute/internal/pipeline"
)

const configFilesLabel = "com.docker.compose.project.config_files"

type Stage struct {
	detector *Detector
}

func NewStage(detector *Detector) *Stage {
	return &Stage{detector: detector}
}

func (s *Stage) Run(_ context.Context, cycle *pipeline.Cycle) error {
	for _, host := range cycle.ReachableHosts() {
		for _, candidate := range host.Candidates {
			container := candidate.Container
			configFiles := container.Labels[configFilesLabel]
			if container.IsCompose() && configFiles != "" && configFiles != container.ImageLabels[configFilesLabel] {
				candidate.Result.Drift = s.detector.Drift(configFiles, container.Service(), container.ImageRef)
			}
		}
	}
	return nil
}
