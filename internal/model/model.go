package model

const (
	DefaultMaxBodyBytes   int64 = 8 << 20
	DefaultMaxBatch             = 1000
	DefaultMaxBannerBytes       = 64 << 10
)

type Scan struct {
	IP             string `json:"ip"`
	Port           int    `json:"port"`
	Banner         string `json:"banner"`
	BannerEncoding string `json:"banner_encoding,omitempty"`
}

type Result struct {
	IP         string  `json:"ip"`
	Port       int     `json:"port"`
	Protocol   string  `json:"protocol"`
	Product    string  `json:"product"`
	Version    string  `json:"version"`
	OSHint     string  `json:"os_hint"`
	Confidence float64 `json:"confidence"`
}

type APIError struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Index     *int   `json:"index,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

func Unknown(s Scan) Result {
	return Result{IP: s.IP, Port: s.Port, Protocol: "unknown"}
}
