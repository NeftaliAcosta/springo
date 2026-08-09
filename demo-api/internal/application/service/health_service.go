package service

import (
	"context"
	"github.com/NeftaliAcosta/springo/demo-api/internal/domain/port/in"
	configInternal "github.com/NeftaliAcosta/springo/demo-api/internal/infrastructure/config"
	"github.com/NeftaliAcosta/springo/demo-api/internal/infrastructure/dtos/response"
	"github.com/NeftaliAcosta/springo/framework/config"
	"github.com/NeftaliAcosta/springo/framework/ioc"
	"github.com/NeftaliAcosta/springo/framework/version"
	"github.com/NeftaliAcosta/springo/framework/web"
	"os"
)

type healthServiceImpl struct{}

// init registers the health service factory automatically
func init() {
	ioc.GetContainer().RegisterServiceFactory("HealthService", func() interface{} {
		return NewHealthService()
	})
}

// NewHealthService creates a new instance
func NewHealthService() in.HealthUseCase {
	return &healthServiceImpl{}
}

// CheckStatus verifies the health of all system components using the framework engine
func (s *healthServiceImpl) CheckStatus(ctx context.Context) response.HealthResponseDTO {
	info := web.GetHealthInfo()

	return response.HealthResponseDTO{
		Status:     info.Status,
		Uptime:     info.Uptime,
		Components: info.Components,
		System:     info.System,
	}
}

func (s *healthServiceImpl) GetInfo(ctx context.Context) response.InfoResponseDTO {
	serverProps := config.Get[configInternal.ServerProperties]()
	profile := os.Getenv("SPRINGO_PROFILES_ACTIVE")
	if profile == "" {
		profile = "default"
	}

	return response.InfoResponseDTO{
		App: response.AppInfo{
			Name:        serverProps.Name,
			Description: "SprinGo Framework Application",
		},
		Version:     version.Current,
		Environment: profile,
	}
}

var _ in.HealthUseCase = (*healthServiceImpl)(nil)
