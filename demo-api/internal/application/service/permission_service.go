package service

import (
	"context"
	"github.com/NeftaliAcosta/springo/demo-api/internal/infrastructure/dtos/response"
	"github.com/NeftaliAcosta/springo/framework/cache"
	"github.com/NeftaliAcosta/springo/framework/ioc"
	"github.com/NeftaliAcosta/springo/framework/logging"
	"time"
)

// PermissionService handles user authorization logic with caching
type PermissionService interface {
	GetUserPermissions(ctx context.Context, userID string) (response.PermissionResponseDTO, error)
}

type permissionServiceImpl struct{}

func init() {
	// Register as a Bean
	ioc.GetContainer().RegisterServiceFactory("PermissionService", func() interface{} {
		return &permissionServiceImpl{}
	})
}

func (s *permissionServiceImpl) GetUserPermissions(ctx context.Context, userID string) (response.PermissionResponseDTO, error) {
	// 🎯 Using the Cache Abstraction
	// Region: "permissions" (configured in application.yaml with 1h TTL)
	// Key: the userID
	return cache.Execute(ctx, "permissions", userID, func() (response.PermissionResponseDTO, error) {

		// 🛠️ Using Structured Logging (Automatic TraceID from context)
		logging.Info(ctx, "Fetching permissions from database", "user_id", userID)
		time.Sleep(500 * time.Millisecond)

		permissions := []string{"USER_READ", "USER_WRITE"}
		if userID == "admin" {
			permissions = append(permissions, "ADMIN_ACCESS")
		}

		return response.PermissionResponseDTO{
			UserID:      userID,
			Permissions: permissions,
			Source:      "Database", // This will be cached
		}, nil
	})
}
