package response

// InfoResponseDTO holds application metadata
type InfoResponseDTO struct {
	App         AppInfo `json:"app"`
	Version     string  `json:"version"`
	Environment string  `json:"environment"`
}

type AppInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}
