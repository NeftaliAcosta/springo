package web

import (
	"github.com/NeftaliAcosta/springo/framework/ioc"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestActuator_BasicAuthGenerated(t *testing.T) {
	// Re-register BasicAuthProperties with empty password to force auto-generation
	props := &BasicAuthProperties{
		Name:     "custom-admin",
		Password: "",
	}
	ioc.GetContainer().RegisterBean("BasicAuthProperties", props)

	// Fetch credentials
	user, pass := getOrGenerateCredentials()
	assert.Equal(t, "custom-admin", user)
	assert.NotEmpty(t, pass)
	assert.Len(t, pass, 16)
}

func TestActuator_MaskSensitiveData(t *testing.T) {
	type testConfig struct {
		AdminPassword string `yaml:"admin-password"`
		AppSecret     string `json:"app_secret"`
		NormalData    string `yaml:"normal-data"`
		PlainToken    string `yaml:"plain_token"`
	}

	cfg := &testConfig{
		AdminPassword: "supersecretpwd",
		AppSecret:     "very-secret-token",
		NormalData:    "public-data",
		PlainToken:    "oauth-token-val",
	}

	masked := maskSensitiveData(cfg).(map[string]interface{})

	assert.Equal(t, "******", masked["admin-password"])
	assert.Equal(t, "******", masked["app_secret"])
	assert.Equal(t, "******", masked["plain_token"])
	assert.Equal(t, "public-data", masked["normal-data"])
}

func TestActuator_ExposureFilter(t *testing.T) {
	// Register properties manually
	mProps := &ManagementProperties{
		Endpoints: EndpointsProperties{
			Web: WebProperties{
				Exposure: ExposureProperties{
					Include: "health,info,loggers",
				},
			},
		},
	}
	ioc.GetContainer().RegisterBean("ManagementProperties", mProps)

	assert.True(t, isEndpointExposed("health"))
	assert.True(t, isEndpointExposed("loggers"))
	assert.False(t, isEndpointExposed("env"))
	assert.False(t, isEndpointExposed("beans"))

	// Verify wildcard
	mProps.Endpoints.Web.Exposure.Include = "*"
	assert.True(t, isEndpointExposed("env"))
	assert.True(t, isEndpointExposed("beans"))
}

func TestActuator_SecurityAuthMiddleware(t *testing.T) {
	props := &BasicAuthProperties{
		Name:     "admin",
		Password: "fixed-password",
	}
	ioc.GetContainer().RegisterBean("BasicAuthProperties", props)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// 1. Non-actuator route bypasses auth
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	w := httptest.NewRecorder()
	ActuatorBasicAuthMiddleware(next).ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 2. Health endpoint bypasses auth
	req = httptest.NewRequest(http.MethodGet, "/actuator/health", nil)
	w = httptest.NewRecorder()
	ActuatorBasicAuthMiddleware(next).ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 3. Sensitive actuator endpoint gets 401 without auth
	req = httptest.NewRequest(http.MethodGet, "/actuator/beans", nil)
	w = httptest.NewRecorder()
	ActuatorBasicAuthMiddleware(next).ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Header().Get("WWW-Authenticate"), "Basic")

	// 4. Sensitive actuator endpoint gets 200 with correct auth
	req = httptest.NewRequest(http.MethodGet, "/actuator/beans", nil)
	req.SetBasicAuth("admin", "fixed-password")
	w = httptest.NewRecorder()
	ActuatorBasicAuthMiddleware(next).ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}


func TestActuator_BasicAuthPropertiesProductionValidation(t *testing.T) {
	t.Setenv("SPRINGO_PROFILES_ACTIVE", "prod")
	props := &BasicAuthProperties{Name: "admin", Password: ""}
	assert.Error(t, props.Validate())

	props.Password = "secure-prod-pass"
	assert.NoError(t, props.Validate())
}
