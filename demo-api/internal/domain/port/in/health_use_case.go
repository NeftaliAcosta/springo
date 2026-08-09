package in

import (
	"context"
	"github.com/NeftaliAcosta/springo/demo-api/internal/infrastructure/dtos/response"
)

// HealthUseCase is an inbound port for system monitoring
type HealthUseCase interface {
	CheckStatus(ctx context.Context) response.HealthResponseDTO
	GetInfo(ctx context.Context) response.InfoResponseDTO
}
