package config_test

import (
	"testing"

	"github.com/NeftaliAcosta/springo/framework/config"
	"github.com/NeftaliAcosta/springo/framework/ioc"
)

func TestWhenConditionMet(t *testing.T) {
	config.ClearConditionals()
	defer config.ClearConditionals()

	executed := false
	config.When("encryption.enabled", true, func() {
		executed = true
	})

	loader := config.NewConfigLoader()
	loader.Data = map[string]interface{}{
		"encryption": map[string]interface{}{
			"enabled": true,
		},
	}

	config.ExecuteConditionals(loader)

	if !executed {
		t.Fatalf("expected callback to execute when property condition was met")
	}
}

func TestWhenConditionNotMet(t *testing.T) {
	config.ClearConditionals()
	defer config.ClearConditionals()

	executed := false
	config.When("encryption.enabled", true, func() {
		executed = true
	})

	loader := config.NewConfigLoader()
	loader.Data = map[string]interface{}{
		"encryption": map[string]interface{}{
			"enabled": false,
		},
	}

	config.ExecuteConditionals(loader)

	if executed {
		t.Fatalf("expected callback NOT to execute when condition was not met")
	}
}

func TestWhenProfileMatches(t *testing.T) {
	config.ClearConditionals()
	defer config.ClearConditionals()

	executed := false
	config.WhenProfile([]string{"prod", "staging"}, func() {
		executed = true
	})

	loader := config.NewConfigLoader()
	loader.ActiveProfile = "prod"

	config.ExecuteConditionals(loader)

	if !executed {
		t.Fatalf("expected profile callback to execute for matching active profile 'prod'")
	}
}

func TestWhenProfileSingleString(t *testing.T) {
	config.ClearConditionals()
	defer config.ClearConditionals()

	executed := false
	config.WhenProfile("prod", func() {
		executed = true
	})

	loader := config.NewConfigLoader()
	loader.ActiveProfile = "prod"

	config.ExecuteConditionals(loader)

	if !executed {
		t.Fatalf("expected single string profile callback to execute")
	}
}

func TestWhenProfileDoesNotMatch(t *testing.T) {
	config.ClearConditionals()
	defer config.ClearConditionals()

	executed := false
	config.WhenProfile([]string{"prod", "staging"}, func() {
		executed = true
	})

	loader := config.NewConfigLoader()
	loader.ActiveProfile = "dev"

	config.ExecuteConditionals(loader)

	if executed {
		t.Fatalf("expected profile callback NOT to execute for non-matching profile 'dev'")
	}
}

type mockNotificationService struct {
	Driver string
}

func TestRegisterConditionalBeanMatching(t *testing.T) {
	config.ClearConditionals()
	defer config.ClearConditionals()
	ioc.GetContainer().ResetAll()
	defer ioc.GetContainer().ResetAll()

	config.RegisterConditionalBean("notificationService", "notifications.enabled", true, func() interface{} {
		return &mockNotificationService{Driver: "smtp"}
	})

	loader := config.NewConfigLoader()
	loader.Data = map[string]interface{}{
		"notifications": map[string]interface{}{
			"enabled": true,
		},
	}

	config.ExecuteConditionals(loader)

	ioc.GetContainer().InitializeAllBeans(nil)

	bean := ioc.GetContainer().GetBean("notificationService")
	if bean == nil {
		t.Fatalf("expected bean 'notificationService' to be registered when condition was met")
	}

	service, ok := bean.(*mockNotificationService)
	if !ok || service.Driver != "smtp" {
		t.Fatalf("unexpected bean instance: %v", bean)
	}
}

func TestRegisterConditionalBeanNotMatching(t *testing.T) {
	config.ClearConditionals()
	defer config.ClearConditionals()
	ioc.GetContainer().ResetAll()
	defer ioc.GetContainer().ResetAll()

	config.RegisterConditionalBean("disabledService", "notifications.enabled", true, func() interface{} {
		return &mockNotificationService{Driver: "smtp"}
	})

	loader := config.NewConfigLoader()
	loader.Data = map[string]interface{}{
		"notifications": map[string]interface{}{
			"enabled": false,
		},
	}

	config.ExecuteConditionals(loader)

	ioc.GetContainer().InitializeAllBeans(nil)

	bean := ioc.GetContainer().GetBean("disabledService")
	if bean != nil {
		t.Fatalf("expected bean 'disabledService' NOT to be registered when condition failed")
	}
}

type ObservabilityProperties struct {
	ServiceName string `yaml:"service-name"`
	SamplerRate int    `yaml:"sampler-rate"`
}

func TestCoexistenceModels(t *testing.T) {
	config.ResetProperties()
	defer config.ResetProperties()

	props := &ObservabilityProperties{
		ServiceName: "default-service",
		SamplerRate: 10,
	}
	config.RegisterProperties("observability", props)

	loader := config.NewConfigLoader()
	loader.Data = map[string]interface{}{
		"observability": map[string]interface{}{
			"service-name": "payment-api",
			"sampler-rate": 100,
		},
	}

	if err := config.InitializeProperties(loader); err != nil {
		t.Fatalf("failed to initialize properties: %v", err)
	}

	verifyTypedModel(t)
	verifyDirectModel(t)
}

func verifyTypedModel(t *testing.T) {
	t.Helper()
	typed := config.Get[ObservabilityProperties]()
	if typed == nil || typed.ServiceName != "payment-api" || typed.SamplerRate != 100 {
		t.Fatalf("expected typed struct to have 'payment-api' and 100, got %v", typed)
	}
}

func verifyDirectModel(t *testing.T) {
	t.Helper()
	if name := config.GetString("observability.service-name", ""); name != "payment-api" {
		t.Fatalf("expected direct read to return 'payment-api', got %q", name)
	}
	if rate := config.GetInt("observability.sampler-rate", 0); rate != 100 {
		t.Fatalf("expected direct read to return 100, got %d", rate)
	}
}
